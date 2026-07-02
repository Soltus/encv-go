// internal/v2/plugins/video/searchable.go
//
// 2026-07-02 用户反馈：插件自主声明容器内可被全文搜索的内容
// 视频插件声明 searchable: subtitle / title / chapters
//
// 实现 SearchableContentsExtractor 接口：
//   - GetSearchableContentsManifest 告诉系统"我能搜什么"
//   - ExtractSearchableContents 从加密容器（同级目录）提取字幕/标题文本

package video

import (
	"bufio"
	"fmt"
	"os"
	"path/filepath"
	"strings"

	pluginInterfaces "github.com/Soltus/encv-go/internal/v2/plugins/interfaces"
)

// GetSearchableContentsManifest 声明视频插件可被全文搜索的内容类型。
//
// 设计：
//   - subtitle: 关联的 .srt/.ass/.vtt 字幕（KVI 命名）
//   - title:    视频文件名（basename，去扩展名）
//   - chapters: 视频章节列表（如果有）
//
// 注意：实际能不能搜到这些内容，取决于容器加密时是否把这些内容也存进了容器同级目录。
// 当前视频插件的 HandleSubtitlesForEncryption 会把字幕复制到 container 同一目录
// （KVI 命名），所以 FTS5 scanner 找得到。
func (p *VideoPlugin) GetSearchableContentsManifest() pluginInterfaces.SearchableContentsManifest {
	return pluginInterfaces.SearchableContentsManifest{
		Enabled: true,
		Types: []string{
			pluginInterfaces.SearchableTypeSubtitle,
			pluginInterfaces.SearchableTypeTitle,
		},
	}
}

// ExtractSearchableContents 从视频容器同级目录中提取字幕内容。
//
// 输入：containerPath 是 .sccgv 容器的绝对路径
// 输出：多条 SearchableContentItem，每条对应一个字幕轨道 + 一条 title
// 错误：字幕文件读取失败时 item 列表可能部分成功；只有严重错误才返回 error
//
// 算法：
//   1. 拿 container 的 dirname
//   2. 扫该目录下所有 .srt/.ass/.vtt/.dm.ass 文件（KVI 命名格式）
//   3. 每个文件作为一个 SearchableContentItem（Type="subtitle", Name=文件名, Text=文件内容）
//   4. 容器名去扩展名作为 title（Type="title"）
func (p *VideoPlugin) ExtractSearchableContents(containerPath string) ([]pluginInterfaces.SearchableContentItem, error) {
	if containerPath == "" {
		return nil, fmt.Errorf("empty container path")
	}

	containerDir := filepath.Dir(containerPath)
	containerName := filepath.Base(containerPath)

	// 字幕扩展名（与 settings.TrackExtensions 同步；空时用默认）
	extList := p.settings.TrackExtensions
	if extList == "" {
		extList = ".ass,.srt,.dm.ass"
	}
	exts := splitExtList(extList)

	dirEntries, err := os.ReadDir(containerDir)
	if err != nil {
		return nil, fmt.Errorf("read container dir: %w", err)
	}

	var items []pluginInterfaces.SearchableContentItem

	for _, entry := range dirEntries {
		if entry.IsDir() {
			continue
		}
		name := entry.Name()
		lower := strings.ToLower(name)
		// 是否字幕文件（按扩展名匹配）
		isSubtitle := false
		for _, ext := range exts {
			if strings.HasSuffix(lower, strings.ToLower(ext)) {
				isSubtitle = true
				break
			}
		}
		if !isSubtitle {
			continue
		}

		// 只读容器关联的字幕（KVI 命名通常以容器 basename 起头）
		// 简化策略：读 containerDir 下所有字幕文件，文件大小限制 256KB
		fullPath := filepath.Join(containerDir, name)
		info, err := entry.Info()
		if err != nil {
			continue
		}
		if info.Size() > 256*1024 {
			// 字幕文件不应该超过 256KB（异常），跳过
			continue
		}

		text, err := readSubtitleText(fullPath, info.Size())
		if err != nil {
			// 单个字幕读失败不影响其他
			continue
		}

		items = append(items, pluginInterfaces.SearchableContentItem{
			Type: pluginInterfaces.SearchableTypeSubtitle,
			Name: name,
			Text: text,
		})
	}

	// title：容器文件名去扩展名
	title := strings.TrimSuffix(containerName, filepath.Ext(containerName))
	items = append(items, pluginInterfaces.SearchableContentItem{
		Type: pluginInterfaces.SearchableTypeTitle,
		Name: "title",
		Text: title,
	})

	return items, nil
}

// readSubtitleText 读字幕文件，返回纯文本（去掉 SRT 时间戳、ASS 格式符号等）。
// 限制大小防止异常大文件拖慢索引。
func readSubtitleText(path string, maxSize int64) (string, error) {
	f, err := os.Open(path)
	if err != nil {
		return "", err
	}
	defer f.Close()

	// 限制读取字节数
	limit := maxSize
	if limit > 256*1024 {
		limit = 256 * 1024
	}

	scanner := bufio.NewScanner(f)
	scanner.Buffer(make([]byte, 64*1024), int(limit))

	var lines []string
	for scanner.Scan() {
		line := strings.TrimSpace(scanner.Text())
		// 跳过 SRT 序号行（纯数字）
		if isSRTIndexLine(line) {
			continue
		}
		// 跳过 SRT 时间戳行（如 "00:00:01,000 --> 00:00:02,000"）
		if isSRTTimestampLine(line) {
			continue
		}
		// 跳过空行
		if line == "" {
			continue
		}
		// 跳过 ASS 格式头（[Script Info] 等）
		if strings.HasPrefix(line, "[") && strings.HasSuffix(line, "]") {
			continue
		}
		// 去掉 ASS 样式覆盖（{\b1} 等）
		line = stripASSTags(line)
		lines = append(lines, line)
	}
	if err := scanner.Err(); err != nil {
		return "", err
	}
	return strings.Join(lines, " "), nil
}

// isSRTIndexLine 检测是否是 SRT 序号行（纯数字）
func isSRTIndexLine(line string) bool {
	if line == "" {
		return false
	}
	for _, c := range line {
		if c < '0' || c > '9' {
			return false
		}
	}
	return true
}

// isSRTTimestampLine 检测是否是 SRT 时间戳行
func isSRTTimestampLine(line string) bool {
	// 00:00:01,000 --> 00:00:02,000 或 00:00:01.000 --> 00:00:02.000
	if !strings.Contains(line, "-->") {
		return false
	}
	// 简化：包含 "-->" 即视为时间戳行
	return true
}

// stripASSTags 去掉 ASS 样式覆盖标签（如 {\b1}、{\i1}、{\fnMicrosoft YaHei}）
func stripASSTags(line string) string {
	var sb strings.Builder
	sb.Grow(len(line))
	inTag := false
	for _, c := range line {
		switch {
		case c == '{':
			inTag = true
		case c == '}':
			inTag = false
		case !inTag:
			sb.WriteRune(c)
		}
	}
	return sb.String()
}

// splitExtList 把逗号分隔的扩展名列表拆成数组
func splitExtList(s string) []string {
	parts := strings.Split(s, ",")
	out := make([]string, 0, len(parts))
	for _, p := range parts {
		p = strings.TrimSpace(p)
		if p == "" {
			continue
		}
		if !strings.HasPrefix(p, ".") {
			p = "." + p
		}
		out = append(out, p)
	}
	return out
}
