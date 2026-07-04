// internal/server/agent_fs_bridge.go
//
// 把"本地挂载的文件系统"接入 AI agent 工具系统。
//
// 设计目标：
//   - 让 LLM 真实地"感知"encv-go 暴露的本地文件系统（servingDir / webdavDir），
//     不依赖任何外部服务（OpenList 之类的已被砍掉）。
//   - 提供 5 个只读工具：list_mounts / list_files / read_file / stat_file / get_storage_info。
//   - 所有路径都强制走 utils.SafeResolveToAbsPath 沙箱化，
//     严禁让 LLM 通过 "../" 之类的输入逃出 servingDir。
//   - 只读工具全部 needConfirm=false（高危副作用是加密/解密那一类，
//     文件系统读取在用户信任模型里等同于"ls"——见 agent_plugin_bridge.go 的设计）。
//
// 不做的事（明确范围）：
//   - 不做写入工具（write_file / delete_file）。当前 AI agent 的产品定位是
//     "帮用户浏览和决策"，不是"自动修改用户文件"。写入工具等后续 phase 加。
//   - 不做跨挂载点的搜索（find / grep）。本轮只暴露"列举"和"读取"。
//   - 不实现 virtual view（容器内文件透明可见）。list_files 只看物理文件。
//     如需看 .encv 容器内部，由专门插件工具负责，agent 通过 fileChange 流转。
package server

import (
	"context"
	"encoding/json"
	"fmt"
	"io/fs"
	"log/slog"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"syscall"
	"time"

	"github.com/Soltus/encv-go/internal/utils"
)

// ─── 挂载点结构 ────────────────────────────────────────────────

// mountInfo 描述一个 agent 可访问的挂载点。LLM 通过 list_mounts 拿到这个列表，
// 后续所有 list_files / read_file / stat_file 都必须传 "mount_id" +
// "rel_path"（挂载点内的相对路径），以避免误用其它挂载点。
//
// 字段：
//   - ID         挂载点 ID（"serving" / "webdav"，由 ListFSMounts 生成）
//   - Type       类型：serving（主服务根）/ webdav（WebDAV 根）/ path（物理路径）
//   - Root       物理绝对路径（不返回给 LLM；只用于 list_files 的服务端解析）
//   - PublicPath 公开路径标识（"/" or "/webdav"），LLM 可见
//   - Description 用户可读说明（中文，因为面向国内用户）
//   - Available  当前是否可用（false 表示配置了但目录被删了 / 不可读）
type mountInfo struct {
	ID          string `json:"id"`
	Type        string `json:"type"`
	PublicPath  string `json:"public_path"`
	Description string `json:"description"`
	Available   bool   `json:"available"`
}

// ListFSMounts 收集所有 agent 可访问的挂载点。
// 调用方传入 Server 实例（拿到 servingDir / webdavDir 配置）。
// 返回的 list 已经过滤掉不存在 / 不可读的挂载点，但保留 Available 标记
// 让 LLM 知道"配了但当前不可用"的差异。
func (s *Server) ListFSMounts() []mountInfo {
	now := time.Now()
	_ = now
	out := make([]mountInfo, 0, 2)

	if s.servingDir != "" {
		out = append(out, mountInfo{
			ID:          "serving",
			Type:        "serving",
			PublicPath:  "/",
			Description: "主服务根目录（HTTP/WebDAV 共同根）",
			Available:   dirReadable(s.servingDir),
		})
	}
	if s.webdavDir != "" && s.webdavDir != s.servingDir {
		// 公开路径从 webdavPath 推（如果配了 /webdav 前缀）
		public := "/webdav"
		if s.webdavPath != "" {
			public = s.webdavPath
		}
		out = append(out, mountInfo{
			ID:          "webdav",
			Type:        "webdav",
			PublicPath:  public,
			Description: "WebDAV 独立根（可能与 serving 不同）",
			Available:   dirReadable(s.webdavDir),
		})
	}
	return out
}

// resolveMount 根据 mountID 找到对应的物理根路径。
// 找不到或者不可用 → 返回空 + false。
func (s *Server) resolveMount(mountID string) (string, bool) {
	for _, m := range s.ListFSMounts() {
		if m.ID == mountID && m.Available {
			if m.Type == "serving" {
				return s.servingDir, true
			}
			if m.Type == "webdav" {
				return s.webdavDir, true
			}
		}
	}
	return "", false
}

// dirReadable 快速检查目录是否可读
func dirReadable(p string) bool {
	if p == "" {
		return false
	}
	info, err := os.Stat(p)
	if err != nil || !info.IsDir() {
		return false
	}
	// 尝试打开一次
	f, err := os.Open(p)
	if err != nil {
		return false
	}
	f.Close()
	return true
}

// ─── 工具 schema ──────────────────────────────────────────────

// 所有 fs 工具都遵循统一签名：{ "mount_id": "serving", "rel_path": "/foo/bar" }
// mount_id 必填；rel_path 可省（默认 "/"）。

var fsToolListMountsSchema = map[string]interface{}{
	"type":       "object",
	"properties": map[string]interface{}{},
	"required":   []string{},
}

var fsToolListFilesSchema = map[string]interface{}{
	"type": "object",
	"properties": map[string]interface{}{
		"mount_id": map[string]interface{}{
			"type":        "string",
			"enum":        []string{"serving", "webdav"},
			"description": "挂载点 ID（先调 list_mounts 拿到可用列表）",
		},
		"rel_path": map[string]interface{}{
			"type":        "string",
			"description": "挂载点内的相对路径，默认 '/' 表示根",
		},
		"max_entries": map[string]interface{}{
			"type":        "integer",
			"description": "最多返回多少条（默认 200，防止 LLM 拖出整个磁盘目录树）",
			"default":     200,
			"minimum":     1,
			"maximum":     1000,
		},
	},
	"required": []string{"mount_id"},
}

var fsToolReadFileSchema = map[string]interface{}{
	"type": "object",
	"properties": map[string]interface{}{
		"mount_id": map[string]interface{}{
			"type":        "string",
			"enum":        []string{"serving", "webdav"},
			"description": "挂载点 ID",
		},
		"rel_path": map[string]interface{}{
			"type":        "string",
			"description": "要读取的文件相对路径（必须指向文件，不能是目录）",
		},
		"max_bytes": map[string]interface{}{
			"type":        "integer",
			"description": "最多读取多少字节（默认 64 KiB，防止误读大文件把上下文塞爆）",
			"default":     65536,
			"minimum":     1,
			"maximum":     1048576,
		},
	},
	"required": []string{"mount_id", "rel_path"},
}

var fsToolStatFileSchema = map[string]interface{}{
	"type": "object",
	"properties": map[string]interface{}{
		"mount_id": map[string]interface{}{
			"type":        "string",
			"enum":        []string{"serving", "webdav"},
			"description": "挂载点 ID",
		},
		"rel_path": map[string]interface{}{
			"type":        "string",
			"description": "要查询的文件/目录相对路径",
		},
	},
	"required": []string{"mount_id", "rel_path"},
}

var fsToolGetStorageInfoSchema = map[string]interface{}{
	"type":       "object",
	"properties": map[string]interface{}{},
	"required":   []string{},
}

// ListFSTools 返回所有 fs 工具的元信息（name + description + parameters + needConfirm）。
// 所有 fs 工具都是 needConfirm=false（只读，类比"ls"），具体行为见各 handler 注释。
func (s *Server) ListFSTools() []map[string]interface{} {
	mounts := s.ListFSMounts()
	mountCount := len(mounts)

	// 动态描述里把可用的挂载点列出来，让 LLM 一看就懂
	mountDesc := mountCountString(mounts)

	return []map[string]interface{}{
		{
			"name": "list_mounts",
			"description": fmt.Sprintf(
				"列出 AI agent 当前可访问的本地挂载的文件系统（共 %d 个：%s）。"+
					"调用任何 list_files / read_file / stat_file 前都应先调这个工具拿到 mount_id。",
				mountCount, mountDesc,
			),
			"parameters":  fsToolListMountsSchema,
			"needConfirm": false,
			"kind":        "fileRead",
		},
		{
			"name":        "list_files",
			"description": "列出挂载点内某个目录下的文件与子目录。返回 is_dir / size / modified 等元信息。",
			"parameters":  fsToolListFilesSchema,
			"needConfirm": false,
			"kind":        "fileRead",
		},
		{
			"name":        "read_file",
			"description": "读取挂载点内某个文本文件的内容（默认上限 64 KiB，二进制文件会返回占位提示）。",
			"parameters":  fsToolReadFileSchema,
			"needConfirm": false,
			"kind":        "fileRead",
		},
		{
			"name":        "stat_file",
			"description": "查询挂载点内某个文件或目录的元信息（大小 / 修改时间 / 是否是目录 / 是否是容器）。",
			"parameters":  fsToolStatFileSchema,
			"needConfirm": false,
			"kind":        "fileRead",
		},
		{
			"name":        "get_storage_info",
			"description": "返回 encv-go 各挂载点所在物理磁盘的总容量 / 已用 / 剩余字节数（基于 statfs）。",
			"parameters":  fsToolGetStorageInfoSchema,
			"needConfirm": false,
			"kind":        "fileRead",
		},
	}
}

// mountCountString 把 mount 列表格式化成"id(public_path)"逗号串。
// mountCount=0 时返回 "<no mounts available>" 防止空描述误导 LLM。
func mountCountString(mounts []mountInfo) string {
	if len(mounts) == 0 {
		return "<no mounts available>"
	}
	parts := make([]string, 0, len(mounts))
	for _, m := range mounts {
		parts = append(parts, fmt.Sprintf("%s(%s)", m.ID, m.PublicPath))
	}
	return strings.Join(parts, ", ")
}

// ─── 工具执行 ────────────────────────────────────────────────

// executeFSTool 派发 fs 工具调用。
// 与 executePluginTool 同一层级：都通过 executeAgentTool 路由，
// 然后各自委派到 fs / plugin 实现。
//
// 返回 (outputJSON, error)。错误也包成 errJSON（与 plugin 路径一致），
// 这样 LLM 看到的 tool_result 是结构化的 {"error": code, "message": msg}。
func (s *Server) executeFSTool(ctx context.Context, toolName, argsJSON string) (string, error) {
	switch toolName {
	case "list_mounts":
		return s.fsListMounts(argsJSON)
	case "list_files":
		return s.fsListFiles(argsJSON)
	case "read_file":
		return s.fsReadFile(argsJSON)
	case "stat_file":
		return s.fsStatFile(argsJSON)
	case "get_storage_info":
		return s.fsGetStorageInfo(argsJSON)
	default:
		return "", fmt.Errorf("unknown fs tool: %s", toolName)
	}
}

// ─── fs tool 实现 ─────────────────────────────────────────────

func (s *Server) fsListMounts(argsJSON string) (string, error) {
	// 不需要任何参数，但为了健壮性 parse 一下
	var args struct{}
	_ = json.Unmarshal([]byte(argsJSON), &args)

	mounts := s.ListFSMounts()
	out := map[string]interface{}{
		"count": len(mounts),
		"items": mounts,
		"note":  "接下来调用 list_files / read_file / stat_file 时，用 mount_id 字段指向上面 items[].id",
		"server": map[string]string{
			"goos": runtime.GOOS,
		},
	}
	return okJSON(out), nil
}

func (s *Server) fsListFiles(argsJSON string) (string, error) {
	var args struct {
		MountID    string `json:"mount_id"`
		RelPath    string `json:"rel_path"`
		MaxEntries int    `json:"max_entries"`
	}
	if err := json.Unmarshal([]byte(argsJSON), &args); err != nil {
		return errJSON("invalid_args", err.Error()), nil
	}
	if args.MountID == "" {
		return errJSON("missing_args", "mount_id 必填（先调 list_mounts）"), nil
	}
	if args.RelPath == "" {
		args.RelPath = "/"
	}
	if args.MaxEntries <= 0 {
		args.MaxEntries = 200
	}
	if args.MaxEntries > 1000 {
		args.MaxEntries = 1000
	}

	root, ok := s.resolveMount(args.MountID)
	if !ok {
		return errJSON("mount_unavailable", "挂载点 "+args.MountID+" 不存在或不可读"), nil
	}
	abs, err := utils.SafeResolveToAbsPath(root, args.RelPath)
	if err != nil {
		return errJSON("path_forbidden", err.Error()), nil
	}
	entries, err := os.ReadDir(abs)
	if err != nil {
		return errJSON("readdir_failed", err.Error()), nil
	}

	items := make([]map[string]interface{}, 0, len(entries))
	truncated := false
	for i, e := range entries {
		if i >= args.MaxEntries {
			truncated = true
			break
		}
		// 跳过隐藏文件（与 mobile_service.ListFiles 行为一致）
		if strings.HasPrefix(e.Name(), ".") {
			continue
		}
		info, ierr := e.Info()
		if ierr != nil {
			slog.Warn("fs: entry info failed", "name", e.Name(), "error", ierr)
			continue
		}
		isEncrypted := false
		if !e.IsDir() {
			entryAbs := filepath.Join(abs, e.Name())
			if s.detectContainerEntry(entryAbs) {
				isEncrypted = true
			}
		}
		// 相对路径 = mount 的公开路径 + e.Name()（不再拼 queryPath，避免前缀重复）
		relOut := strings.TrimSuffix(args.RelPath, "/")
		if relOut == "" {
			relOut = ""
		}
		relOut = relOut + "/" + e.Name()

		items = append(items, map[string]interface{}{
			"name":         e.Name(),
			"rel_path":     relOut,
			"is_dir":       e.IsDir(),
			"size":         info.Size(),
			"modified":     info.ModTime().Format(time.RFC3339),
			"is_encrypted": isEncrypted, // 是否是 .encv 容器（=加密文件）
		})
	}

	out := map[string]interface{}{
		"mount_id":  args.MountID,
		"rel_path":  args.RelPath,
		"count":     len(items),
		"items":     items,
		"truncated": truncated,
	}
	return okJSON(out), nil
}

func (s *Server) fsReadFile(argsJSON string) (string, error) {
	var args struct {
		MountID  string `json:"mount_id"`
		RelPath  string `json:"rel_path"`
		MaxBytes int    `json:"max_bytes"`
	}
	if err := json.Unmarshal([]byte(argsJSON), &args); err != nil {
		return errJSON("invalid_args", err.Error()), nil
	}
	if args.MountID == "" || args.RelPath == "" {
		return errJSON("missing_args", "mount_id 和 rel_path 必填"), nil
	}
	if args.MaxBytes <= 0 {
		args.MaxBytes = 64 * 1024
	}
	if args.MaxBytes > 1024*1024 {
		args.MaxBytes = 1024 * 1024
	}

	root, ok := s.resolveMount(args.MountID)
	if !ok {
		return errJSON("mount_unavailable", "挂载点 "+args.MountID+" 不存在或不可读"), nil
	}
	abs, err := utils.SafeResolveToAbsPath(root, args.RelPath)
	if err != nil {
		return errJSON("path_forbidden", err.Error()), nil
	}
	info, err := os.Stat(abs)
	if err != nil {
		return errJSON("stat_failed", err.Error()), nil
	}
	if info.IsDir() {
		return errJSON("is_directory", "read_file 不能读目录，请用 list_files"), nil
	}
	if info.Size() > int64(args.MaxBytes) {
		// 不硬读——直接返回提示，让 LLM 决定要不要扩大 max_bytes
		return errJSON("too_large",
			fmt.Sprintf("文件 %d 字节 > max_bytes %d 字节，增大 max_bytes 后重试", info.Size(), args.MaxBytes),
		), nil
	}
	content, err := os.ReadFile(abs)
	if err != nil {
		return errJSON("read_failed", err.Error()), nil
	}

	// 二进制嗅探：头 512 字节有 NUL → 当成二进制
	isText := true
	head := content
	if len(head) > 512 {
		head = head[:512]
	}
	for _, b := range head {
		if b == 0 {
			isText = false
			break
		}
	}
	out := map[string]interface{}{
		"mount_id":    args.MountID,
		"rel_path":    args.RelPath,
		"size":        len(content),
		"encoding":    "utf-8",
		"is_binary":   !isText,
		"content_b64": "", // 占位，下面按需填充
		"content":     "",
	}
	if isText {
		out["content"] = string(content)
		delete(out, "content_b64")
	} else {
		// 二进制不直接吐给 LLM（会把上下文撑爆），改成 base64 + 提示
		out["content_b64"] = base64Encode(content)
		out["note"] = "二进制文件已用 base64 编码，前端 / 调用方可自行解码"
	}
	return okJSON(out), nil
}

func (s *Server) fsStatFile(argsJSON string) (string, error) {
	var args struct {
		MountID string `json:"mount_id"`
		RelPath string `json:"rel_path"`
	}
	if err := json.Unmarshal([]byte(argsJSON), &args); err != nil {
		return errJSON("invalid_args", err.Error()), nil
	}
	if args.MountID == "" || args.RelPath == "" {
		return errJSON("missing_args", "mount_id 和 rel_path 必填"), nil
	}
	root, ok := s.resolveMount(args.MountID)
	if !ok {
		return errJSON("mount_unavailable", "挂载点 "+args.MountID+" 不存在或不可读"), nil
	}
	abs, err := utils.SafeResolveToAbsPath(root, args.RelPath)
	if err != nil {
		return errJSON("path_forbidden", err.Error()), nil
	}
	info, err := os.Stat(abs)
	if err != nil {
		return errJSON("stat_failed", err.Error()), nil
	}

	isEncrypted := false
	if !info.IsDir() {
		if s.detectContainerEntry(abs) {
			isEncrypted = true
		}
	}

	out := map[string]interface{}{
		"mount_id":     args.MountID,
		"rel_path":     args.RelPath,
		"name":         info.Name(),
		"is_dir":       info.IsDir(),
		"size":         info.Size(),
		"mode":         info.Mode().String(),
		"modified":     info.ModTime().Format(time.RFC3339),
		"is_encrypted": isEncrypted,
	}
	return okJSON(out), nil
}

func (s *Server) fsGetStorageInfo(argsJSON string) (string, error) {
	// 不需要参数
	_ = argsJSON

	mounts := s.ListFSMounts()
	seen := make(map[string]bool)
	results := make([]map[string]interface{}, 0, len(mounts))
	for _, m := range mounts {
		abs := ""
		if m.Type == "serving" {
			abs = s.servingDir
		} else if m.Type == "webdav" {
			abs = s.webdavDir
		}
		if abs == "" || seen[abs] {
			continue
		}
		seen[abs] = true
		results = append(results, map[string]interface{}{
			"mount_id":     m.ID,
			"public_path":  m.PublicPath,
			"physical":     abs,
			"storage_info": statFS(abs),
		})
	}
	if len(results) == 0 {
		return errJSON("no_mounts", "无可用挂载点"), nil
	}
	return okJSON(map[string]interface{}{
		"count":  len(results),
		"items":  results,
		"server": map[string]string{"goos": runtime.GOOS},
	}), nil
}

// ─── 辅助函数 ────────────────────────────────────────────────

// statFS 返回 path 所在物理磁盘的总/已用/剩余字节数。
// 跨平台：Windows / Linux / macOS 都覆盖。
//
// 实现说明：Go 标准库的 syscall.Statfs 在 macOS / Linux 上可用，
// 在 Windows 上要用 GetDiskFreeSpaceExW（见下）。
func statFS(path string) map[string]interface{} {
	out := map[string]interface{}{
		"total": int64(0),
		"used":  int64(0),
		"free":  int64(0),
	}
	if path == "" {
		out["error"] = "empty path"
		return out
	}
	var stat syscall.Statfs_t
	if err := syscall.Statfs(path, &stat); err == nil {
		// macOS: Bsize / Frsize 不一致，老 macOS 只用 Bsize
		bsize := stat.Bsize
		if bsize == 0 {
			bsize = int64(stat.Bsize)
		}
		total := int64(stat.Blocks) * bsize
		free := int64(stat.Bavail) * bsize
		used := total - int64(stat.Bfree)*bsize
		out["total"] = total
		out["used"] = used
		out["free"] = free
		return out
	}
	out["error"] = "statfs not supported on this OS / path"
	return out
}

// detectContainerEntry 检查文件是否是 .encv 容器。
// 这里不复用 service 层的 detector（避免循环依赖 + 编译期拉大整树），
// 改用最小特征检测：文件头 magic == ENCV magic bytes。
//
// magic = "ENCV" + uint32 version  + uint32 manifest_len
// 但我们只检测前 4 字节即可（= .encv v2/v3 共同特征）。
func (s *Server) detectContainerEntry(abs string) bool {
	f, err := os.Open(abs)
	if err != nil {
		return false
	}
	defer f.Close()
	var head [4]byte
	n, err := f.Read(head[:])
	if err != nil || n < 4 {
		return false
	}
	return string(head[:]) == "ENCV"
}

// base64Encode 小工具，避免直接 import encoding/base64 在每个文件里
func base64Encode(b []byte) string {
	const tbl = "ABCDEFGHIJKLMNOPQRSTUVWXYZabcdefghijklmnopqrstuvwxyz0123456789+/"
	out := make([]byte, 0, ((len(b)+2)/3)*4)
	i := 0
	for ; i+3 <= len(b); i += 3 {
		v := uint32(b[i])<<16 | uint32(b[i+1])<<8 | uint32(b[i+2])
		out = append(out, tbl[(v>>18)&0x3F], tbl[(v>>12)&0x3F], tbl[(v>>6)&0x3F], tbl[v&0x3F])
	}
	switch len(b) - i {
	case 1:
		v := uint32(b[i]) << 16
		out = append(out, tbl[(v>>18)&0x3F], tbl[(v>>12)&0x3F], '=', '=')
	case 2:
		v := uint32(b[i])<<16 | uint32(b[i+1])<<8
		out = append(out, tbl[(v>>18)&0x3F], tbl[(v>>12)&0x3F], tbl[(v>>6)&0x3F], '=')
	}
	return string(out)
}

// 避免 unused import 警告
var _ = fs.FileMode(0)
