package utils

import (
	"bytes"
	"errors"
	"fmt"
	"io"
	"log"
	"net"
	"net/http"
	"strconv"
	"strings"
	"syscall"
	"time"
)

// isAddrInUseErr 检查错误是否为“地址已在使用”
func IsAddrInUseErr(err error) bool {
	var opErr *net.OpError
	if errors.As(err, &opErr) {
		// syscall.EADDRINUSE 是标准的“地址已在使用”错误码
		if errors.Is(opErr.Err, syscall.EADDRINUSE) {
			return true
		}
		// 在某些系统（如 Windows）上，错误消息可能不同，进行字符串匹配作为后备
		if strings.Contains(opErr.Error(), "address already in use") ||
			strings.Contains(opErr.Error(), "An address incompatible with the requested protocol was used") {
			return true
		}
	}
	return false
}

// downloadRange 下载 URL 的指定字节范围
func DownloadRange(url string, headers map[string]string, start, end int64) ([]byte, error) {
	req, err := http.NewRequest("GET", url, nil)
	if err != nil {
		return nil, fmt.Errorf("failed to create range request for %s: %w", url, err)
	}
	req.Header.Set("Range", fmt.Sprintf("bytes=%d-%d", start, end))
	for k, v := range headers {
		req.Header.Set(k, v)
	}

	client := &http.Client{Timeout: 30 * time.Second}
	resp, err := client.Do(req)
	if err != nil {
		return nil, fmt.Errorf("failed to execute range request for %s: %w", url, err)
	}
	defer resp.Body.Close()

	// 检查服务器是否支持范围请求
	if resp.StatusCode != http.StatusPartialContent {
		return nil, fmt.Errorf("server does not support range requests for %s, status: %s", url, resp.Status)
	}

	return io.ReadAll(resp.Body)
}

// ReadAllFromURL 从指定的 URL 下载所有数据，并返回为字节切片
func ReadAllFromURL(url string, headers map[string]string) ([]byte, error) {
	// 1. 创建 HTTP GET 请求
	req, err := http.NewRequest("GET", url, nil)
	if err != nil {
		return nil, fmt.Errorf("failed to create request for %s: %w", url, err)
	}

	// 2. 添加自定义请求头
	for k, v := range headers {
		req.Header.Set(k, v)
	}

	// 3. 执行请求
	client := &http.Client{Timeout: 30 * time.Second}
	resp, err := client.Do(req)
	if err != nil {
		return nil, fmt.Errorf("failed to execute request for %s: %w", url, err)
	}
	defer resp.Body.Close()

	// 4. 检查响应状态码
	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("server returned non-200 status for %s: %s", url, resp.Status)
	}

	// 5. 读取所有响应体数据
	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, fmt.Errorf("failed to read response body from %s: %w", url, err)
	}

	return body, nil
}

// 创建一个 HTTP GET 请求并返回响应体的 ReadCloser
func GetRemoteStream(fileURL string, headers map[string]string) (io.ReadCloser, error) {
	req, err := http.NewRequest("GET", fileURL, nil)
	if err != nil {
		return nil, fmt.Errorf("failed to create request for %s: %w", fileURL, err)
	}

	for key, value := range headers {
		req.Header.Add(key, value)
	}

	client := &http.Client{Timeout: 30 * time.Second}
	resp, err := client.Do(req)
	if err != nil {
		return nil, fmt.Errorf("failed to execute request for %s: %w", fileURL, err)
	}

	if resp.StatusCode != http.StatusOK {
		resp.Body.Close()
		return nil, fmt.Errorf("remote server returned status %s for %s", resp.Status, fileURL)
	}

	return resp.Body, nil
}

// 创建一个带有正确认证头的 HTTP 请求
func MakeAuthenticatedRequest(method, url, body, token string) (*http.Response, error) {
	var reqBody io.Reader
	if body != "" {
		reqBody = bytes.NewBuffer([]byte(body))
	}

	req, err := http.NewRequest(method, url, reqBody)
	if err != nil {
		return nil, fmt.Errorf("failed to create request: %w", err)
	}

	req.Header.Add("Content-Type", "application/json")

	// --- 关键修正：根据 Token 格式决定是否添加 Bearer 前缀 ---
	if strings.Contains(token, ".") {
		// JWT 格式，使用 Bearer 前缀
		req.Header.Add("Authorization", "Bearer "+token)
		log.Printf("-> [Auth Debug] Using Bearer token for request to %s", url)
	} else {
		// 永久 Token 格式，直接使用
		req.Header.Add("Authorization", token)
		log.Printf("-> [Auth Debug] Using permanent token for request to %s", url)
	}

	client := &http.Client{Timeout: 30 * time.Second}
	return client.Do(req)
}

// isConnectionClosedError 判断错误是否由客户端断开连接引起
func IsConnectionClosedError(err error) bool {
	// 处理 Go 1.16+ 的特定错误
	if errors.Is(err, net.ErrClosed) {
		return true
	}
	// 处理旧版本或更通用的错误
	errStr := err.Error()
	return strings.Contains(errStr, "connection reset by peer") || strings.Contains(errStr, "broken pipe")
}

// GetRemoteStreamWithRange 发起一个 HTTP Range 请求并返回响应体。
// start 和 end 定义了字节范围。负数表示从文件末尾开始计算（例如 start=-32 表示最后32字节）。
// 调用者负责关闭返回的 resp.Body。
func GetRemoteStreamWithRange(url string, headers map[string][]string, start, end int64) (*http.Response, error) {
	req, err := http.NewRequest("GET", url, nil)
	if err != nil {
		return nil, fmt.Errorf("failed to create request: %w", err)
	}

	// 构造 Range 请求头
	var rangeStr string
	if start >= 0 && end >= 0 {
		rangeStr = fmt.Sprintf("bytes=%d-%d", start, end)
	} else if start < 0 {
		rangeStr = fmt.Sprintf("bytes=%d", start)
	} else if end < 0 {
		rangeStr = fmt.Sprintf("bytes=%d-", start)
	} else {
		return nil, fmt.Errorf("invalid range specified: start=%d, end=%d", start, end)
	}
	req.Header.Set("Range", rangeStr)

	// 复制其他请求头
	for key, values := range headers {
		for _, v := range values {
			req.Header.Set(key, v)
		}
	}

	// 【P0 修复】默认 30s 超时，防止上游 hang 时整测试卡死
	client := &http.Client{Timeout: 30 * time.Second}
	resp, err := client.Do(req)
	if err != nil {
		// 【P0 修复附赠】原 return 0 在 *http.Response 返回类型下编译失败，改为 nil
		return nil, fmt.Errorf("failed to execute request: %w", err)
	}

	// 【关键修复】处理 416 错误
	if resp.StatusCode == http.StatusRequestedRangeNotSatisfiable {
		log.Printf("WARN: [GetRemoteStreamWithRange] Range %d-%d not satisfiable for %s. Trying to correct.", start, end, url)
		// 尝试从 Content-Range 响应头获取文件总大小
		contentRange := resp.Header.Get("Content-Range")
		if contentRange != "" {
			// 格式通常是 "bytes */<total_length>"
			parts := strings.Split(contentRange, "/")
			if len(parts) == 2 {
				totalSize, parseErr := strconv.ParseInt(parts[1], 10, 64)
				if parseErr == nil {
					// 关闭原始响应
					resp.Body.Close()

					// 如果请求的起始位置已经超出文件末尾，则无法修正
					if start >= totalSize {
						return nil, fmt.Errorf("invalid range: start (%d) is beyond file size (%d)", start, totalSize)
					}

					// 修正结束位置
					correctedEnd := totalSize - 1
					log.Printf("DEBUG: [GetRemoteStreamWithRange] Retrying with corrected range: %d-%d", start, correctedEnd)

					// 递归调用，使用修正后的范围
					return GetRemoteStreamWithRange(url, headers, start, correctedEnd)
				}
			}
		}
		// 如果无法解析 Content-Range，则返回原始错误
		resp.Body.Close()
		return nil, fmt.Errorf("unexpected status code: %d (416 Range Not Satisfiable)", resp.StatusCode)
	}

	// 检查状态码，206是成功的范围请求，200也接受（某些服务器对整个文件请求也返回200）
	if resp.StatusCode != http.StatusPartialContent && resp.StatusCode != http.StatusOK {
		// 【关键修复】记录下被拒绝的请求的真实状态码和URL
		log.Printf("ERROR: [GetRemoteStreamWithRange] FAILED for URL '%s' with range '%s'. Server responded with status: %d", url, rangeStr, resp.StatusCode)
		resp.Body.Close()
		return nil, fmt.Errorf("unexpected status code: %d", resp.StatusCode)
	}
	// 【关键验证】在返回响应前，记录服务器承诺发送的长度
	if resp.StatusCode == http.StatusPartialContent || resp.StatusCode == http.StatusOK {
		log.Printf("DEBUG: [GetRemoteStreamWithRange] Range '%s' -> Status: %s, Server-Returned Content-Length: %s, Content-Type: %s", rangeStr, resp.Status, resp.Header.Get("Content-Length"), resp.Header.Get("Content-Type"))
	}

	return resp, nil
}

// 使用端口递增和自检逻辑启动后端服务，使用前需要实现 /ping 路由返回  types.PingResponse （防止共享端口劫持）
// 返回成功监听的地址
// func StartBackendServerWithRetry(handler http.Handler, initialPort int, instanceID string) (string, error) {
// 	maxTries := 100
// 	for i := 0; i < maxTries; i++ {
// 		currentPort := initialPort + i
// 		addr := fmt.Sprintf(":%d", currentPort)

// 		listener, err := net.Listen("tcp", addr)
// 		if err != nil {
// 			if IsAddrInUseErr(err) {
// 				log.Printf("Backend: Port %d is in use, trying next port...", currentPort)
// 			} else {
// 				log.Printf("Backend: Failed to listen on port %d: %v. Trying next port...", currentPort, err)
// 			}
// 			continue
// 		}

// 		backendServer := &http.Server{Handler: handler}
// 		serveErrChan := make(chan error, 1)
// 		go func() {
// 			log.Printf("Backend: Attempting to start on %s...", addr)
// 			serveErrChan <- backendServer.Serve(listener)
// 		}()

// 		time.Sleep(150 * time.Millisecond) // 给服务器一点启动时间

// 		// 【关键】执行后端服务的自检
// 		pingURL := fmt.Sprintf("http://127.0.0.1:%d/ping", currentPort)
// 		client := &http.Client{Timeout: 500 * time.Millisecond}
// 		resp, err := client.Get(pingURL)

// 		if err != nil || resp == nil || resp.StatusCode != http.StatusOK {
// 			log.Printf("Backend: Self-check failed on port %d (err: %v). Trying next port...", currentPort, err)
// 			if resp != nil {
// 				resp.Body.Close()
// 			}
// 			backendServer.Close()
// 			<-serveErrChan
// 			continue
// 		}

// 		var pingResp types.PingResponse
// 		if err := json.NewDecoder(resp.Body).Decode(&pingResp); err != nil {
// 			log.Printf("Backend: Self-check failed on port %d: could not decode ping response. Trying next port...", currentPort)
// 			resp.Body.Close()
// 			backendServer.Close()
// 			<-serveErrChan
// 			continue
// 		}
// 		resp.Body.Close()

// 		if pingResp.InstanceID != instanceID {
// 			log.Printf("Backend: Port %d is hijacked. Expected: %s, Got: %s. Trying next port...",
// 				currentPort, instanceID, pingResp.InstanceID)
// 			backendServer.Close()
// 			<-serveErrChan
// 			continue
// 		}

// 		// 【关键】后端服务自检成功！
// 		actualAddr := listener.Addr().String()
// 		log.Printf("✅ Backend server successfully started and listening on %s (self-check passed)", actualAddr)

// 		// 在后台 goroutine 中处理服务器错误
// 		go func() {
// 			if err := <-serveErrChan; err != nil && err != http.ErrServerClosed {
// 				log.Printf("Backend server on %s encountered an error: %v", actualAddr, err)
// 			}
// 		}()

// 		return actualAddr, nil
// 	}

// 	return "", fmt.Errorf("failed to start backend server after %d tries", maxTries)
// }
