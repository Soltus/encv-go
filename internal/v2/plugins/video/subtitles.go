// internal/v2/plugins/video/subtitles.go

package video

import (
	"fmt"
	"log"
	"os"
	"path/filepath"
	"sort"
	"strings"

	"github.com/Soltus/encv-go/internal/config"
	"github.com/Soltus/encv-go/internal/utils"
)

// HandleSubtitlesForEncryption 处理加密时的字幕逻辑
func (p *VideoPlugin) HandleSubtitlesForEncryption(cfg *config.Config, vIndex *VideoIndex, outputDir, encryptedBaseName string) error {
	subtitleTracks, err := DiscoverSubtitleTracks(vIndex.OriginalInputPath, p.trackExtensionsList)
	if err != nil {
		log.Printf("Warning: subtitle discovery failed for %s: %v", vIndex.OriginalInputPath, err)
		// 不返回错误，继续无字幕打包
		vIndex.SubtitleTracks = nil
		return nil
	}
	// 【关键修复】使用 CopyAndRenameSubtitles 返回的 kviTracks，它们包含 Title 字段（加密后的文件名）
	kviTracks, err := CopyAndRenameSubtitles(subtitleTracks, vIndex.OriginalInputPath, outputDir, encryptedBaseName)
	if err != nil {
		return err
	}
	vIndex.SubtitleTracks = kviTracks
	return nil
}

// RestoreSubtitlesForDecryption 处理解密时的字幕还原
func RestoreSubtitlesForDecryption(index *VideoIndex, containerDir, outputDir string) error {

	// containerDir := filepath.Dir(containerPath)
	if err := RestoreSubtitlesFromKVI(index, containerDir, outputDir); err != nil {
		// 返回警告而不是错误，允许主流程继续
		return fmt.Errorf("warning: failed to restore subtitles: %w", err)
	}

	return nil
}

// DiscoverSubtitleTracks 发现视频关联的字幕轨道，只返回信息
func DiscoverSubtitleTracks(inputPath string, trackExtensions []string) ([]SubtitleTracks, error) {
	fmt.Println("-> Discovering subtitle tracks...")
	videoDir := filepath.Dir(inputPath)
	videoBaseName := strings.TrimSuffix(filepath.Base(inputPath), filepath.Ext(inputPath))

	files, _ := os.ReadDir(videoDir)
	sortedExts := utils.SortExtensionsByLength(trackExtensions)
	var tracks []SubtitleTracks

	for _, f := range files {
		if f.IsDir() {
			continue
		}
		fileName := f.Name()

		isSubtitle := false
		for _, ext := range sortedExts {
			if strings.HasSuffix(fileName, ext) {
				isSubtitle = true
				break
			}
		}
		if !isSubtitle {
			continue
		}

		subBaseName := utils.StripKnownExtensions(fileName, sortedExts)
		if (strings.HasPrefix(subBaseName, videoBaseName) || subBaseName == videoBaseName) && subBaseName != "" {
			log.Printf("-> Found track: %s\n", fileName)
			lang := "und"
			if strings.Contains(fileName, "chi") || strings.Contains(fileName, "zh") {
				lang = "chi"
			} else if strings.Contains(fileName, "eng") {
				lang = "eng"
			}
			tracks = append(tracks, SubtitleTracks{
				Language: lang,
				Filename: fileName,
			})
		}
	}
	return tracks, nil
}

// CopyAndRenameSubtitles 将发现的字幕复制到输出目录并重命名
func CopyAndRenameSubtitles(tracks []SubtitleTracks, videoPath, outputDir, encBaseName string) ([]SubtitleTracks, error) {
	if len(tracks) == 0 {
		return nil, nil
	}

	videoDir := filepath.Dir(videoPath)
	// 排序以确保命名一致
	sort.Slice(tracks, func(i, j int) bool {
		return tracks[i].Filename < tracks[j].Filename
	})

	var kviTracks []SubtitleTracks
	for i, track := range tracks {
		originalPath := filepath.Join(videoDir, track.Filename)

		ext := filepath.Ext(track.Filename)
		newFilename := fmt.Sprintf("%s%s", encBaseName, ext)
		if i > 0 {
			newFilename = fmt.Sprintf("%s.%d%s", encBaseName, i+1, ext)
		}

		newPath := filepath.Join(outputDir, newFilename)
		if err := utils.CopyFile(originalPath, newPath); err != nil {
			return nil, fmt.Errorf("failed to copy subtitle from %s to %s: %w", originalPath, newPath, err)
		}
		log.Printf("-> Copied subtitle to '%s'\n", newFilename)

		kviTracks = append(kviTracks, SubtitleTracks{
			Language: track.Language,
			Title:    newFilename,    // 加密后关联的文件名（字幕本身不加密）
			Filename: track.Filename, // 原始文件名
		})
	}
	return kviTracks, nil
}

// RestoreSubtitlesFromKVI 根据 KVI 中的信息，将字幕从容器目录恢复到输出目录
func RestoreSubtitlesFromKVI(index *VideoIndex, containerDir, outputDir string) error {
	if len(index.SubtitleTracks) == 0 {
		return nil
	}
	fmt.Println("-> Restoring subtitles...")
	for _, sub := range index.SubtitleTracks {
		// sub.Title 是加密后存储在容器目录中的字幕文件名 (e.g., "myvideo.sccgv.srt")
		// sub.Filename 是需要恢复成的原始文件名 (e.g., "myvideo.zh.srt")
		srcPath := filepath.Join(containerDir, sub.Title)
		dstPath := filepath.Join(outputDir, sub.Filename)

		if _, err := os.Stat(srcPath); os.IsNotExist(err) {
			log.Printf("-> Warning: Subtitle file '%s' not found in container directory, skipping.\n", sub.Title)
			continue
		}

		log.Printf("-> Restoring subtitle: %s\n", sub.Filename)
		if err := utils.CopyFile(srcPath, dstPath); err != nil {
			log.Printf("-> Warning: Failed to restore subtitle '%s': %v\n", sub.Filename, err)
		}
	}
	return nil
}
