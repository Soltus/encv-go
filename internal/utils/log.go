package utils

import (
	"io"
	"log"
	"os"
	"path/filepath"
)

// setupLogging 设置日志记录，同时输出到控制台和文件，并返回日志文件路径
func SetupLogging(logFileName string) string {
	logFilePath := filepath.Join(os.TempDir(), logFileName)
	logFile, err := os.OpenFile(logFilePath, os.O_CREATE|os.O_WRONLY|os.O_APPEND, 0666)
	if err != nil {
		// 如果无法创建日志文件，我们只能回退到控制台
		log.Printf("Warning: could not open log file %s: %v", logFilePath, err)
		return logFilePath
	}

	// 创建一个 MultiWriter，同时写入标准错误和日志文件
	multiWriter := io.MultiWriter(os.Stderr, logFile)
	log.SetOutput(multiWriter)

	// 打印一条分隔线，以便区分不同的运行
	log.Println("--- ENCV Run Started ---")

	return logFilePath
}
