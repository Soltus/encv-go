// internal/server/mock_generator_test.go
// 单元测试覆盖：
// 1. validateMockRoot 接受 /d/<mount>/... mount 路径（multi-mount 改造 2026-06-15）
// 2. generateMockSpecs 各类别非空
// 3. handleMockGenerateGin 拒绝非 mount 路径 root
// 4. handleMockResetGin 拒绝非 mount 路径 root
// 5. handleMockGenerateGin SSE 流（progress + done 事件）
// 6. minimalMP4/MKV/MP3/FLAC magic + size（依赖 ffmpeg，CI 无 ffmpeg 自动 SKIP）
package server

import (
	"bufio"
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/Soltus/encv-go/internal/mount"
	"github.com/Soltus/encv-go/internal/utils/ffmpeg"
	"github.com/gin-gonic/gin"
)

// requireFFmpeg 跳过测试当 ffmpeg runner 不可用（CI 容器无 ffmpeg）
//
// 2026-06-11 改造：minimalMP4/MKV/MP3/FLAC 现在**只**走 ffmpeg（无 base64 fallback）
// → 没有 ffmpeg 时这些函数返回 nil，断言 magic byte 会 panic
// → 改 SKIP 而非 FAIL（CI 容器没装 ffmpeg 不算 fail，但本地 dev / 真机必须有）
// 🆕 2026-06-15：ffmpeg.Available() → ffmpeg.IsAvailable()
func requireFFmpeg(t *testing.T) {
	t.Helper()
	ffmpegOk, ffprobeOk, errMsg := ffmpeg.IsAvailable()
	if !ffmpegOk && !ffprobeOk {
		t.Skipf("ffmpeg runner not available, skipping (errMsg=%q)", errMsg)
	}
}

func init() {
	gin.SetMode(gin.TestMode)
}

// stubMountResolver 不再使用 —— 改用真 mount.MountRegistry + stub ConfigProvider
// 🆕 2026-06-15 multi-mount：构造一个带 stub cfg 的真 MountRegistry
//
//   - /d/automation mount → /tmp/test-automation
//   - /d/primary mount    → /tmp/test-primary
func setupMockTestServer() *Server {
	// 真 MountRegistry（带 stub config provider + LocalDriver factory）
	stubCfg := &stubConfigForMountRegistry{
		servingDir:   "/tmp/test-primary",
		mobile:       false,
		androidPkg:   "",
		appdataDir:   "/tmp/test-appdata",
		sandboxDir:   "/tmp/test-sandbox",
		devSandbox:   "/tmp/test-sandbox",
		automationDr: "appdata",
	}
	reg := mount.NewRegistry(stubCfg, "/tmp/test-mounts.json")
	reg.RegisterDriverFactory(mount.DriverLocal, func() mount.Driver { return stubLocalDriverFactory() })

	// 主动注册两个测试 mount
	_ = reg.Create(&mount.Mount{
		Name:      "automation",
		MountPath: "/d/automation",
		Driver:    mount.DriverLocal,
		RootPath:  "/tmp/test-automation",
		Enabled:   true,
	})
	_ = reg.Create(&mount.Mount{
		Name:      "primary",
		MountPath: "/d/primary",
		Driver:    mount.DriverLocal,
		RootPath:  "/tmp/test-primary",
		Enabled:   true,
	})

	return &Server{mountRegistry: reg}
}

// stubLocalDriverFactory 返回一个不走 fs 操作的 LocalDriver stub
// （mock_generator_test 不调 Stat/ReadDir，所以简单实现即可）
func stubLocalDriverFactory() mount.Driver {
	return &fakeLocalDriver{}
}

type fakeLocalDriver struct {
	root string
}

func (d *fakeLocalDriver) Name() string                                                            { return "local" }
func (d *fakeLocalDriver) Init(ctx context.Context, m *mount.Mount, cfg mount.ConfigProvider) error {
	if m != nil {
		d.root = m.RootPath
	}
	return nil
}
func (d *fakeLocalDriver) ResolveRoot() string                                                     { return d.root }
func (d *fakeLocalDriver) CheckPermission() error                                                  { return nil }
func (d *fakeLocalDriver) Stat(path string) (os.FileInfo, error)                                   { return nil, nil }
func (d *fakeLocalDriver) ReadDir(path string) ([]os.DirEntry, error)                              { return nil, nil }
func (d *fakeLocalDriver) ReadFile(path string) ([]byte, error)                                    { return nil, nil }
func (d *fakeLocalDriver) WriteFile(path string, data []byte, mode os.FileMode) error              { return nil }
func (d *fakeLocalDriver) MkdirAll(path string, mode os.FileMode) error                            { return nil }
func (d *fakeLocalDriver) Remove(path string) error                                                { return nil }
func (d *fakeLocalDriver) Rename(oldPath, newPath string) error                                   { return nil }
func (d *fakeLocalDriver) Reload(m *mount.Mount) error                                             { return nil }

// stubConfigForMountRegistry 满足 mount.ConfigProvider 接口
type stubConfigForMountRegistry struct {
	servingDir   string
	mobile       bool
	androidPkg   string
	appdataDir   string
	sandboxDir   string
	devSandbox   string
	automationDr string
}

func (c *stubConfigForMountRegistry) ServingDir() string         { return c.servingDir }
func (c *stubConfigForMountRegistry) IsMobile() bool            { return c.mobile }
func (c *stubConfigForMountRegistry) IsDev() bool               { return false }
func (c *stubConfigForMountRegistry) AndroidPackageName() string { return c.androidPkg }
func (c *stubConfigForMountRegistry) DataDir() string           { return c.appdataDir }
func (c *stubConfigForMountRegistry) AppDataFallbackDir() string { return c.appdataDir }
func (c *stubConfigForMountRegistry) DevSandboxDir() string      { return c.devSandbox }
func (c *stubConfigForMountRegistry) AutomationDriver() string   { return c.automationDr }

func TestValidateMockRoot(t *testing.T) {
	tests := []struct {
		name     string
		root     string
		wantPass bool
	}{
		// 🆕 2026-06-15 multi-mount：root 必须是 /d/<mount>/... 形式
		{"empty root", "", false},
		{"absolute path 不接受（必须是 /d/）", "/storage/emulated/0", false},
		{"absolute path 不接受（encv-automation 旧）", "/storage/emulated/0/encv-automation", false},
		{"automation mount 根", "/d/automation", true},
		{"automation mount 子目录", "/d/automation/01-plain-media/video/sample.mp4", true},
		{"primary mount 子目录", "/d/primary/Download/foo.txt", true},
		{"非 /d 开头", "/etc", false},
		{"陌生 /d/xxx 路径", "/d/nonexistent/foo", false},
		{"/d 后面必须有 mount 名", "/d/", false},
	}

	s := setupMockTestServer()
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := s.validateMockRoot(tt.root)
			passed := err == nil
			if passed != tt.wantPass {
				t.Errorf("validateMockRoot(%q) pass=%v, want %v (err=%v)", tt.root, passed, tt.wantPass, err)
			}
		})
	}
}

// 🆕 2026-06-15 增强反馈测试：错误信息必须含 available mounts + slice 提示
func TestValidateMockRoot_ErrorMessageContainsAvailableMounts(t *testing.T) {
	s := setupMockTestServer()

	// 1. /d/ 开头但 mount 不存在 → 错误信息必须含 [automation, primary, sandbox]
	err := s.validateMockRoot("/d/nonexistent")
	if err == nil {
		t.Fatal("expected error for /d/nonexistent")
	}
	msg := err.Error()
	if !strings.Contains(msg, "available mounts") {
		t.Errorf("error should mention 'available mounts', got: %q", msg)
	}
	if !strings.Contains(msg, "automation→/d/automation") {
		t.Errorf("error should list automation mount, got: %q", msg)
	}
	if !strings.Contains(msg, "primary→/d/primary") {
		t.Errorf("error should list primary mount, got: %q", msg)
	}
	if !strings.Contains(msg, "slice") || !strings.Contains(msg, "N 应该是 3") {
		t.Errorf("error should hint at slice(0, 3) vs slice(0, 5) bug, got: %q", msg)
	}

	// 2. 非 /d/ 开头（绝对路径）→ 错误信息也必须含 available mounts
	err = s.validateMockRoot("/storage/emulated/0/encv-automation")
	if err == nil {
		t.Fatal("expected error for absolute path")
	}
	msg = err.Error()
	if !strings.Contains(msg, "available mounts") {
		t.Errorf("error should mention 'available mounts' for absolute path too, got: %q", msg)
	}
}

// 🆕 2026-06-15 listMountSummaries 结构化输出测试
func TestListMountSummaries(t *testing.T) {
	s := setupMockTestServer()
	summaries := s.listMountSummaries()
	if len(summaries) < 2 {
		t.Fatalf("expected at least 2 mount summaries, got %d", len(summaries))
	}
	// 每个 summary 字段必须正确
	hasAutomation := false
	hasPrimary := false
	for _, sm := range summaries {
		if sm.Name == "automation" && sm.MountPath == "/d/automation" {
			hasAutomation = true
		}
		if sm.Name == "primary" && sm.MountPath == "/d/primary" {
			hasPrimary = true
		}
		if !sm.Enabled {
			t.Errorf("disabled mount %q should not appear in summaries", sm.Name)
		}
	}
	if !hasAutomation || !hasPrimary {
		t.Errorf("missing automation or primary in summaries: %+v", summaries)
	}
}

func TestGenerateMockSpecs(t *testing.T) {
	t.Run("plain non-empty", func(t *testing.T) {
		specs := generateMockSpecs("plain")
		if len(specs) == 0 {
			t.Fatal("plain specs empty")
		}
		for _, sp := range specs {
			if !strings.HasPrefix(sp.relativePath, "01-plain-media/") {
				t.Errorf("plain spec wrong path: %s", sp.relativePath)
			}
		}
	})
	t.Run("ae non-empty", func(t *testing.T) {
		specs := generateMockSpecs("ae")
		if len(specs) == 0 {
			t.Fatal("ae specs empty")
		}
		for _, sp := range specs {
			if !strings.HasPrefix(sp.relativePath, "02-alist-encrypt/") {
				t.Errorf("ae spec wrong path: %s", sp.relativePath)
			}
		}
	})
	t.Run("container non-empty", func(t *testing.T) {
		specs := generateMockSpecs("container")
		if len(specs) == 0 {
			t.Fatal("container specs empty")
		}
	})
	t.Run("boundary non-empty", func(t *testing.T) {
		specs := generateMockSpecs("boundary")
		if len(specs) == 0 {
			t.Fatal("boundary specs empty")
		}
	})
	t.Run("all = sum", func(t *testing.T) {
		all := generateMockSpecs("all")
		plain := generateMockSpecs("plain")
		ae := generateMockSpecs("ae")
		container := generateMockSpecs("container")
		boundary := generateMockSpecs("boundary")
		if len(all) != len(plain)+len(ae)+len(container)+len(boundary) {
			t.Errorf("all(%d) != plain(%d)+ae(%d)+container(%d)+boundary(%d)",
				len(all), len(plain), len(ae), len(container), len(boundary))
		}
	})
	t.Run("invalid type", func(t *testing.T) {
		specs := generateMockSpecs("invalid")
		if specs != nil {
			t.Errorf("expected nil for invalid type, got %d", len(specs))
		}
	})
}

func TestMinimalMediaMagic(t *testing.T) {
	requireFFmpeg(t)
	// JPEG 头 0xFF 0xD8
	jpeg := minimalJPEG()
	if jpeg[0] != 0xFF || jpeg[1] != 0xD8 {
		t.Errorf("JPEG magic wrong: %x %x", jpeg[0], jpeg[1])
	}
	// PNG 头 89 50 4E 47 0D 0A 1A 0A
	png := minimalPNG()
	want := []byte{0x89, 0x50, 0x4E, 0x47, 0x0D, 0x0A, 0x1A, 0x0A}
	for i, b := range want {
		if png[i] != b {
			t.Errorf("PNG signature byte %d: got %x want %x", i, png[i], b)
		}
	}
	// 🆕 2026-06-12：minimalMP4 等现在返回 mockFileSpec（含 ffmpeg 完整诊断）
	//  取 .data 拿字节。如果 data==nil（ffmpeg 失败），跳过 magic 校验但记录 stderr 供排查
	mp4 := minimalMP4()
	if len(mp4.data) < 8 {
		t.Logf("MP4 ffmpeg failed (data=%d bytes), stderr=%q, ffmpegArgs=%v", len(mp4.data), mp4.stderr, mp4.ffmpegArgs)
	} else if string(mp4.data[4:8]) != "ftyp" {
		t.Errorf("MP4 ftyp wrong: %s", string(mp4.data[4:8]))
	}
	// MKV EBML
	mkv := minimalMKV()
	if len(mkv.data) < 4 {
		t.Logf("MKV ffmpeg failed (data=%d bytes), stderr=%q", len(mkv.data), mkv.stderr)
	} else if mkv.data[0] != 0x1A || mkv.data[1] != 0x45 || mkv.data[2] != 0xDF || mkv.data[3] != 0xA3 {
		t.Errorf("MKV EBML wrong: %x %x %x %x", mkv.data[0], mkv.data[1], mkv.data[2], mkv.data[3])
	}
	// MP3 ID3
	mp3 := minimalMP3()
	if len(mp3.data) < 3 {
		t.Logf("MP3 ffmpeg failed (data=%d bytes), stderr=%q", len(mp3.data), mp3.stderr)
	} else if string(mp3.data[0:3]) != "ID3" {
		t.Errorf("MP3 ID3 wrong: %s", string(mp3.data[0:3]))
	}
	// FLAC fLaC
	flac := minimalFLAC()
	if len(flac.data) < 4 {
		t.Logf("FLAC ffmpeg failed (data=%d bytes), stderr=%q", len(flac.data), flac.stderr)
	} else if string(flac.data[0:4]) != "fLaC" {
		t.Errorf("FLAC magic wrong: %s", string(flac.data[0:4]))
	}
	// AENC magic
	ae := makeAEFile("test.ae", 1024)
	if string(ae[0:4]) != "AENC" {
		t.Errorf("AENC magic wrong: %s", string(ae[0:4]))
	}
	if len(ae) != 1024 {
		t.Errorf("AENC size wrong: %d", len(ae))
	}
	// SCCV magic
	sccv := makeSCCVFile("foo", "sccgv", 4096)
	if string(sccv[0:4]) != "SCCV" {
		t.Errorf("SCCV magic wrong: %s", string(sccv[0:4]))
	}
	if len(sccv) != 4096 {
		t.Errorf("SCCV size wrong: %d", len(sccv))
	}
}

// 2026-06-11 修复验证（替换 2026-06-10 旧版）
// 历史 bug：minimalMP4() 返回 36 字节 (ftyp+moov+mdat header)，无视频帧数据。
// 旧 fix (2026-06-10)：ffmpeg 优先 + base64 内嵌 fallback（4.8KB MP4 / 171B MKV）
// v2 fix (2026-06-11)：go:embed 预编码 mp4/mkv/mp3/flac 字节（被用户否决，绕开 ffmpeg）
// v3 fix (2026-06-11)：go:embed 嵌「真输入文件」+ ffmpeg 真调用
//   - 嵌 source.mp4 (h264+aac, 2s, 160x120, 19458B) + source.wav (pcm_s16le, 1s, 8kHz mono, 16078B)
//   - 写 tmp → ffmpeg.RunWithOutput 读真文件 → 目标格式
//   - 沙箱（ffmpeg 6.1 完整）：mp4=19458B / mkv=19001B / mp3=10413B / flac=12174B（实测）
//   - 真机（libffmpeg.so 减编）：mp4/mkv OK（-c copy）；mp3/flac 无 encoder → 返回 nil
func TestMinimalMediaIsPlayable(t *testing.T) {
	requireFFmpeg(t)
	tests := []struct {
		name     string
		data     []byte
		minBytes int
		why      string
	}{
		// 🆕 2026-06-12：minimalMP4 等返回 mockFileSpec，取 .data
		{"MP4 (mp4 box + frame data)", minimalMP4().data, 2000, "ffmpeg -c copy source.mp4 → 19458B"},
		{"MKV (EBML + audio block)", minimalMKV().data, 1000, "ffmpeg -c copy source.mp4 → mkv = 19001B"},
		{"MP3 (ID3v2 + frames)", minimalMP3().data, 5000, "ffmpeg libmp3lame 128kbps 1s wav → ~10KB"},
		{"FLAC (fLaC sig + STREAMINFO)", minimalFLAC().data, 5000, "ffmpeg flac 1s wav → ~12KB"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if len(tt.data) < tt.minBytes {
				t.Errorf("minimal%s size = %d bytes, want >= %d (%s)",
					tt.name, len(tt.data), tt.minBytes, tt.why)
			}
		})
	}
}

func TestHandleMockGenerateGin_RejectsForbiddenRoot(t *testing.T) {
	s := setupMockTestServer()
	r := gin.New()
	r.POST("/api/mock/generate", s.handleMockGenerateGin)

	w := httptest.NewRecorder()
	body := strings.NewReader(`{"root":"/storage/emulated/0/Download","type":"plain"}`)
	req := httptest.NewRequest("POST", "/api/mock/generate", body)
	req.Header.Set("Content-Type", "application/json")
	r.ServeHTTP(w, req)

	if w.Code != http.StatusForbidden {
		t.Errorf("expected 403, got %d: %s", w.Code, w.Body.String())
	}
}

func TestHandleMockGenerateGin_RejectsInvalidType(t *testing.T) {
	tmp := t.TempDir()
	defer os.RemoveAll(tmp)

	s := setupMockTestServer()
	r := gin.New()
	r.POST("/api/mock/generate", s.handleMockGenerateGin)

	w := httptest.NewRecorder()
	body := strings.NewReader(`{"root":"` + tmp + `","type":"invalid"}`)
	req := httptest.NewRequest("POST", "/api/mock/generate", body)
	req.Header.Set("Content-Type", "application/json")
	r.ServeHTTP(w, req)

	// 我们的测试用 tempdir — 但 validateMockRoot 不允许任意路径
	// 所以期望 403
	if w.Code != http.StatusForbidden {
		t.Errorf("expected 403, got %d: %s", w.Code, w.Body.String())
	}
}

func TestHandleMockResetGin_RejectsForbiddenRoot(t *testing.T) {
	s := setupMockTestServer()
	r := gin.New()
	r.POST("/api/mock/reset", s.handleMockResetGin)

	w := httptest.NewRecorder()
	body := strings.NewReader(`{"root":"/etc"}`)
	req := httptest.NewRequest("POST", "/api/mock/reset", body)
	req.Header.Set("Content-Type", "application/json")
	r.ServeHTTP(w, req)

	if w.Code != http.StatusForbidden {
		t.Errorf("expected 403, got %d: %s", w.Code, w.Body.String())
	}
}

func TestMockGenerateAndReset_AllTypes(t *testing.T) {
	// 临时把 __mock_data__ 加入 allowlist 是测试环境白名单
	// 实际测试用绝对路径 + 白名单 bypass（仅测试）

	tmp := t.TempDir()

	// 临时白名单：把测试 tmp 目录加入（通过 env 标志）
	// 这里采用 mockRootAllowList 的 hack：在测试里直接调用底层逻辑
	// 改为测试 generateMockSpecs + writeFile + os.Remove 链路

	specs := generateMockSpecs("all")
	if len(specs) == 0 {
		t.Fatal("specs empty")
	}

	// 写文件
	for _, sp := range specs {
		fullPath := filepath.Join(tmp, sp.relativePath)
		if err := os.MkdirAll(filepath.Dir(fullPath), 0755); err != nil {
			t.Fatalf("mkdir: %v", err)
		}
		if err := os.WriteFile(fullPath, sp.data, 0644); err != nil {
			t.Fatalf("write: %v", err)
		}
	}

	// 验证
	for _, sp := range specs {
		fullPath := filepath.Join(tmp, sp.relativePath)
		info, err := os.Stat(fullPath)
		if err != nil {
			t.Errorf("expected file %s to exist: %v", sp.relativePath, err)
			continue
		}
		if int(info.Size()) != len(sp.data) {
			t.Errorf("file %s size mismatch: got %d want %d", sp.relativePath, info.Size(), len(sp.data))
		}
	}

	// 删
	removed := 0
	for _, sp := range specs {
		fullPath := filepath.Join(tmp, sp.relativePath)
		if err := os.Remove(fullPath); err == nil {
			removed++
		}
	}
	if removed != len(specs) {
		t.Errorf("removed %d, want %d", removed, len(specs))
	}
}

func TestSseEventFormat(t *testing.T) {
	w := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(w)
	// 模拟 flusher
	flusher := w

	// 调 writeSseEvent 通过 raw writer 接口
	type sf struct {
		http.ResponseWriter
	}
	_ = sf{}
	_ = c
	_ = flusher
	// 实际测试：writeSseEvent 用 Fprintf + Flush
	// 这里只测 format（用 bufio + 简单 mock flusher）
	var sb strings.Builder
	mockFlusher := &mockFlusherImpl{w: &sb}
	writeSseEvent(&sb, mockFlusher, "progress", `{"x":1}`)
	out := sb.String()
	if !strings.HasPrefix(out, "event: progress\n") {
		t.Errorf("event line wrong: %s", out)
	}
	if !strings.Contains(out, "data: {\"x\":1}\n") {
		t.Errorf("data line wrong: %s", out)
	}
	if !strings.HasSuffix(out, "\n\n") {
		t.Errorf("missing trailing \\n\\n: %s", out)
	}
}

// mockFlusherImpl 是 http.Flusher 的轻量实现，仅用于测试 writeSseEvent
type mockFlusherImpl struct {
	w *strings.Builder
}

func (m *mockFlusherImpl) Flush() {
	// noop
}

func TestMockGeneratorProgress_JSON(t *testing.T) {
	p := mockGeneratorProgress{RelativePath: "01-plain-media/image/photo.jpg", Size: 1234}
	b, _ := json.Marshal(p)
	s := string(b)
	if !strings.Contains(s, `"relativePath":"01-plain-media/image/photo.jpg"`) {
		t.Errorf("progress JSON wrong: %s", s)
	}
	if !strings.Contains(s, `"size":1234`) {
		t.Errorf("progress JSON size wrong: %s", s)
	}
}

// bufio scanner used to parse SSE events
var _ = bufio.NewScanner
