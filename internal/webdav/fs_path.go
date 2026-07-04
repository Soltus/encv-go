package webdav

import (
	"fmt"
	"path"
	"path/filepath"
	"strings"

	"github.com/Soltus/encv-go/internal/utils"
)

// webdavPathToIndexKey 将 WebDAV Handler 传入的绝对路径，转换为用于索引查找的标准键。
// 例如："/webdav/output/file.txt" -> "output/file.txt"
// 例如："/webdav/" -> "."
//
// 🆕 2026-06-17：多挂载点 webdav 适配（multi-mount-storage-refactor spec 续）
//   - 当 goWebdav.Handler.Prefix 被正确设置（server.go 层用 fs.WebdavPrefix() 注入），
//     Stat/OpenFile/Mkdir/RemoveAll 收到的 name 是已经剥离 URL 前缀的相对路径。
//   - 但保持函数同时支持【含前缀】和【不含前缀】两种入参，向后兼容老调用方 + 测试。
//
// 🆕 2026-06-17：多挂载点安全强化 — 显式拦截 .. 段，防止攻击者构造含 .. 的 webdav 路径
// （虽然 webdavPathToIndexKey 只查表不触发 fs 操作，防御性也加上）
func (fs *encvWebDAVFS) webdavPathToIndexKey(name string) (string, error) {
	// 1. 防御性剥离：如果入参仍含 webdavPrefix，主动剥离（兼容旧调用方 / 测试）
	if fs.webdavPrefix != "" && strings.HasPrefix(name, fs.webdavPrefix) {
		name = strings.TrimPrefix(name, fs.webdavPrefix)
	}
	name = strings.TrimPrefix(name, "/")
	name = strings.TrimSuffix(name, "/")
	if name == "" {
		return ".", nil
	}

	// 2. 拦截 .. 段（攻击者可能构造 "/webdav/../etc/passwd"，即使 h.Prefix 已剥离）
	for _, seg := range strings.Split(filepath.ToSlash(name), "/") {
		if seg == ".." {
			return "", fmt.Errorf("path traversal detected: '%s' contains '..' segments", name)
		}
	}
	return name, nil
}

// physicalPathToIndexKey 将物理容器路径和解析出的虚拟文件名，组合成标准的索引键。
// 例如：physicalPath="A:\...\output\video.sccgv", virtualFilename="video.mp4" -> "output/video.mp4"
func (fs *encvWebDAVFS) physicalPathToIndexKey(physicalPath, virtualFilename string) (string, error) {
	// 1. 计算物理文件相对于服务根目录的相对路径
	relPath, err := filepath.Rel(fs.dir, physicalPath)
	if err != nil {
		return "", err
	}

	// 2. 获取该相对路径的目录部分
	virtualDir := filepath.ToSlash(filepath.Dir(relPath))

	// 3. 使用 path.Join 组合成最终的索引键
	// path.Join 会处理斜杠，确保格式正确
	return path.Join(virtualDir, virtualFilename), nil
}

// resolvePath 将 WebDAV 路径安全地解析为本地文件系统绝对路径
//
// 🆕 2026-06-17：多挂载点 webdav 适配（multi-mount-storage-refactor spec 续）
//   - 当 goWebdav.Handler.Prefix 被正确设置（server.go 层用 fs.WebdavPrefix() 注入），
//     Stat/OpenFile/Mkdir/RemoveAll 收到的 name 是已经剥离 URL 前缀的相对路径。
//   - 但保持函数同时支持【含前缀】和【不含前缀】两种入参，向后兼容老调用方 + 测试。
//
// 🆕 2026-06-17：多挂载点安全强化（multi-mount-storage-refactor spec 续）
//   - 在 SafeResolveToAbsPath 之前先做 path.Clean 显式检查 ..
func (fs *encvWebDAVFS) resolvePath(name string) (string, error) {
	// 1. 防御性剥离：如果入参仍含 webdavPrefix，主动剥离（兼容旧调用方 / 测试）
	if fs.webdavPrefix != "" && strings.HasPrefix(name, fs.webdavPrefix) {
		name = strings.TrimPrefix(name, fs.webdavPrefix)
	}
	if name == "" {
		name = "."
	}

	// 🆕 2026-06-17：显式拦截路径穿越（防止 userPath 是绝对路径 / 含 .. 段）
	//
	// 为什么需要这步：SafeResolveToAbsPath 用 filepath.Rel 校验 .. 段，
	// 但 filepath.Clean 会把开头的 .. 段裁剪（POSIX 语义：".." 在根目录之上无效），
	// 导致 absPath 越过 baseDir 上层目录。
	//
	// 攻击向量：userPath = "/../../etc/passwd" → Clean → "/etc/passwd" → absPath = baseDir + "/etc/passwd"
	// filepath.Rel(baseDir, baseDir/etc/passwd) = "etc/passwd" → 不以 .. 开头 → 通过校验
	// 实际 attacker 拿到了 baseDir/etc/passwd 的访问权限（虽然不能逃出 baseDir，但可读 baseDir 任意子目录）
	//
	// 修复：直接按 path segment 检查 ..（不依赖 Clean 行为）。
	// 任何段 == ".." 立即拒绝。
	for _, seg := range strings.Split(filepath.ToSlash(name), "/") {
		if seg == ".." {
			return "", fmt.Errorf("path traversal detected: '%s' contains '..' segments", name)
		}
	}

	return utils.SafeResolveToAbsPath(fs.dir, name)
}
