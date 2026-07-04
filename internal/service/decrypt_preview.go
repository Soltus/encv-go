//go:build !android

// internal/service/decrypt_preview.go
// 预览功能：处理通过HTTP流和mpv播放器来预览加密文件的逻辑。

package service

import (
	"context"
	"errors"
	"fmt"
	"log"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"time"
)

// DecryptMode 定义解压的模式
type DecryptMode string

const (
	// ModePreview 预览模式：解压到临时目录并用默认程序打开
	ModePreview DecryptMode = "preview"
	// ModeToFolder 解压到指定文件夹
	ModeToFolder DecryptMode = "to-folder"
	// ModeHere 解压到当前文件夹
	ModeHere DecryptMode = "here"
	// ModeToSubfolder 解压到同名文件夹
	ModeToSubfolder DecryptMode = "to-subfolder"
)

// DecryptOptions 包含解密操作所需的选项。
type DecryptOptions struct {
	// OutputDir 指定解密后文件的输出目录。
	OutputDir string
	Mode      DecryptMode // 解压模式
}

// Preview 通过HTTP流和mpv播放器来预览加密文件。
func Preview(ctx context.Context, inputPath string) error {
	// 1. 构造视频和字幕的HTTP URL
	videoURL, subtitleURL, err := constructURLs(inputPath)
	if err != nil {
		return fmt.Errorf("failed to construct URLs: %w", err)
	}

	// 2. 查找 mpv 播放器
	mpvPath, err := exec.LookPath("mpv")
	if err != nil {
		return fmt.Errorf("mpv not found in PATH. Please install mpv to use the preview feature: %w", err)
	}

	// 3. 启动 mpv 播放器
	log.Printf("-> Starting mpv to stream: %s\n", videoURL)
	args := []string{videoURL}
	if subtitleURL != "" {
		args = append(args, "--sub-files="+subtitleURL)
		log.Printf("-> With subtitles: %s\n", subtitleURL)
	}
	// 添加 --keep-open 参数，使mpv播放完后不自动退出
	args = append(args, "--keep-open")
	// 添加 --log-file 参数来捕获详细日志
	logFilePath := filepath.Join(os.TempDir(), "mpv_preview.log")
	args = append(args, "--log-file="+logFilePath)
	log.Printf("-> mpv detailed logs will be saved to: %s\n", logFilePath)

	cmd := exec.Command(mpvPath, args...)
	// 将 mpv 的标准错误输出重定向到我们程序的标准错误输出
	// 这样我们就能在终端看到 mpv 的所有日志和错误信息
	cmd.Stderr = os.Stderr
	cmd.Stdout = os.Stdout

	if err := cmd.Start(); err != nil {
		return fmt.Errorf("failed to start mpv: %w", err)
	}

	// 4. 等待 mpv 进程结束，并设置超时
	fmt.Println("-> Preview started. Waiting for mpv to close...")
	waitCtx, cancel := context.WithTimeout(context.Background(), 30*time.Minute)
	defer cancel()

	done := make(chan error, 1)
	go func() {
		err = cmd.Wait()
		done <- err
	}()

	select {
	case <-waitCtx.Done():
		fmt.Println("-> Timeout reached while waiting for mpv to close. Forcing exit.")
		if err := cmd.Process.Signal(os.Interrupt); err != nil {
			cmd.Process.Kill()
		}
		return waitCtx.Err()
	case err := <-done:
		if err != nil {
			log.Printf("-> mpv exited with an error: %v\n", err)
			// 尝试获取并打印退出码
			var exitError *exec.ExitError
			if errors.As(err, &exitError) {
				log.Printf("-> mpv Exit Code: %d\n", exitError.ExitCode())
			}
		} else {
			fmt.Println("-> mpv closed normally (exit code 0).")
		}
		return nil
	}
}

// constructURLs 根据本地文件路径构造视频和字幕的HTTP URL。
// 它假设HTTP服务运行在 localhost:1999，并且服务的根目录是当前工作目录。
func constructURLs(inputPath string) (videoURL, subtitleURL string, err error) {
	const (
		serverAddr = "http://localhost:1999"
	)

	// 获取相对于当前工作目录的路径，作为HTTP路径
	relPath, err := filepath.Rel(".", inputPath)
	if err != nil {
		return "", "", fmt.Errorf("could not get relative path for '%s': %w", inputPath, err)
	}

	// 统一使用正斜杠作为HTTP路径分隔符
	httpPath := strings.ReplaceAll(relPath, "\\", "/")
	videoURL = serverAddr + "/" + httpPath

	// 推断字幕文件路径
	subtitlePath := inferSubtitlePath(inputPath)
	if subtitlePath != "" {
		subtitleRelPath, err := filepath.Rel(".", subtitlePath)
		if err != nil {
			return "", "", fmt.Errorf("could not get relative path for subtitle '%s': %w", subtitlePath, err)
		}
		subtitleHTTPPath := strings.ReplaceAll(subtitleRelPath, "\\", "/")
		subtitleURL = serverAddr + "/" + subtitleHTTPPath
	}

	return videoURL, subtitleURL, nil
}

// inferSubtitlePath 根据视频文件路径推断同名的字幕文件路径。
func inferSubtitlePath(videoPath string) string {
	ext := filepath.Ext(videoPath)
	if ext == "" {
		return ""
	}
	// 假设字幕文件与视频文件同名，但扩展名为 .ass 或 .srt
	base := videoPath[:len(videoPath)-len(ext)]

	// 检查常见的字幕文件是否存在
	for _, subExt := range []string{".ass", ".srt"} {
		subPath := base + subExt
		if _, err := os.Stat(subPath); err == nil {
			return subPath
		}
	}

	return "" // 未找到字幕文件
}
