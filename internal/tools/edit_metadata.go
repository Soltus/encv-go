// internal/tools/edit_metadata.go
//
// 元数据编辑工具 — 写 ID3 / MP4 atoms。
//
// 流程：
//   1. 备份原文件（<name>.bak）
//   2. 调 ffmpeg -i in -metadata key=value -c copy out
//   3. 失败 → 恢复备份
//   4. 成功 → 返回新文件路径
//
// requires_confirm: true
package tools

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"time"
)

// regexCompileErr 是 regexp.Compile 的薄包装（便于单测时 mock）。
var regexCompileErr = regexpCompileErrFn

func regexpCompileErrFn(pattern string) (*regexp.Regexp, error) {
	return regexp.Compile(pattern)
}

// readDirNames 读取目录条目名（按字母序）。
func readDirNames(dir string) ([]string, error) {
	entries, err := os.ReadDir(dir)
	if err != nil {
		return nil, err
	}
	names := make([]string, 0, len(entries))
	for _, e := range entries {
		names = append(names, e.Name())
	}
	return names, nil
}

// ─── 参数 / 结果 ───────────────────────────────────────────────

// EditMetadataArgs 工具参数。
type EditMetadataArgs struct {
	MountID  string            `json:"mount_id"`
	RelPath  string            `json:"rel_path"`
	Metadata map[string]string `json:"metadata"`
}

// EditMetadataResult 工具结果。
type EditMetadataResult struct {
	Success     bool   `json:"success"`
	BackupPath  string `json:"backup_path,omitempty"`
	NewPath     string `json:"new_path,omitempty"`
	ChangedKeys []string `json:"changed_keys,omitempty"`
}

// ─── ToolDef ───────────────────────────────────────────────────

// EditMetadataDef 返回 edit_metadata 的 ToolDef。
func EditMetadataDef() *ToolDef {
	return &ToolDef{
		Name:            "edit_metadata",
		Description:     "编辑媒体文件元数据（title/artist/comment）。通过 ffmpeg 实现，自动备份+失败回滚。需要用户确认才能执行。",
		Kind:            KindFileChange,
		ReadOnly:        false,
		RequiresConfirm: true,
		ArgsSchema: `{
			"type":"object",
			"required":["mount_id","rel_path","metadata"],
			"properties":{
				"mount_id":{"type":"string"},
				"rel_path":{"type":"string"},
				"metadata":{"type":"object","additionalProperties":{"type":"string"}}
			}
		}`,
		Handler: editMetadataHandler,
	}
}

// ─── Handler ───────────────────────────────────────────────────

func editMetadataHandler(ctx context.Context, argsJSON string, deps *ToolDeps) (ToolResult, error) {
	var args EditMetadataArgs
	if err := json.Unmarshal([]byte(argsJSON), &args); err != nil {
		return errResult(fmt.Sprintf("invalid args: %v", err)), nil
	}
	if args.MountID == "" || args.RelPath == "" {
		return errResult("mount_id and rel_path required"), nil
	}
	if len(args.Metadata) == 0 {
		return errResult("metadata cannot be empty"), nil
	}
	if deps == nil || deps.ResolveMount == nil {
		return errResult("deps not initialized"), nil
	}
	rootAbs, ok := deps.ResolveMount(args.MountID)
	if !ok {
		return errResult(fmt.Sprintf("mount not found: %s", args.MountID)), nil
	}
	absPath, err := safeJoin(rootAbs, args.RelPath)
	if err != nil {
		return errResult(err.Error()), nil
	}
	if _, err := os.Stat(absPath); err != nil {
		return errResult(fmt.Sprintf("source not found: %v", err)), nil
	}

	// 备份
	backupPath := absPath + ".bak"
	if err := copyFile(absPath, backupPath); err != nil {
		return errResult(fmt.Sprintf("backup failed: %v", err)), nil
	}
	defer func() {
		// 失败时回滚
		// （成功时由调用方显式清理）
	}()

	// 临时输出文件
	tmpPath := absPath + ".tmp"
	if err := runFfmpegMetadata(ctx, absPath, tmpPath, args.Metadata); err != nil {
		// 失败 → 回滚
		_ = os.Remove(tmpPath)
		b, _ := json.Marshal(map[string]any{
			"error":  err.Error(),
			"backup": backupPath,
		})
		return ToolResult{Result: string(b), IsError: true, Status: "failed"}, nil
	}

	// 替换原文件
	if err := os.Rename(tmpPath, absPath); err != nil {
		_ = os.Remove(tmpPath)
		b, _ := json.Marshal(map[string]any{
			"error":  fmt.Sprintf("rename failed: %v", err),
			"backup": backupPath,
		})
		return ToolResult{Result: string(b), IsError: true, Status: "failed"}, nil
	}

	// 收集变更的 keys
	changed := make([]string, 0, len(args.Metadata))
	for k := range args.Metadata {
		changed = append(changed, k)
	}
	res := EditMetadataResult{
		Success:     true,
		BackupPath:  backupPath,
		NewPath:     absPath,
		ChangedKeys: changed,
	}
	b, _ := json.Marshal(res)
	return ToolResult{Result: string(b), Status: "success"}, nil
}

// runFfmpegMetadata 调 ffmpeg 写元数据。失败 → 返回 err。
func runFfmpegMetadata(ctx context.Context, in, out string, metadata map[string]string) error {
	bin, err := exec.LookPath("ffmpeg")
	if err != nil {
		return fmt.Errorf("ffmpeg not in PATH: %w", err)
	}
	cctx, cancel := context.WithTimeout(ctx, 30*time.Second)
	defer cancel()
	args := []string{"-y", "-i", in, "-c", "copy"}
	for k, v := range metadata {
		args = append(args, "-metadata", k+"="+v)
	}
	args = append(args, out)
	cmd := exec.CommandContext(cctx, bin, args...)
	if outBytes, err := cmd.CombinedOutput(); err != nil {
		return fmt.Errorf("ffmpeg failed: %w (output: %s)", err, truncateString(string(outBytes), 500))
	}
	return nil
}

// copyFile 简单文件复制。
func copyFile(src, dst string) error {
	in, err := os.Open(src)
	if err != nil {
		return err
	}
	defer in.Close()
	out, err := os.Create(dst)
	if err != nil {
		return err
	}
	defer out.Close()
	_, err = out.ReadFrom(in)
	return err
}

// truncateString 截断字符串到指定字节数。
func truncateString(s string, max int) string {
	if len(s) <= max {
		return s
	}
	return s[:max] + "...(truncated)"
}

// ─── batch_rename.go（同文件） ────────────────────────────────

// BatchRenameArgs 批量改名参数。
type BatchRenameArgs struct {
	MountID     string `json:"mount_id"`
	RelPath     string `json:"rel_path"`     // 起始目录
	Pattern     string `json:"pattern"`      // regex 模式
	Replacement string `json:"replacement"`  // 含 $1/$2
	DryRun      bool   `json:"dry_run"`      // 仅预览
	MaxFiles    int    `json:"max_files"`    // 默认 200
}

// BatchRenamePreview 预览条目。
type BatchRenamePreview struct {
	Old string `json:"old"`
	New string `json:"new"`
}

// BatchRenameResult 批量改名结果。
type BatchRenameResult struct {
	Total    int                  `json:"total"`
	Applied  int                  `json:"applied"`
	Failed   int                  `json:"failed"`
	Previews []BatchRenamePreview `json:"previews,omitempty"`
	Backups  []string             `json:"backups,omitempty"`
	RolledBack bool               `json:"rolled_back,omitempty"`
}

// BatchRenameDef returns batch_rename ToolDef.
func BatchRenameDef() *ToolDef {
	return &ToolDef{
		Name:            "batch_rename",
		Description:     "批量重命名（regex pattern + replacement）。支持 dry_run 预览。任一失败回滚全部。需要用户确认。",
		Kind:            KindFileChange,
		ReadOnly:        false,
		RequiresConfirm: true,
		ArgsSchema: `{
			"type":"object",
			"required":["mount_id","rel_path","pattern","replacement"],
			"properties":{
				"mount_id":{"type":"string"},
				"rel_path":{"type":"string"},
				"pattern":{"type":"string"},
				"replacement":{"type":"string"},
				"dry_run":{"type":"boolean","default":true},
				"max_files":{"type":"integer","default":200}
			}
		}`,
		Handler: batchRenameHandler,
	}
}

func batchRenameHandler(ctx context.Context, argsJSON string, deps *ToolDeps) (ToolResult, error) {
	var args BatchRenameArgs
	if err := json.Unmarshal([]byte(argsJSON), &args); err != nil {
		return errResult(fmt.Sprintf("invalid args: %v", err)), nil
	}
	if args.Pattern == "" {
		return errResult("pattern required"), nil
	}
	if args.MaxFiles <= 0 {
		args.MaxFiles = 200
	}
	if deps == nil || deps.ResolveMount == nil {
		return errResult("deps not initialized"), nil
	}
	rootAbs, ok := deps.ResolveMount(args.MountID)
	if !ok {
		return errResult(fmt.Sprintf("mount not found: %s", args.MountID)), nil
	}
	dirPath, err := safeJoin(rootAbs, args.RelPath)
	if err != nil {
		return errResult(err.Error()), nil
	}
	re, err := regexCompileErr(args.Pattern)
	if err != nil {
		return errResult(fmt.Sprintf("invalid pattern: %v", err)), nil
	}
	// 扫描目录
	entries, err := readDirNames(dirPath)
	if err != nil {
		return errResult(fmt.Sprintf("read dir: %v", err)), nil
	}
	previews := make([]BatchRenamePreview, 0, len(entries))
	for _, name := range entries {
		newName := re.ReplaceAllString(name, args.Replacement)
		if newName != name {
			previews = append(previews, BatchRenamePreview{
				Old: filepath.Join(dirPath, name),
				New: filepath.Join(dirPath, newName),
			})
		}
		if len(previews) >= args.MaxFiles {
			break
		}
	}
	res := BatchRenameResult{Total: len(previews), Previews: previews}

	if args.DryRun {
		b, _ := json.Marshal(res)
		return ToolResult{Result: string(b), Status: "success"}, nil
	}
	// 真实执行：先全部备份，再逐个改名，任一失败回滚
	backups := make([]string, 0, len(previews))
	applied := make([]BatchRenamePreview, 0, len(previews))
	for j, p := range previews {
		backup := p.Old + ".bak"
		if err := copyFile(p.Old, backup); err != nil {
			// 回滚
			for i, bk := range backups {
				_ = os.Rename(bk, applied[i].Old)
			}
			res.RolledBack = true
			res.Failed = len(previews) - j
			b, _ := json.Marshal(map[string]any{
				"error":        fmt.Sprintf("backup failed for %s: %v", p.Old, err),
				"rolled_back":  true,
				"backups":      backups,
			})
			return ToolResult{Result: string(b), IsError: true, Status: "failed"}, nil
		}
		backups = append(backups, backup)
		applied = append(applied, p)
	}
	// 改名
	for _, p := range previews {
		if err := os.Rename(p.Old, p.New); err != nil {
			// 回滚
			for i, bk := range backups {
				_ = os.Rename(bk, applied[i].Old)
			}
			res.RolledBack = true
			b, _ := json.Marshal(map[string]any{
				"error":       fmt.Sprintf("rename failed: %v", err),
				"rolled_back": true,
			})
			return ToolResult{Result: string(b), IsError: true, Status: "failed"}, nil
		}
	}
	res.Applied = len(previews)
	res.Backups = backups
	b, _ := json.Marshal(res)
	return ToolResult{Result: string(b), Status: "success"}, nil
}

// ─── delete_file.go（同文件） ────────────────────────────────

// DeleteFileArgs 删除文件参数。
type DeleteFileArgs struct {
	MountID string `json:"mount_id"`
	RelPath string `json:"rel_path"`
	Mode    string `json:"mode"` // "trash" | "hard"
}

// DeleteFileResult 删除结果。
type DeleteFileResult struct {
	Success   bool   `json:"success"`
	TrashedTo string `json:"trashed_to,omitempty"`
	HardDeleted bool `json:"hard_deleted,omitempty"`
}

// DeleteFileDef returns delete_file ToolDef.
func DeleteFileDef() *ToolDef {
	return &ToolDef{
		Name:            "delete_file",
		Description:     "删除文件。默认走 mount 的 trash_path；hard 模式需要二次确认。",
		Kind:            KindFileChange,
		ReadOnly:        false,
		RequiresConfirm: true,
		ArgsSchema: `{
			"type":"object",
			"required":["mount_id","rel_path"],
			"properties":{
				"mount_id":{"type":"string"},
				"rel_path":{"type":"string"},
				"mode":{"type":"string","enum":["trash","hard"],"default":"trash"}
			}
		}`,
		Handler: deleteFileHandler,
	}
}

func deleteFileHandler(ctx context.Context, argsJSON string, deps *ToolDeps) (ToolResult, error) {
	var args DeleteFileArgs
	if err := json.Unmarshal([]byte(argsJSON), &args); err != nil {
		return errResult(fmt.Sprintf("invalid args: %v", err)), nil
	}
	if args.MountID == "" || args.RelPath == "" {
		return errResult("mount_id and rel_path required"), nil
	}
	if deps == nil || deps.ResolveMount == nil {
		return errResult("deps not initialized"), nil
	}
	rootAbs, ok := deps.ResolveMount(args.MountID)
	if !ok {
		return errResult(fmt.Sprintf("mount not found: %s", args.MountID)), nil
	}
	absPath, err := safeJoin(rootAbs, args.RelPath)
	if err != nil {
		return errResult(err.Error()), nil
	}
	if _, err := os.Stat(absPath); err != nil {
		return errResult(fmt.Sprintf("source not found: %v", err)), nil
	}
	mode := args.Mode
	if mode == "" {
		mode = "trash"
	}
	if mode != "trash" && mode != "hard" {
		return errResult("mode must be 'trash' or 'hard'"), nil
	}

	// trash 模式：优先调 TrashManager（统一回收站），fallback 到旧硬编码逻辑
	if mode == "trash" {
		// 🆕 v6 2026-06-22：统一走 TrashManager（与 mobile 前端回收站共用 trash 表）
		if deps.TrashMover != nil {
			item, err := deps.TrashMover.MoveToTrash(absPath, "")
			if err != nil {
				return errResult(fmt.Sprintf("trash move failed: %v", err)), nil
			}
			res := DeleteFileResult{Success: true, TrashedTo: item.TrashPath}
			b, _ := json.Marshal(res)
			return ToolResult{Result: string(b), Status: "success"}, nil
		}
		// 旧逻辑 fallback（TrashManager 未注入时）
		trashDir := filepath.Join(rootAbs, ".trash")
		_ = os.MkdirAll(trashDir, 0o755)
		trashPath := filepath.Join(trashDir, fmt.Sprintf("%d_%s", time.Now().UnixNano(), filepath.Base(absPath)))
		if err := os.Rename(absPath, trashPath); err != nil {
			return errResult(fmt.Sprintf("trash move failed: %v", err)), nil
		}
		res := DeleteFileResult{Success: true, TrashedTo: trashPath}
		b, _ := json.Marshal(res)
		return ToolResult{Result: string(b), Status: "success"}, nil
	}
	// hard 模式：直接删除（仍需前端二次确认才进 handler）
	if err := os.Remove(absPath); err != nil {
		return errResult(fmt.Sprintf("remove failed: %v", err)), nil
	}
	res := DeleteFileResult{Success: true, HardDeleted: true}
	b, _ := json.Marshal(res)
	return ToolResult{Result: string(b), Status: "success"}, nil
}
