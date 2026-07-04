package handler

import (
	"fmt"
	"io"
	"log"
	"net/http"
	"path/filepath"
	"strconv"
	"strings"

	"github.com/Soltus/encv-go/internal/utils"
	"github.com/Soltus/encv-go/internal/v2/provider"
)

// ContentHandler 是一个无状态的处理器，它封装了所有文件服务的 HTTP 逻辑。
// 它接收一个实现了 provider.FileContentProvider 接口的对象，并将其内容通过 HTTP 协议正确地发送给客户端。
type ContentHandler struct{}

// NewContentHandler 创建一个新的 ContentHandler 实例。
func NewContentHandler() *ContentHandler {
	return &ContentHandler{}
}

// ServeFile 是一个通用的函数，用于通过 HTTP 响应提供文件内容。
// 它是整个文件服务架构的终点，负责所有 HTTP 协议层的细节。
//
// 参数:
//   - w: http.ResponseWriter
//   - r: *http.Request
//   - prov: 一个实现了 provider.FileContentProvider 接口的对象（本地或远程）。
func (h *ContentHandler) ServeFile(w http.ResponseWriter, r *http.Request, prov provider.FileContentProvider) {
	// 1. 从提供者获取所有必要信息
	reader := prov.GetReader()
	seeker, isSeekable := prov.GetSeeker()
	seekerTo, isSeekableTo := prov.GetSeekerTo()
	originalSize := prov.GetSize()
	originalFilename := prov.GetName()

	// 2. 解析 HTTP Range 请求头
	rangeHeader := r.Header.Get("Range")
	start, end, statusCode := parseRangeHeader(rangeHeader, originalSize)

	// 3. 根据文件能力处理 Seek 操作
	if isSeekable && seeker != nil {
		// 优先使用 io.Seeker
		_, err := seeker.Seek(start, io.SeekStart)
		if err != nil {
			log.Printf("ERROR: [ContentHandler.ServeFile] Failed to seek (io.Seeker) to position %d for file '%s': %v", start, originalFilename, err)
			http.Error(w, "Internal Server Error", http.StatusInternalServerError)
			return
		}
	} else if isSeekableTo && seekerTo != nil {
		// 降级使用 SeekTo 接口（常见于远程流）
		if err := seekerTo.SeekTo(start); err != nil {
			if err == io.EOF {
				// SeekTo 失败可能是因为请求的 offset 超出范围
				log.Printf("WARN: [ContentHandler.ServeFile] Client requested range starting at %d, which is beyond all data fragments for file '%s'.", start, originalFilename)
				w.Header().Set("Content-Range", fmt.Sprintf("bytes */%d", originalSize))
				http.Error(w, "Requested Range Not Satisfiable", http.StatusRequestedRangeNotSatisfiable)
				return
			}
			log.Printf("ERROR: [ContentHandler.ServeFile] Failed to seek (SeekerTo) to position %d for file '%s': %v", start, originalFilename, err)
			http.Error(w, "Failed to seek to requested position", http.StatusInternalServerError)
			return
		}
	} else {
		// 如果不支持任何 Seek，且不是从头开始，则无法处理 Range 请求
		if start > 0 {
			log.Printf("WARN: [ContentHandler.ServeFile] File '%s' is not seekable, but client requested a non-zero range (%d). Rejecting.", originalFilename, start)
			http.Error(w, "Seek Not Supported", http.StatusRequestedRangeNotSatisfiable)
			return
		}
	}

	// 4. 设置通用响应头
	contentType := utils.GetContentType(filepath.Ext(originalFilename))
	w.Header().Set("Content-Type", contentType)
	w.Header().Set("Content-Disposition", fmt.Sprintf("inline; filename=\"%s\"", originalFilename))
	w.Header().Set("Accept-Ranges", "bytes") // 告诉客户端我们支持 Range 请求

	// 5. 设置 Range 相关响应头
	if statusCode == http.StatusPartialContent {
		w.Header().Set("Content-Range", fmt.Sprintf("bytes %d-%d/%d", start, end, originalSize))
	}

	// 6. 设置 Content-Length 并写入状态码
	contentLength := end - start + 1
	w.Header().Set("Content-Length", fmt.Sprintf("%d", contentLength))
	w.WriteHeader(statusCode)

	// 7. 传输数据
	// 使用 io.LimitReader 确保即使底层的 reader 是无限流，我们也只发送请求范围内的数据
	readerToCopy := io.LimitReader(reader, contentLength)
	bytesWritten, err := io.Copy(w, readerToCopy)
	if err != nil {
		// 客户端断开连接是常见情况，记录为 WARN 而非 ERROR
		log.Printf("WARN: [ContentHandler.ServeFile] Stream to client was interrupted or failed for file '%s': %v", originalFilename, err)
		return
	}

	// 8. 【可选】数据完整性检查（主要用于调试）
	if bytesWritten != contentLength {
		// 这通常意味着上游数据被截断，或者 io.Copy 提前退出
		log.Printf("CRITICAL: [ContentHandler.ServeFile] DATA INTEGRITY CHECK FAILED for file '%s'. Expected to write %d bytes, but actually wrote %d bytes.", originalFilename, contentLength, bytesWritten)
	} else {
		log.Printf("INFO: [ContentHandler.ServeFile] Successfully served %d bytes for file '%s'.", bytesWritten, originalFilename)
	}
}

// parseRangeHeader 解析 HTTP Range 请求头（符合 RFC 7233）。
// 它返回起始字节、结束字节和应该使用的 HTTP 状态码。
//
// 支持的格式（参考 RFC 7233 §2.1）：
//   - "bytes=start-end"     闭区间 [start, end]
//   - "bytes=start-"        从 start 到文件末尾
//   - "bytes=-suffix"      文件最后 suffix 个字节
//   - "bytes=0-"           整个文件（等价于不带 Range）
//
// 边界处理：
//   - end > totalSize-1     → 截断为 totalSize-1，状态码 206（合法截断）
//   - start > totalSize-1   → 状态码 416
//   - start > end（用户显式）→ 状态码 416
//   - 格式无法解析         → 忽略 Range 头，按全量 200 返回
func parseRangeHeader(rangeHeader string, totalSize int64) (start, end int64, statusCode int) {
	start = 0
	end = totalSize - 1
	statusCode = http.StatusOK

	if rangeHeader == "" {
		return
	}

	// 严格只接受 bytes= 前缀
	if !strings.HasPrefix(rangeHeader, "bytes=") {
		return
	}
	spec := strings.TrimPrefix(rangeHeader, "bytes=")

	// suffix range: "-N" （最后 N 个字节）
	if strings.HasPrefix(spec, "-") {
		suffixStr := strings.TrimPrefix(spec, "-")
		suffix, err := strconv.ParseInt(suffixStr, 10, 64)
		if err != nil || suffix <= 0 {
			start = 0
			end = totalSize - 1
			statusCode = http.StatusRequestedRangeNotSatisfiable
			return
		}
		if suffix > totalSize {
			suffix = totalSize
		}
		start = totalSize - suffix
		end = totalSize - 1
		statusCode = http.StatusPartialContent
		return
	}

	// "start-end" 或 "start-"
	dashIdx := strings.Index(spec, "-")
	if dashIdx < 0 {
		// 格式无效，忽略 Range
		return
	}
	startStr := spec[:dashIdx]
	endStr := spec[dashIdx+1:]

	s, err := strconv.ParseInt(startStr, 10, 64)
	if err != nil || s < 0 {
		return
	}
	start = s

	if endStr == "" {
		// "start-" → 从 start 到文件末尾
		end = totalSize - 1
	} else {
		e, err := strconv.ParseInt(endStr, 10, 64)
		if err != nil || e < 0 {
			return
		}
		if e > totalSize-1 {
			// 合法截断（RFC 7233 §2.1：end 大于文件大小时，截断为最后一字节）
			end = totalSize - 1
		} else {
			end = e
		}
	}

	if start > end {
		start = 0
		end = totalSize - 1
		statusCode = http.StatusRequestedRangeNotSatisfiable
		return
	}
	if start >= totalSize {
		start = 0
		end = totalSize - 1
		statusCode = http.StatusRequestedRangeNotSatisfiable
		return
	}
	statusCode = http.StatusPartialContent
	return
}
