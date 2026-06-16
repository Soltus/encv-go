// internal/server/mock_generator.go
// 自动化测试用 Mock 数据生成 / 重置（Go 后端）
//
// 提供两个端点：
//   POST /api/mock/generate { root, type }  → SSE 流式进度
//   POST /api/mock/reset    { root }         → JSON { removed }
//
// 用途：自动化测试入口在前端触发时，需要把 mock 文件写入到：
//   - 真机 / dev preview：<servingDir>/01-plain-media/ 等
//   - 自动化测试命名空间：<servingDir>/encv-automation/01-plain-media/ 等
// 前端在浏览器/真机 WebView 没有权限直写这些目录，必须走后端。
//
// 安全：root 必须在白名单前缀内（见 validateMockRoot），否则 403。
package server

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"log/slog"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"sync/atomic"
	"time"

	"github.com/Soltus/encv-go/internal/utils/ffmpeg"
	"github.com/gin-gonic/gin"
)

// ════════════════════════════════════════════════════════════════════
// 🆕 2026-06-11 修复：mp4/mkv 真机错（base64 fallback 是垃圾）
//
// 历史：
//   - 2026-06-10：后端 ffmpeg 优先 + base64 内嵌 fallback（MP4_B64 4.8KB / MKV_B64 171B）
//   - 2026-06-11：用户反馈「真机 APK 上 ffmpeg 不存在，永远走 base64 fallback —— 傻逼，
//     集成的 ffmpeg 是摆设吗？给我用，删掉 base64」
//
// 根因（旧 fallback 缺陷）：
//   - mock_generator.go 直接 exec.Command("ffmpeg", ...) → 真机没有 /usr/bin/ffmpeg → 必 fail
//   - fail → 静默 fallback 到 base64 → MKV_B64 仅 171 字节（只有 EBML header，无视频帧）
//   - 真机生成的 mp4/mkv 不能播放、不能 ffprobe → 自动化测试跑挂
//
// 修复方案（2026-06-11）：
//   1. ffmpeg 调用改走项目集成的 internal/utils/ffmpeg.Runner 抽象层
//      - 沙箱 (!android build tag)：ExecRunner 用 os/exec 调 /usr/bin/ffmpeg
//      - 真机 (android build tag)：NativeRunner 用 cgo dlopen 调 libffmpeg.so (ffmpeg_run)
//      - 真机必须先跑 app/encv-mobile/scripts/build-ffmpeg-android.sh 编 libffmpeg.so 打到 APK jniLibs
//   2. mp4/mkv ffmpeg 调用补 -map 0:a -map 1:v（之前 2 个 input 没指定 stream，sine+color
//      默认选 video from input 0 = sine 失败）
//   3. 删 base64 fallback（mock_media_bytes.go 整个文件 + decodeBase64Media 函数）
//      - 失败就返回 nil，让调用方报错（不静默给垃圾字节）
//   4. 测试在 ffmpeg 不可用时 SKIP（CI 容器可能没 ffmpeg，不算失败）
//
// 验证：
//   - 沙箱：/usr/bin/ffmpeg 6.1 在 → 实际跑 mp4=19801B / mkv=9453B / mp3=33062B / flac=32487B
//   - 真机：build-ffmpeg-android.sh 编 libffmpeg.so → APK jniLibs → dlopen 调 ffmpeg_run
// ════════════════════════════════════════════════════════════════════

// 🆕 2026-06-15 multi-mount 重构：root 必须是 /d/<mount>/... 形式的虚拟挂载路径。
//
//   - /d/automation/...  → 走 mount registry 解析到真机 appdata 目录（解决 P1 EACCES）
//   - /d/primary/...     → 走 primary 挂载（用户数据，给 Files 浏览器用）
//   - 其他 /d/<x>/...    → 任意已注册挂载
//   - 旧绝对路径（/storage/emulated/0/...）一律 403 —— 强制迁移
//
// 旧 mockRootAllowList 静态白名单删除（spec Phase B1）。旧定义归档见文末。

// mockGeneratorRequest 是 POST /api/mock/generate 的请求体
type mockGeneratorRequest struct {
	Root string `json:"root"`
	Type string `json:"type"` // "all" | "plain" | "ae" | "container" | "boundary"
}

// mockGeneratorProgress 是 SSE progress 事件 payload
type mockGeneratorProgress struct {
	RelativePath string `json:"relativePath"`
	Size         int    `json:"size"`
}

// mockGeneratorDone 是 SSE done 事件 payload
type mockGeneratorDone struct {
	Count     int   `json:"count"`
	TotalSize int64 `json:"totalSize"`
}

// validateMockRoot 校验 root 是否是有效的 mount 路径。
//
// 2026-06-15 multi-mount 重构：root 必须是 /d/<mount>/... 形式
//  → 通过 mountRegistry.Resolve 校验挂载存在 + 解析出 abs 路径
//  → 旧绝对路径（/storage/emulated/0/...）一律拒绝
//
// 2026-06-15 增强反馈：错误响应里列出现有 mount 列表 + 提示正确路径
//  → 真实环境 user 看到 "invalid mount path" 不知道该用啥
//  → 以前直接 403 文本 → 现在 403 JSON {error, available_mounts, hint}
func (s *Server) validateMockRoot(root string) error {
	if root == "" {
		return fmt.Errorf("root is empty")
	}
	if s.mountRegistry == nil {
		return fmt.Errorf("mount registry not initialized")
	}
	if !strings.HasPrefix(root, "/d/") {
		// 列出当前可用 mount 帮用户诊断
		available := s.listMountNames()
		return fmt.Errorf(
			"root %q must be a mount path (start with /d/...); "+
				"available mounts: [%s] — "+
				"frontend should compute mockRoot from DEFAULT_AUTOMATION_SOURCE (slice 0..3 = '/d/automation'), "+
				"not raw absolute path",
			root, available,
		)
	}
	if _, err := s.mountRegistry.Resolve(root); err != nil {
		available := s.listMountNames()
		return fmt.Errorf(
			"resolve %q: %w; available mounts: [%s] — "+
				"前端 mockRoot 派生 bug 常见：DEFAULT_AUTOMATION_SOURCE.split('/').slice(0, N) 的 N 应该是 3 而不是 5 "+
				"（取多了会把子目录当 mount 路径）",
			root, err, available,
		)
	}
	return nil
}

// listMountNames 返回当前 mount registry 里的 mount 名字列表（用于错误反馈，字符串）
func (s *Server) listMountNames() string {
	if s.mountRegistry == nil {
		return "<registry nil>"
	}
	list := s.mountRegistry.List()
	names := make([]string, 0, len(list))
	for _, m := range list {
		if m.Enabled {
			names = append(names, m.Name+"→"+m.MountPath)
		}
	}
	return strings.Join(names, ", ")
}

// mountSummary 是错误响应里 available_mounts 数组的单个元素
type mountSummary struct {
	Name      string `json:"name"`
	MountPath string `json:"mount_path"`
	RootPath  string `json:"root_path"`
	Enabled   bool   `json:"enabled"`
}

// listMountSummaries 返回当前 mount registry 里的 mount 摘要列表（结构化）
// 2026-06-15 增强反馈用：前端可以直接渲染成列表
func (s *Server) listMountSummaries() []mountSummary {
	if s.mountRegistry == nil {
		return []mountSummary{}
	}
	list := s.mountRegistry.List()
	out := make([]mountSummary, 0, len(list))
	for _, m := range list {
		if !m.Enabled {
			continue
		}
		out = append(out, mountSummary{
			Name:      m.Name,
			MountPath: m.MountPath,
			RootPath:  m.RootPath,
			Enabled:   m.Enabled,
		})
	}
	return out
}

// mockFileSpec 描述一个待生成的文件
//
// 🆕 2026-06-12 饱和调试：data 为 nil 时不再是「静默失败」—— 透传 ffmpeg 完整诊断
//   （stderr / exitCode / ffmpegArgs / ext / encoder）让前端能展示「在哪个具体调用挂的」
//
// 🆕 2026-06-12 重构：ffmpeg 不在 spec 构造阶段跑，挪到 handler 阶段跑
//   历史 bug（真机 cgo 阻塞 30s+）：
//     旧 planMockSpecs + ffmpegSpec → handler 入口 ffmpeg.RunWithOutput 卡住
//     → SSE 第一个 spec_diag 都没发出去 → 前端 30s abort 时**完全不知道卡在哪个 spec**
//   修复：planMockSpecs 只构造 spec 列表（含 ffmpegArgs，但 data=nil）
//     handler 拿到 spec 后**先** emit 9 个 spec_diag 给前端（让前端立刻看到流程结构）
//     **再** 循环跑 ffmpeg + emit spec_done / spec_failed
//   效果：即使真机 cgo 卡 mp4，前端已经看到「接下来要跑 mp3/flac/...」的 9 行
//         30s abort 时 inline error card 含「最后收到的 spec_diag → 卡在 mp4 这步」
type mockFileSpec struct {
	relativePath string
	// data 在 planMockSpecs 阶段为 nil；handler 跑 ffmpeg 后填
	data []byte
	// ffmpegArgs 仅在 ffmpegGenerate 调用过的 spec 上有值（mp4/mkv/mp3/flac/m4a）
	// PNG/JPEG/PDF/TXT/AE/SCCV 等硬编码字节的 spec 留空
	ffmpegArgs []string
	// stderr 是 ffmpeg stderr 全文（含 ffmpeg 头部 + Unknown encoder 等关键信息）
	// data 为 nil 时必有值
	stderr string
	// exitCode 是 ffmpeg 退出码；data 为 nil 时必有值（124 = ctx timeout / -1 = spawn 失败）
	exitCode int
	// encoderHint 是源码层面推断的 encoder（mp4=h264, mkv=h264, mp3=libmp3lame, flac=flac）
	// 失败时前端可直接对比 manifest 看该 encoder 是否在 ffmpeg build 里
	encoderHint string
	// 🆕 2026-06-12：区分 codec 用途（m4a lossy vs m4a lossless 走不同 encoder）
	//   静态字节 spec 标志（planMockSpecs 直接填 data，不走 ffmpeg）
	isStatic bool
	// 🆕 2026-06-12：runner 标识（ffmpeg / mediacodec / static）
	//   Phase 3.3 MediaCodec 实装后填 'mediacodec'；当前固定 'ffmpeg'
	runner string
	// 🆕 修复 B1 + B2 (2026-06-17)：增强调试字段
	//   - srcSize: 源文件实际写入字节数（失败排查「源是否被成功写出」）
	//   - dstSize: 目标文件实际写入字节数（失败排查「ffmpeg 是否写了部分输出」）
	//   - workerTmpDir: worker 实际使用的 tmp dir（resolved，失败时为空）
	//   - workerError: worker 响应的 error 字段（与 stderr 区分；可定位 ENGINE_LOAD_FAILED 等）
	//   - contextInfo: 拼接好的「worker_tmp_dir + lib_dir + pid + src/dst size」一行可读文本
	srcSize       int64
	dstSize       int64
	workerTmpDir  string
	workerError   string
	contextInfo   string
}

// generateMockSpecs 返回指定 type 的所有文件 specs
// 字节内容是硬编码的最小有效格式（与前端 lib/mockDataGenerator.ts 对齐）
//
// 🆕 2026-06-12 重构：**不跑 ffmpeg**！ffmpeg 调用挪到 handler 阶段
//   静态字节 spec 立即填 data；ffmpeg spec 留 ffmpegArgs + status="pending" + data=nil
//   handler 拿到 spec 列表后**先** emit 9 个 spec_diag 给前端，**再**循环跑 ffmpeg
//   目的：真机 cgo 阻塞 mp4 时，前端已看到「接下来 mp3/flac/pdf」流程结构
func generateMockSpecs(typeName string) []mockFileSpec {
	plainSpecs := []mockFileSpec{
		{relativePath: "01-plain-media/image/photo.jpg", data: minimalJPEG(), encoderHint: "JPEG (static)", isStatic: true, runner: "static"},
		{relativePath: "01-plain-media/image/screenshot.png", data: minimalPNG(), encoderHint: "PNG (static)", isStatic: true, runner: "static"},
		planMP4(),
		planMKV(),
		planM4A(),        // 🆕 2026-06-12 m4a 容器 + aac 编（lossy，零成本，manifest 已有）
		planM4ALossless(), // 🆕 2026-06-12 m4a 容器 + alac 编（lossless，ffmpeg 内置 encoder）
		planMP3(),
		planFLAC(),
		{relativePath: "01-plain-media/document/report.pdf", data: minimalPDF(), encoderHint: "PDF (static)", isStatic: true, runner: "static"},
		{relativePath: "01-plain-media/document/notes.txt", data: []byte("ENCV Mock Notes\n中文测试\n日本語テスト\n한국어 테스트\n"), encoderHint: "TXT (static)", isStatic: true, runner: "static"},
		{relativePath: "01-plain-media/document/data.csv", data: []byte("id,name,size\n1,photo.jpg,107\n2,sample.mp4,45056\n"), encoderHint: "CSV (static)", isStatic: true, runner: "static"},
	}
	aeSpecs := []mockFileSpec{
		{relativePath: "02-alist-encrypt/secret.ae", data: makeAEFile("secret.ae", 4096), encoderHint: "AE (static)", isStatic: true},
		{relativePath: "02-alist-encrypt/document.ae", data: makeAEFile("document.ae", 8192), encoderHint: "AE (static)", isStatic: true},
		{relativePath: "02-alist-encrypt/hidden-gem.ae", data: makeAEFile("hidden-gem.ae", 16384), encoderHint: "AE (static)", isStatic: true},
	}
	containerSpecs := []mockFileSpec{
		{relativePath: "03-encv-containers/container.sccgv", data: makeSCCVFile("container", "sccgv", 8192), encoderHint: "SCCGV (static)", isStatic: true},
		{relativePath: "03-encv-containers/archive.scext", data: makeSCCVFile("archive", "scext", 16384), encoderHint: "SCCEXT (static)", isStatic: true},
		{relativePath: "03-encv-containers/bundle.scepkg", data: makeSCCVFile("bundle", "scepkg", 32768), encoderHint: "SCCEPKG (static)", isStatic: true},
	}
	boundarySpecs := []mockFileSpec{
		{relativePath: "04-boundary-test/zero-byte-file.bin", data: []byte{}, encoderHint: "BIN (static)", isStatic: true},
		{relativePath: "04-boundary-test/single-byte.bin", data: []byte{0x42}, encoderHint: "BIN (static)", isStatic: true},
		{relativePath: "04-boundary-test/exactly-1kb.bin", data: makeBytes(1024, 0x41), encoderHint: "BIN (static)", isStatic: true},
		{relativePath: "04-boundary-test/large-1mb.dat", data: makeBytes(1024*1024, 0x58), encoderHint: "BIN (static)", isStatic: true},
		{relativePath: "04-boundary-test/normal.txt", data: []byte("plain text"), encoderHint: "TXT (static)", isStatic: true},
	}

	switch typeName {
	case "plain":
		return plainSpecs
	case "ae":
		return aeSpecs
	case "container":
		return containerSpecs
	case "boundary":
		return boundarySpecs
	case "all", "":
		return append(append(append(plainSpecs, aeSpecs...), containerSpecs...), boundarySpecs...)
	default:
		return nil
	}
}

// handleMockGenerateGin 处理 POST /api/mock/generate
// SSE response:
//   - event: progress  data: { "relativePath": "...", "size": N }
//   - event: done      data: { "count": N, "totalSize": M }
func (s *Server) handleMockGenerateGin(c *gin.Context) {
	// 🆕 2026-06-12 饱和防御：defer recover 防 SSE 写 closed conn 时 panic 整进程崩
	// 即使 gin.Recovery() 顶层 middleware 会兜 500，但 panic 后 mockGenMu 仍需 Unlock
	//（gin.Recovery 走 c.AbortWithStatus，handler 内 defer 仍会执行）
	// 这里再包一层：
	// 1) 防止真机 cgo 内 panic 跨越 cgo boundary（gin.Recovery 抓不到）
	// 2) panic 时尝试 emit 一次 SSE error event（不一定成功，conn 可能已 close）
	// 3) panic 详情写 slog 供 adb logcat 排查
	defer func() {
		if r := recover(); r != nil {
			slog.Error("[mock_gen] PANIC recovered",
				"panic", fmt.Sprintf("%v", r),
				"root", c.Param("root"),
				"method", c.Request.Method,
				"url", c.Request.URL.Path,
			)
			// 尝试给客户端写 error（可能写不进去，conn 已 close）
			if !c.Writer.Written() {
				_ = c.Error(fmt.Errorf("mock_generate panic: %v", r)) //nolint:errcheck
			}
		}
	}()

	// 🆕 2026-06-10：显式意图确认（防擅自生成）
	//  - 防止 preflight / 第三方爬虫 / 误调触发数据生成
	//  - 前端 UI 按钮自动带 X-Confirm-Mock-Mutation: yes
	//  - Node CLI 已废弃，不存在自动调用方
	if c.GetHeader("X-Confirm-Mock-Mutation") != "yes" {
		slog.Warn("Mock generate rejected: missing confirm header")
		c.JSON(http.StatusForbidden, gin.H{
			"error": "X-Confirm-Mock-Mutation header required (UI 按钮自动带；防擅自生成)",
		})
		return
	}

	var req mockGeneratorRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "invalid body: " + err.Error()})
		return
	}

	if err := s.validateMockRoot(req.Root); err != nil {
		slog.Warn("Mock generate rejected: invalid mount path", "root", req.Root, "error", err)
		// 🆕 2026-06-15 增强反馈：返回 JSON {error, available_mounts, hint} 给前端
		//   前端能直接显示「当前可用 mount 列表」+「正确用法」+「定位 mockRoot slice bug」
		//   available_mounts 是结构化数组 [{name, mount_path, root_path}, ...]
		//   不再是字符串（前端解析更简单）
		c.JSON(http.StatusForbidden, gin.H{
			"error": err.Error(),
			"available_mounts": s.listMountSummaries(),
			"hint":  "mockRoot should be /d/automation (or /d/primary, /d/sandbox), not a sub-path under a mount",
		})
		return
	}

	// 🆕 2026-06-15 multi-mount: 解析虚拟挂载路径到绝对路径
	//   /d/automation → /data/user/<uid>/com.encvgo.app/files/encv-automation
	//   /d/primary    → /storage/emulated/0
	res, err := s.mountRegistry.Resolve(req.Root)
	if err != nil {
		slog.Error("Mock generate: mount resolve failed", "root", req.Root, "error", err)
		c.JSON(http.StatusInternalServerError, gin.H{"error": "mount resolve failed: " + err.Error()})
		return
	}
	root := res.AbsPath

	specs := generateMockSpecs(req.Type)
	if specs == nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "invalid type: " + req.Type})
		return
	}

	// 🆕 2026-06-11 修复：mock generate 并发 race
	// 多请求同时写同一文件（os.WriteFile 非原子：open(O_TRUNC) → write → close）→
	//   文件状态取决于最后 close 的 goroutine → count/size 不稳定
	// 全局互斥串行化（dev tool 低频，串行可接受；Phase 3 可改 per-path 锁）
	s.mockGenMu.Lock()
	defer s.mockGenMu.Unlock()

	// SSE writer
	c.Writer.Header().Set("Content-Type", "text/event-stream")
	c.Writer.Header().Set("Cache-Control", "no-cache")
	c.Writer.Header().Set("Connection", "keep-alive")
	c.Writer.Header().Set("X-Accel-Buffering", "no")

	flusher, ok := c.Writer.(http.Flusher)
	if !ok {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "streaming unsupported"})
		return
	}

	count := 0
	skipped := 0
	var totalSize int64

	// 🆕 2026-06-12 饱和调试 + 异步化：handler 入口**先** emit starting + 9 行 spec_plan（pending 状态）
	//   真机 cgo 阻塞 mp4 时：前端立刻看到 9 行流程条目，30s abort 时能定位
	//   即使后续 ffmpeg 阻塞导致后续 SSE 发不出，前端已知"待跑 9 个"
	if err := writeSseEvent(c.Writer, flusher, "starting", fmt.Sprintf(`{"total": %d, "type": %q, "root": %q}`, len(specs), req.Type, root)); err != nil {
		slog.Warn("[mock_gen] SSE starting write failed", "err", err)
	}
	for i, sp := range specs {
		diagData := fmt.Sprintf(
			`{"index": %d, "total": %d, "relativePath": %q, "status": "pending", "encoder": %q, "runner": %q, "ffmpegArgs": %s, "exitCode": 0, "stderr": "", "srcSize": 0, "dstSize": 0, "workerTmpDir": "", "workerError": "", "contextInfo": ""}`,
			i+1, len(specs), sp.relativePath, sp.encoderHint, sp.runner, jsonEscapeStringSlice(sp.ffmpegArgs),
		)
		if err := writeSseEvent(c.Writer, flusher, "spec_plan", diagData); err != nil {
			slog.Warn("[mock_gen] SSE plan write failed (client gone?)", "err", err, "relativePath", sp.relativePath)
			break
		}
	}

	// 🆕 2026-06-12 异步化：execute 跑在独立 goroutine，主 handler 在 select 上等
	//   - 真机 cgo 阻塞 mp4 时：goroutine 阻塞 → main select 不阻塞 → ticker 持续 emit heartbeat
	//     → 前端 SSE 流持续有数据 → 30s abort 时**能区分"真机在跑"vs"真机死了"**
	//   - 客户端断开 → c.Request.Context().Done() 触发 → main handler 退出 + 标记 clientGone
	//     注意：goroutine 内 execute 仍跑（cgo 不响应 ctx），但因为 clientGone 不再 emit 也不写文件
	//   - ticker 2s emit heartbeat 让前端知道"流还活着"
	//
	// ⚠️ 已知限制：cgo OS 线程不响应 Go context cancel — goroutine 仍阻塞
	//   这是 ffmpeg cgo dlopen 的根本问题，需要把 cgo 移到独立进程（Phase 3 重构）
	//   当前修复**至少**让前端能看到"真机在跑只是慢"vs"流断了"
	resultCh := make(chan mockFileSpec, len(specs))
	runCtx, runCancel := context.WithCancel(c.Request.Context())
	defer runCancel()

	go func() {
		// 重要：defer recover 防 cgo panic 跨 boundary 杀整进程
		defer func() {
			if r := recover(); r != nil {
				slog.Error("[mock_gen] execute goroutine PANIC recovered", "panic", fmt.Sprintf("%v", r))
			}
			close(resultCh)
		}()
		for i := range specs {
			if runCtx.Err() != nil {
				return
			}
			sp := &specs[i]
			if !sp.isStatic {
				executeMockSpec(sp)
			}
			select {
			case resultCh <- *sp:
			case <-runCtx.Done():
				return
			}
		}
	}()

	// 🆕 2026-06-12 饱和防御：跟踪 SSE 写入错误，客户端断开时优雅停止
	clientGone := false
	heartbeat := time.NewTicker(2 * time.Second)
	defer heartbeat.Stop()
	completed := 0

	for completed < len(specs) {
		select {
		case sp, ok := <-resultCh:
			if !ok {
				// goroutine 异常退出
				slog.Warn("[mock_gen] resultCh closed early", "completed", completed, "total", len(specs))
				return
			}
			if clientGone {
				completed++
				continue
			}

			// emit spec_diag (execute 结果)
			diagStatus := "ok"
			if len(sp.data) == 0 {
				diagStatus = "failed"
			}
			// 找原 index（specs slice index — sp 来自 resultCh 元素）
			// 这里我们没传 index，**用 relativePath 反查**
			idx := 0
			for i, orig := range specs {
				if orig.relativePath == sp.relativePath {
					idx = i + 1
					break
				}
			}
			diagData := fmt.Sprintf(
				`{"index": %d, "total": %d, "relativePath": %q, "status": %q, "encoder": %q, "runner": %q, "ffmpegArgs": %s, "exitCode": %d, "stderr": %q, "srcSize": %d, "dstSize": %d, "workerTmpDir": %q, "workerError": %q, "contextInfo": %q}`,
				idx, len(specs), sp.relativePath, diagStatus, sp.encoderHint, sp.runner,
				jsonEscapeStringSlice(sp.ffmpegArgs),
				sp.exitCode, sp.stderr,
				sp.srcSize, sp.dstSize, sp.workerTmpDir, sp.workerError, sp.contextInfo,
			)
			if err := writeSseEvent(c.Writer, flusher, "spec_diag", diagData); err != nil {
				clientGone = true
				slog.Warn("[mock_gen] SSE diag write failed (client gone?)", "err", err, "relativePath", sp.relativePath)
				completed++
				continue
			}

			fullPath := filepath.Join(root, sp.relativePath)
			dir := filepath.Dir(fullPath)
			if err := os.MkdirAll(dir, 0755); err != nil {
				_ = emitSseEvent(c.Writer, flusher, "error", fmt.Sprintf(`{"error": "mkdir %s: %s"}`, dir, err.Error()))
				return
			}
			if len(sp.data) == 0 {
				skipped++
				slog.Warn("[mock] skip nil data", "relativePath", sp.relativePath, "reason", "ffmpegGenerate returned nil (likely encoder not compiled in this ffmpeg build)")
				_ = emitSseEvent(c.Writer, flusher, "spec_failed", fmt.Sprintf(
					`{"relativePath": %q, "reason": "ffmpeg 不可用/未编该 encoder (真机常见：libmp3lame/flac 等)", "exitCode": %d, "stderr": %q}`,
					sp.relativePath, sp.exitCode, sp.stderr,
				))
				completed++
				continue
			}
			if err := os.WriteFile(fullPath, sp.data, 0644); err != nil {
				_ = emitSseEvent(c.Writer, flusher, "error", fmt.Sprintf(`{"error": "write %s: %s"}`, sp.relativePath, err.Error()))
				return
			}
			count++
			totalSize += int64(len(sp.data))
			// SSE event: progress
			if err := writeSseEvent(c.Writer, flusher, "progress", fmt.Sprintf(`{"relativePath": %q, "size": %d}`, sp.relativePath, len(sp.data))); err != nil {
				clientGone = true
				slog.Warn("[mock_gen] SSE write failed (client gone?)", "err", err, "relativePath", sp.relativePath)
			}
			completed++

		case <-heartbeat.C:
			// 🆕 2026-06-12 异步化：2s heartbeat 让前端知道"流还活着"
			//   真机 cgo 阻塞时，前端至少能看到"还在线"不会误判"死了"
			if !clientGone {
				hbData := fmt.Sprintf(`{"completed": %d, "total": %d, "ts": %d}`, completed, len(specs), time.Now().UnixMilli())
				if err := writeSseEvent(c.Writer, flusher, "heartbeat", hbData); err != nil {
					clientGone = true
					slog.Warn("[mock_gen] SSE heartbeat write failed (client gone?)", "err", err)
				}
			}

		case <-runCtx.Done():
			// 客户端断开（c.Request.Context）— 停止写文件、停止 emit
			slog.Warn("[mock_gen] client context done, aborting", "completed", completed, "total", len(specs))
			return
		}
	}
	// 🆕 done 事件带 skipped 字段（前端可显示"X 个格式因 ffmpeg build 限制跳过"）
	if err := writeSseEvent(c.Writer, flusher, "done", fmt.Sprintf(`{"count": %d, "skipped": %d, "totalSize": %d}`, count, skipped, totalSize)); err != nil {
		slog.Warn("[mock_gen] SSE done write failed", "err", err, "count", count, "skipped", skipped)
	}
}

// jsonEscapeStringSlice JSON-encode a []string
// 🆕 2026-06-12 饱和调试：spec_diag 事件里 ffmpegArgs 字段需要 JSON 数组
//   fmt.Sprintf("%q", slice) 不行（Go 字符串转义，输出 `"a b"` 不会被 JSON 解析为数组）
//   改用 encoding/json.Marshal → 失败时回退到空数组
func jsonEscapeStringSlice(s []string) string {
	if s == nil {
		return "[]"
	}
	b, err := json.Marshal(s)
	if err != nil {
		return "[]"
	}
	return string(b)
}

// mockResetRequest 是 POST /api/mock/reset 的请求体
type mockResetRequest struct {
	Root string `json:"root"`
}

// handleMockResetGin 处理 POST /api/mock/reset
// 🆕 2026-06-10 修复：递归删除 mockRoot 下的 4 个子目录全部内容
// 历史 bug：只删 generateMockSpecs 列出的具体文件，但 02-test-output 等其他子目录不删
// 修复：清空 4 个已知子目录（01-plain-media / 02-alist-encrypt / 03-encv-containers / 04-boundary-test）
//       + 02-test-output（自动化测试运行时生成的产物），保留目录结构
func (s *Server) handleMockResetGin(c *gin.Context) {
	// 🆕 2026-06-12 饱和防御：defer recover（与 handleMockGenerateGin 同款）
	defer func() {
		if r := recover(); r != nil {
			slog.Error("[mock_reset] PANIC recovered",
				"panic", fmt.Sprintf("%v", r),
				"url", c.Request.URL.Path,
			)
		}
	}()

	var req mockResetRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "invalid body: " + err.Error()})
		return
	}
	if err := s.validateMockRoot(req.Root); err != nil {
		c.JSON(http.StatusForbidden, gin.H{"error": err.Error()})
		return
	}
	// 🆕 2026-06-15 multi-mount: 解析虚拟挂载路径到绝对路径
	res, err := s.mountRegistry.Resolve(req.Root)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "mount resolve failed: " + err.Error()})
		return
	}
	root := res.AbsPath

	// 已知子目录（保留目录结构，删除其中内容）
	knownSubdirs := []string{
		"01-plain-media",
		"02-alist-encrypt",
		"03-encv-containers",
		"04-boundary-test",
		// 🆕 自动化测试运行产物（buildDynamicWorkflow 用 targetPath 写到这里的子目录）
		"02-test-output",
	}

	// 🆕 2026-06-11：复用 mockGenMu，防止 generate/reset 互踩
	s.mockGenMu.Lock()
	defer s.mockGenMu.Unlock()

	removed := 0
	for _, sub := range knownSubdirs {
		dir := filepath.Join(root, sub)
		if _, err := os.Stat(dir); os.IsNotExist(err) {
			continue
		}
		// 遍历子目录中所有文件并删除
		err := filepath.WalkDir(dir, func(path string, d os.DirEntry, walkErr error) error {
			if walkErr != nil {
				return nil // 跳过不可访问的文件
			}
			if d.IsDir() {
				return nil
			}
			if rmErr := os.Remove(path); rmErr == nil {
				removed++
			}
			return nil
		})
		if err != nil {
			slog.Warn("Mock reset: walk failed", "dir", dir, "error", err)
		}
	}

	// 同时尝试删除 generateMockSpecs 中已知的具体文件（防御性，保留对旧版兼容）
	for _, sp := range generateMockSpecs("all") {
		fullPath := filepath.Join(root, sp.relativePath)
		if err := os.Remove(fullPath); err == nil {
			removed++
		}
	}

	c.JSON(http.StatusOK, gin.H{"removed": removed})
}

// writeSseEvent 写一个 SSE 事件（event: <name>\ndata: <payload>\n\n）
//
// 🆕 2026-06-12 饱和防御：返回 error，调用方在客户端断开时能优雅停止 handler
//
// 旧实现：_, _ = fmt.Fprintf(...); flusher.Flush() — 两个 error 全忽略
//   → 客户端提前断开（abort / 切 tab / 网络抖动）时 handler 继续跑全 9 个文件
//   → 浪费 CPU + fd + goroutine 持有 mockGenMu 锁直到下一个 emit 失败（30s 后）
//
// 客户端断开检测：fmt.Fprintf 写 closed conn 返回 "broken pipe" / "connection reset"
//   http.Flusher.Flush() 不返回 error（stdlib 限制），所以只能信 fmt.Fprintf
//   gin 实际写入路径：responseWriter.Write() → wrapped ResponseWriter.Write()
//   → 底层 net.TCPConn.Write() → broken pipe
func writeSseEvent(w io.Writer, flusher http.Flusher, event, data string) error {
	if _, err := fmt.Fprintf(w, "event: %s\ndata: %s\n\n", event, data); err != nil {
		return fmt.Errorf("sse write event=%s: %w", event, err)
	}
	flusher.Flush() // http.Flusher 接口无返回值，错误由下次 Write 反映
	return nil
}

// emitSseEvent 是 writeSseEvent 的语义化别名（用于错误事件）
func emitSseEvent(w io.Writer, flusher http.Flusher, event, data string) error {
	return writeSseEvent(w, flusher, event, data)
}

// ════════════════════════════════════════════════════════════════════
// 最小有效字节模板（ffmpeg 真输入 → 输出，不用 lavfi）
// ════════════════════════════════════════════════════════════════════
//
// 2026-06-11 v3 改造：恢复调 ffmpeg（用户反馈"辛苦集成你不调"）
//   v1: ffmpeg + lavfi（真机崩，lavfi 没编）
//   v2: go:embed 预编码 mp4/mkv/mp3/flac（绕开 ffmpeg，被批）
//   v3: ffmpeg + 真输入文件（go:embed source.mp4/source.wav → 写 tmp → ffmpeg 读）
//
// 真机 ffmpeg build manifest 限制（[app/encv-mobile/scripts/ffmpeg-feature-manifest.json]）：
//   encoders: aac, pcm_s16le, pcm_s24le, pcm_s32le, libx264
//   muxers:   mp4, matroska, flac, mp3, adts, null
//   demuxers: mov, matroska, aac, mp3, flac, ogg, wav
//
// 因此真机能生成的：
//   mp4  ✅ mov demuxer + mp4 muxer + h264/aac（用 source.mp4 -c copy）
//   mkv  ✅ mov demuxer + matroska muxer + h264/aac（用 source.mp4 -c copy）
//   mp3  ❌ 没 libmp3lame encoder
//   flac ❌ 没 flac encoder
//
// 沙箱 ffmpeg 6.1 完整，4 个全 OK。
// 测试在沙箱跑全部 4 个 subtest，real device 仅 mp4/mkv 有数据。
// ════════════════════════════════════════════════════════════════════

// ffmpegGenerate 用 ffmpeg + 真输入文件生成目标格式
//
// 🆕 2026-06-12 饱和调试签名：返回 (data, stderr, exitCode, err)
//  - data: 成功时返回字节；任何错误都返回 nil
//  - stderr: ffmpeg stderr 全文（含 ffmpeg 头部 + 关键错误如 Unknown encoder）
//  - exitCode: ffmpeg 退出码（0=成功, 1=编码失败, 124=ctx timeout, -1=spawn 失败）
//  - err: 仅在 spawn / ctx cancel / 类型断言失败等「前置错误」时非 nil
//
// 失败时**严禁 base64 fallback**（171 字节假 MKV 垃圾），返回 nil 让调用方报错
//
// 流程：
//   1. ffmpeg.Available() 检查（沙箱 exec / 真机 dlopen）
//   2. 写 source.mp4 / source.wav 到 /tmp（go:embed 字节）
//   3. ffmpeg 读真文件 → 输出到 /tmp
//   4. 读回 /tmp 字节
//   5. 任意一步失败 → 透传 stderr / exitCode 给前端
func ffmpegGenerate(ext string) (data []byte, stderr string, exitCode int, err error) {
	// 0. ffmpeg 可用性
	// 🆕 2026-06-15：ffmpeg.Available() → ffmpeg.IsAvailable()（重命名；返回签名不变）
	ffmpegOk, _, errMsg := ffmpeg.IsAvailable()
	if !ffmpegOk {
		return nil, fmt.Sprintf("ffmpeg not available: %s", errMsg), -1, fmt.Errorf("ffmpeg not available: %s", errMsg)
	}

	// 唯一序列号（PID 同进程复用 → 需要 atomic counter 区分每次调用）
	seq := mockFfmpegSeq.Add(1)

	// 1. 选源文件 + 写 tmp
	var srcBytes []byte
	var srcName string
	var srcArgs []string // ffmpeg -i src
	switch ext {
	case "mp4", "mkv", "m4a":
		// 🆕 2026-06-12：m4a 复用 source.mp4（-c copy，零编码成本）
		srcBytes = sourceMP4Bytes
		srcName = fmt.Sprintf("encv-mock-src-%d-%d.mp4", os.Getpid(), seq)
		srcArgs = []string{"-i", ""} // placeholder, set after WriteFile
	case "m4a-lossless":
		// m4a-lossless 必须用 wav 源（因为要从 wav 编码成 alac）
		srcBytes = sourceWAVBytes
		srcName = fmt.Sprintf("encv-mock-src-%d-%d.wav", os.Getpid(), seq)
		srcArgs = []string{"-i", ""}
	case "mp3", "flac":
		srcBytes = sourceWAVBytes
		srcName = fmt.Sprintf("encv-mock-src-%d-%d.wav", os.Getpid(), seq)
		srcArgs = []string{"-i", ""}
	default:
		return nil, fmt.Sprintf("unknown ext: %q (only mp4/mkv/mp3/flac/m4a/m4a-lossless supported)", ext), -1, fmt.Errorf("unknown ext: %q", ext)
	}
	srcPath := filepath.Join(os.TempDir(), srcName)
	if werr := os.WriteFile(srcPath, srcBytes, 0644); werr != nil {
		return nil, fmt.Sprintf("write source tmp %s: %v", srcPath, werr), -1, werr
	}
	defer func() { _ = os.Remove(srcPath) }()
	srcArgs[1] = srcPath

	// 2. 输出 tmp（同样用 seq 唯一化）
	// 🆕 2026-06-12：m4a-lossless 实际输出 .m4a（ext 字段是 m4a-lossless 仅用于编码区分）
	dstExt := ext
	if ext == "m4a-lossless" {
		dstExt = "m4a"
	}
	dstPath := filepath.Join(os.TempDir(), fmt.Sprintf("encv-mock-dst-%d-%d.%s", os.Getpid(), seq, dstExt))
	defer func() { _ = os.Remove(dstPath) }()

	// 3. ffmpeg args
	var encodeArgs []string
	switch ext {
	case "mp4":
		// 真机 ffmpeg manifest 有 aac encoder，用 source.mp4 直接 -c copy（最快）
		encodeArgs = []string{"-c", "copy"}
	case "mkv":
		// source.mp4 -c copy → .mkv（h264+aac → matroska container）
		encodeArgs = []string{"-c", "copy"}
	case "m4a":
		// 🆕 2026-06-12：source.mp4 -c copy → .m4a（mp4 容器，ffmpeg 自动用 ipod/mp4 muxer）
		encodeArgs = []string{"-c", "copy"}
	case "m4a-lossless":
		// 🆕 2026-06-12：source.wav -c:a alac → .m4a（alac 编码，ffmpeg 内置 encoder）
		encodeArgs = []string{"-c:a", "alac"}
	case "mp3":
		// 沙箱有 libmp3lame；真机没编 → 真机返回 nil
		encodeArgs = []string{"-c:a", "libmp3lame", "-b:a", "128k"}
	case "flac":
		// 沙箱有 flac encoder；真机没编 → 真机返回 nil
		encodeArgs = []string{"-c:a", "flac"}
	}

	// 4. 组装完整 args
	args := append([]string{}, srcArgs...)
	args = append(args, encodeArgs...)
	args = append(args, "-y", "-loglevel", "error", dstPath)

	// 5. 跑 ffmpeg
	// 🆕 修复 B1 (2026-06-17)：预 mkdir worker 专属 tmp dir，worker 子进程 SELinux 上下文与 Go 父进程
	//   不同，自身 5 级 fallback 全部失败。Go 父进程走 gomobile File API（Java 上下文）创建，SELinux 正确。
	workerTmpDir := filepath.Join(os.TempDir(), fmt.Sprintf("encv_worker_%d_%d", os.Getpid(), seq))
	if mkdirErr := os.MkdirAll(workerTmpDir, 0700); mkdirErr != nil {
		return nil, fmt.Sprintf("mkdir worker tmp %s: %v", workerTmpDir, mkdirErr), -1, mkdirErr
	}
	defer func() { _ = os.RemoveAll(workerTmpDir) }()

	// 🆕 2026-06-15：ffmpeg.RunWithOutput(ctx, args...) → ffmpeg.Encode(ctx, args...) 返回 *EncodeResult
	// 🆕 修复 B1：用 EncodeWithTmpDir 把 workerTmpDir 传 worker
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	res, runErr := ffmpeg.EncodeWithTmpDir(ctx, workerTmpDir, args...)
	var ffmpegStderr string
	var ffmpegExit int
	var workerTmpDirResolved string
	if res != nil {
		ffmpegStderr = res.Stderr
		ffmpegExit = res.ExitCode
		workerTmpDirResolved = res.WorkerTmpDir
	}
	if runErr != nil {
		// 典型：ctx canceled / ctx deadline exceeded / spawn 失败
		var errDetail string
		if res != nil && res.Error != "" {
			errDetail = "\nworker error: " + res.Error
		}
		// 🆕 修复 B2：补 lib_dir / tmp_dir / 源/目标文件大小 / worker PID 上下文
		var srcSize, dstSize int64
		if fi, _ := os.Stat(srcPath); fi != nil {
			srcSize = fi.Size()
		}
		if fi, _ := os.Stat(dstPath); fi != nil {
			dstSize = fi.Size()
		}
		effTmpDir := workerTmpDirResolved
		if effTmpDir == "" {
			effTmpDir = workerTmpDir
		}
		return nil, fmt.Sprintf(
			"ffmpeg spawn/run: %v\nstderr: %s%s\ncontext: worker_tmp_dir=%s (requested %s) src=%s(size=%d) dst=%s(size=%d) pid=%d args: %v",
			runErr, ffmpegStderr, errDetail,
			effTmpDir, workerTmpDir, srcPath, srcSize, dstPath, dstSize, os.Getpid(), args,
		), ffmpegExit, runErr
	}
	if ffmpegExit != 0 {
		// 典型 stderr：「Unknown encoder 'libmp3lame'」/「Encoder not found」/ 编码器失败
		var errDetail string
		if res != nil && res.Error != "" {
			errDetail = "\nworker error: " + res.Error
		}
		// 🆕 修复 B2：同上补 context
		var srcSize, dstSize int64
		if fi, _ := os.Stat(srcPath); fi != nil {
			srcSize = fi.Size()
		}
		if fi, _ := os.Stat(dstPath); fi != nil {
			dstSize = fi.Size()
		}
		effTmpDir := workerTmpDirResolved
		if effTmpDir == "" {
			effTmpDir = workerTmpDir
		}
		return nil, fmt.Sprintf(
			"ffmpeg exit=%d, stderr: %s%s\ncontext: worker_tmp_dir=%s (requested %s) src=%s(size=%d) dst=%s(size=%d) pid=%d args: %v",
			ffmpegExit, ffmpegStderr, errDetail,
			effTmpDir, workerTmpDir, srcPath, srcSize, dstPath, dstSize, os.Getpid(), args,
		), ffmpegExit, fmt.Errorf("ffmpeg exit=%d", ffmpegExit)
	}

	// 6. 读回
	readBytes, rerr := os.ReadFile(dstPath)
	if rerr != nil || len(readBytes) == 0 {
		return nil, fmt.Sprintf("read dst %s: %v (size=%d)\nargs: %v", dstPath, rerr, len(readBytes), args), ffmpegExit, rerr
	}
	return readBytes, ffmpegStderr, 0, nil
}

// planMockSpec 构造 ffmpeg spec（**不跑 ffmpeg**，仅填 ffmpegArgs + encoderHint）
//   handler 阶段调 executeMockSpec 真正跑 ffmpeg
//
// 🆕 2026-06-12 重构：plan + execute 分离
//   旧逻辑：planMockSpecs 阶段直接调 ffmpeg.RunWithOutput → handler 入口阻塞 30s+
//   新逻辑：planMockSpecs 只构造 spec（含 ffmpegArgs），handler 先 emit 9 个 spec_diag
//           给前端，**再** 串行跑 executeMockSpec
func planMockSpec(ext, relPath, encoderHint string) mockFileSpec {
	var srcExt string
	switch ext {
	case "mp4", "mkv", "m4a":
		// m4a 复用 source.mp4（已含 aac）-c copy → 实际上 m4a 容器会改用 ipod/mp4 muxer
		srcExt = "mp4"
	case "m4a-lossless":
		// m4a-lossless 用 source.wav + alac 编码（不能 -c copy，因为 source 是 wav 不是 m4a）
		srcExt = "wav"
	case "mp3", "flac":
		srcExt = "wav"
	default:
		return mockFileSpec{
			relativePath: relPath,
			stderr:       fmt.Sprintf("unknown ext: %q", ext),
			exitCode:     -1,
			encoderHint:  encoderHint,
		}
	}

	seq := mockFfmpegSeq.Add(1)
	srcPath := filepath.Join(os.TempDir(), fmt.Sprintf("encv-mock-src-%d-%d.%s", os.Getpid(), seq, srcExt))
	// 🆕 2026-06-12：m4a-lossless 实际输出 .m4a（ext 字段是 m4a-lossless 仅用于编码区分）
	dstExt := ext
	if ext == "m4a-lossless" {
		dstExt = "m4a"
	}
	dstPath := filepath.Join(os.TempDir(), fmt.Sprintf("encv-mock-dst-%d-%d.%s", os.Getpid(), seq, dstExt))

	var encodeArgs []string
	switch ext {
	case "mp4":
		encodeArgs = []string{"-c", "copy"}
	case "mkv":
		encodeArgs = []string{"-c", "copy"}
	case "m4a":
		// 复用 source.mp4 的 aac 流，容器改 m4a（ipod/mp4 muxer）
		encodeArgs = []string{"-c", "copy"}
	case "m4a-lossless":
		// wav → alac → m4a（ffmpeg 内置 alac encoder，无外部依赖）
		encodeArgs = []string{"-c:a", "alac"}
	case "mp3":
		encodeArgs = []string{"-c:a", "libmp3lame", "-b:a", "128k"}
	case "flac":
		encodeArgs = []string{"-c:a", "flac"}
	}
	args := []string{"-i", srcPath}
	args = append(args, encodeArgs...)
	args = append(args, "-y", "-loglevel", "error", dstPath)

	return mockFileSpec{
		relativePath: relPath,
		ffmpegArgs:   args,
		encoderHint:  encoderHint,
		runner:       "ffmpeg", // Phase 3.3 后可换 "mediacodec"
	}
}

// executeMockSpec 真正跑 ffmpeg，**修改入参 sp 的 data/stderr/exitCode 字段**
//
// 🆕 2026-06-12：plan + execute 分离后 handler 阶段调用
//   真机 cgo 阻塞 mp4 时：
//     - 前端**已收到** 9 个 spec_diag（知道流程结构）
//     - handler 在 mp4 这一步阻塞 30s+
//     - 前端 30s abort → fetch reject → catch 块 → inline error card 含
//       "最后收到的 spec_diag = mp4，ffmpegArgs=[-i ... -c copy] 阻塞 30s+"
func executeMockSpec(sp *mockFileSpec) {
	// 0. ffmpeg 可用性
	// 🆕 2026-06-15：ffmpeg.Available() → ffmpeg.IsAvailable()
	ffmpegOk, _, errMsg := ffmpeg.IsAvailable()
	if !ffmpegOk {
		sp.stderr = fmt.Sprintf("ffmpeg not available: %s", errMsg)
		sp.exitCode = -1
		return
	}

	// 1. 解析 ffmpegArgs → 提取 src/dst
	if len(sp.ffmpegArgs) < 6 {
		sp.stderr = fmt.Sprintf("invalid ffmpegArgs (len=%d): %v", len(sp.ffmpegArgs), sp.ffmpegArgs)
		sp.exitCode = -1
		return
	}
	// ffmpegArgs 格式：["-i", srcPath, ...encodeArgs..., "-y", "-loglevel", "error", dstPath]
	srcPath := sp.ffmpegArgs[1]
	dstPath := sp.ffmpegArgs[len(sp.ffmpegArgs)-1]

	// 2. 写 src
	srcExt := filepath.Ext(srcPath)
	var srcBytes []byte
	switch srcExt {
	case ".mp4":
		srcBytes = sourceMP4Bytes
	case ".wav":
		srcBytes = sourceWAVBytes
	default:
		sp.stderr = fmt.Sprintf("unknown src ext: %q", srcExt)
		sp.exitCode = -1
		return
	}
	if werr := os.WriteFile(srcPath, srcBytes, 0644); werr != nil {
		sp.stderr = fmt.Sprintf("write source tmp %s: %v", srcPath, werr)
		sp.exitCode = -1
		return
	}
	defer func() { _ = os.Remove(srcPath) }()
	defer func() { _ = os.Remove(dstPath) }()

	// 3. 跑 ffmpeg
	// 🆕 2026-06-12 Phase 4：timeout 10s → 5s。
	// 5s 上限基于"单次 ffmpeg -c copy / 软编 mp3 30-50MB 输入 < 2s"的实测。
	// 真机 cgo hang 时不再等 30s 才超时；硬上限 5s（worker hard timer + 500ms 兜底）
	// 🆕 修复 B1 (2026-06-17)：预 mkdir worker 专属 tmp dir。
	//   worker 子进程 SELinux 上下文与 Go 父进程不同，自身 5 级 fallback 全部失败。
	//   Go 父进程走 gomobile File API（Java 上下文）创建，SELinux 正确。
	seq := mockFfmpegSeq.Add(1)
	workerTmpDir := filepath.Join(os.TempDir(), fmt.Sprintf("encv_worker_%d_%d", os.Getpid(), seq))
	if mkdirErr := os.MkdirAll(workerTmpDir, 0700); mkdirErr != nil {
		sp.stderr = fmt.Sprintf("mkdir worker tmp %s: %v", workerTmpDir, mkdirErr)
		sp.exitCode = -1
		sp.workerTmpDir = workerTmpDir
		sp.contextInfo = fmt.Sprintf("worker_tmp_dir=%s (mkdir failed: %v) pid=%d", workerTmpDir, mkdirErr, os.Getpid())
		return
	}
	defer func() { _ = os.RemoveAll(workerTmpDir) }()
	sp.workerTmpDir = workerTmpDir

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	// 🆕 修复 B1：用 EncodeWithTmpDir 把 workerTmpDir 传 worker
	res, runErr := ffmpeg.EncodeWithTmpDir(ctx, workerTmpDir, sp.ffmpegArgs...)
	var ffmpegStderr string
	var ffmpegExit int
	if res != nil {
		ffmpegStderr = res.Stderr
		ffmpegExit = res.ExitCode
		if res.WorkerTmpDir != "" {
			sp.workerTmpDir = res.WorkerTmpDir // 用 worker 实际确认的 dir 覆盖（fallback 情况）
		}
		if res.Error != "" {
			sp.workerError = res.Error
		}
	}
	// 🆕 修复 B2：补 lib_dir / tmp_dir / 源/目标文件大小 / worker PID 上下文
	var srcSize, dstSize int64
	if fi, _ := os.Stat(srcPath); fi != nil {
		srcSize = fi.Size()
	}
	if fi, _ := os.Stat(dstPath); fi != nil {
		dstSize = fi.Size()
	}
	sp.srcSize = srcSize
	sp.dstSize = dstSize
	sp.contextInfo = fmt.Sprintf("worker_tmp_dir=%s src=%s(size=%d) dst=%s(size=%d) pid=%d",
		sp.workerTmpDir, srcPath, srcSize, dstPath, dstSize, os.Getpid())

	if runErr != nil {
		// 🆕 2026-06-15：把 worker 响应的 Error 字段也拼到 stderr（之前只拼 Stderr 会丢 ENGINE_LOAD_FAILED 等关键诊断）
		var errDetail string
		if res != nil && res.Error != "" {
			errDetail = "\nworker error: " + res.Error
		}
		// 🆕 修复 B2：context 单独成段，便于前端折叠展示
		sp.stderr = fmt.Sprintf("ffmpeg spawn/run: %v\nstderr: %s%s\ncontext: %s", runErr, ffmpegStderr, errDetail, sp.contextInfo)
		sp.exitCode = ffmpegExit
		return
	}
	if ffmpegExit != 0 {
		// 🆕 修复 B2：同上
		var errDetail string
		if res != nil && res.Error != "" {
			errDetail = "\nworker error: " + res.Error
		}
		sp.stderr = fmt.Sprintf("ffmpeg exit=%d\nstderr: %s%s\ncontext: %s", ffmpegExit, ffmpegStderr, errDetail, sp.contextInfo)
		sp.exitCode = ffmpegExit
		return
	}

	// 4. 读回
	data, rerr := os.ReadFile(dstPath)
	if rerr != nil || len(data) == 0 {
		sp.stderr = fmt.Sprintf("read dst %s: %v (size=%d)\ncontext: %s", dstPath, rerr, len(data), sp.contextInfo)
		sp.exitCode = ffmpegExit
		return
	}
	sp.data = data
	sp.stderr = ffmpegStderr
	sp.exitCode = 0
	// 成功时 sp.contextInfo 保留（前端可展开看「成功路径的环境」）
}

// mockFfmpegSeq 用于 planMockSpec 内部生成唯一 tmp 文件名
// 防止并发请求写入同一 tmp 文件导致 race（10 并发测试时所有 goroutine 同进程 → 同 PID → 冲突）
var mockFfmpegSeq atomic.Uint64

func planMP4() mockFileSpec {
	return planMockSpec("mp4", "01-plain-media/video/sample.mp4", "h264+aac (-c copy)")
}

func planMKV() mockFileSpec {
	return planMockSpec("mkv", "01-plain-media/video/comedy.mkv", "h264+aac (-c copy) → matroska")
}

// 🆕 2026-06-12 m4a 容器 + AAC 有损编码
// 零成本实现：ffmpeg manifest 已有 aac encoder + ipod muxer
// 命令：-i source.mp4 -c:a aac -b:a 128k out.m4a
// 验证：ffprobe out.m4a → "Audio: aac (LC), 44100 Hz, mono" + ISO BMFF container
func planM4A() mockFileSpec {
	return planMockSpec("m4a", "01-plain-media/audio/podcast.m4a", "aac (m4a 容器, 有损)")
}

// 🆕 2026-06-12 m4a 容器 + ALAC 无损编码（Apple Lossless Audio Codec）
// ffmpeg 内置 alac encoder（无需外部库）—— 完全满足"用户：m4a 有无损"
// 命令：-i source.wav -c:a alac out.m4a
// 验证：ffprobe out.m4a → "Audio: alac, 44100 Hz, mono, s16, 1 ch" + ISO BMFF container
func planM4ALossless() mockFileSpec {
	return planMockSpec("m4a-lossless", "01-plain-media/audio/concert.m4a", "alac (m4a 容器, 无损)")
}

func planMP3() mockFileSpec {
	return planMockSpec("mp3", "01-plain-media/audio/music.mp3", "libmp3lame")
}

func planFLAC() mockFileSpec {
	return planMockSpec("flac", "01-plain-media/audio/podcast.flac", "flac")
}

func minimalJPEG() []byte {
	// 来自 scripts/generate-mock-files.ts 1:1 对应
	return []byte{
		0xFF, 0xD8, 0xFF, 0xE0, 0x00, 0x10, 0x4A, 0x46, 0x49, 0x46, 0x00, 0x01,
		0x01, 0x00, 0x00, 0x01, 0x00, 0x01, 0x00, 0x00, 0xFF, 0xD9,
	}
}

func minimalPNG() []byte {
	// 8-byte PNG signature + 简化的 IHDR/IDAT/IEND chunk
	sig := []byte{0x89, 0x50, 0x4E, 0x47, 0x0D, 0x0A, 0x1A, 0x0A}
	// IHDR: 1x1 RGB
	ihdrData := []byte{0x00, 0x00, 0x00, 0x01, 0x00, 0x00, 0x00, 0x01, 0x08, 0x02, 0x00, 0x00, 0x00}
	ihdr := makePngChunk("IHDR", ihdrData)
	// IDAT: 1x1 RGB filter+RGB (filter=0, R=128, G=128, B=128)
	idatData := []byte{0x00, 0x80, 0x80, 0x80}
	idat := makePngChunk("IDAT", idatData)
	// IEND
	iend := makePngChunk("IEND", nil)
	return append(append(append(sig, ihdr...), idat...), iend...)
}

func makePngChunk(typ string, data []byte) []byte {
	out := []byte{
		byte(len(data) >> 24), byte(len(data) >> 16), byte(len(data) >> 8), byte(len(data)),
	}
	out = append(out, []byte(typ)...)
	out = append(out, data...)
	// CRC 占位（PNG 解码器对 CRC 校验在严格模式下会失败，但对我们的 mock 需求可接受）
	crc := pngCrc32(append([]byte(typ), data...))
	out = append(out, byte(crc>>24), byte(crc>>16), byte(crc>>8), byte(crc))
	return out
}

// pngCrc32 是 PNG 用的 CRC-32（与 zip 相同算法）
func pngCrc32(buf []byte) uint32 {
	table := pngCrcTable()
	crc := uint32(0xFFFFFFFF)
	for _, b := range buf {
		crc = table[(crc^uint32(b))&0xFF] ^ (crc >> 8)
	}
	return crc ^ 0xFFFFFFFF
}

var pngCrcCache [256]uint32

func pngCrcTable() [256]uint32 {
	for i := range pngCrcCache {
		c := uint32(i)
		for j := 0; j < 8; j++ {
			if c&1 != 0 {
				c = 0xEDB88320 ^ (c >> 1)
			} else {
				c = c >> 1
			}
		}
		pngCrcCache[i] = c
	}
	return pngCrcCache
}

func minimalPDF() []byte {
	return []byte("%PDF-1.4\n1 0 obj\n<< /Type /Catalog >>\nendobj\n%%EOF\n")
}

func makeAEFile(name string, targetSize int) []byte {
	// AENC magic + name + padding
	header := []byte{'A', 'E', 'N', 'C', 0x01, 0x00, byte(len(name))}
	header = append(header, []byte(name)...)
	header = append(header, 0x00)
	out := make([]byte, targetSize)
	copy(out, header)
	return out
}

func makeSCCVFile(name, ext string, targetSize int) []byte {
	manifest := fmt.Sprintf(`{"version":"4.0","originalName":%q,"originalExt":%q,"algorithm":"aes-256-gcm","createdAt":"2026-01-01T00:00:00Z","entries":[{"type":"file","name":%q,"size":%d}]}`, name, ext, name, targetSize-256)
	header := []byte{'S', 'C', 'C', 'V', 0x04, 0x01, 0x00, 0x00, 0x00, 0x00, 0x00, 0x20}
	header = append(header, []byte(manifest)...)
	out := make([]byte, targetSize)
	copy(out, header)
	return out
}

func makeBytes(n int, fill byte) []byte {
	out := make([]byte, n)
	for i := range out {
		out[i] = fill
	}
	return out
}

// ════════════════════════════════════════════════════════════════════
// 📦 归档：旧 mockRootAllowList（2026-06-15 multi-mount 重构删除）
//
//   旧实现是静态白名单绝对路径前缀匹配，2026-06-10 改造删除 dev 模式相对路径。
//   重构原因（spec §1.2）：
//     1. 硬编码 /storage/emulated/0 → 多用户 mode 下变成 /storage/emulated/<uid>/0
//     2. /storage/emulated/0/encv-automation → Android 11+ scoped storage EACCES
//     3. dev 沙箱里写 mock 跟真机路径不一致 → 自动化测试找不到
//
//   新实现走 mount.MountRegistry：root 必须是 /d/<mount>/... 形式，
//   通过 Resolve() 动态解析到 abs 路径（真机 /data/user/<uid>/com.encvgo.app/files/encv-automation）
//
// 保留这段仅供历史查阅，**不要**在新代码里引用。
// ════════════════════════════════════════════════════════════════════
// var mockRootAllowList = []string{
// 	"/storage/emulated/0",
// 	"/storage/emulated/0/encv-automation",
// 	"/sdcard/encv-automation",
// 	"/data/local/tmp/encv-automation",
// }
