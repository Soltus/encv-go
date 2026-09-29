package encv_test

// ⚠️ 回归锁：移动端（原生播放器 mpv）与网页端（<video>/ArtPlayer）的"边下边播"
// **完全依赖 /stream 的 HTTP Range 能力** —— 播放器只是普通 HTTP 客户端，
// 它不会解密容器。2026-09-29 这条链路上只做过手工实测（206 + 跨段内容一致），
// 这里把它固化成自动用例，防止以后改坏了没人发现。
//
// 断言三件事：
//  1. 带 Range 的请求必须返回 206（不是 200 全量）—— 否则播放器会先下载整个文件；
//  2. Content-Range 里的总长必须是**明文**长度，不是容器长度 ——
//     否则播放器的进度条和 seek 范围全错；
//  3. 返回的字节必须与明文在**同一偏移**上逐字节一致，包括跨段边界的窗口
//     （段边界处 keystream 重置算错的话，长度仍然对，只有内容错 —— CTR 无认证，不报错）。
//
// 位置说明：这个测试必须放在 pkg/encv（而不是 internal/server）——
// internal/server 被 pkg/encv 依赖，在它自己的测试里 import pkg/encv 会形成
// import cycle（实测：`import cycle not allowed in test`）。

import (
	"bytes"
	"context"
	"encoding/json"
	"io"
	"net"
	"net/http"
	"net/url"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"testing"
	"time"

	"github.com/Soltus/encv-go/internal/config"
	"github.com/Soltus/encv-go/internal/server"
	"github.com/Soltus/encv-go/internal/v2/types"
	encv "github.com/Soltus/encv-go/pkg/encv"
	"github.com/Soltus/encv-go/pkg/encv/plugins"
)

func TestStreamRequest_RangeReturnsPlaintext(t *testing.T) {
	tmp := t.TempDir()
	// 挂载注册表是全局文件，隔离到本次临时目录（同 internal/server 里 webdav 测试的做法）
	t.Setenv("ENCV_MOUNTS_FILE", filepath.Join(tmp, "mounts.json"))

	// 1) 明文：5MB+，保证跨过段边界（默认段大小 4MB，见 encrypt_stream_v2_test 的用例）
	const size = 5<<20 + 12345
	plain := make([]byte, size)
	for i := range plain {
		plain[i] = byte((i*31 + 7) & 0xff)
	}
	// ⚠️ 扩展名必须落在某个插件手里：.bin 之类没有插件时 EncryptFileStreamV2 直接失败
	// （no suitable plugin found），加密这一步就会挂。用 .txt 走 text 插件。
	src := filepath.Join(tmp, "plain.txt")
	if err := os.WriteFile(src, plain, 0o600); err != nil {
		t.Fatalf("写明文失败：%v", err)
	}

	const password = "stream-range-test-pw"
	container := filepath.Join(tmp, "plain.txt.sccgt")
	if err := encv.EncryptFileStreamV2(context.Background(), src, container, password); err != nil {
		t.Fatalf("流式加密失败：%v", err)
	}
	st, err := os.Stat(container)
	if err != nil {
		t.Fatalf("stat 容器失败：%v", err)
	}
	if st.Size() == int64(size) {
		t.Fatalf("测试前提不成立：容器长度 %d 与明文相同，无法区分「总长是明文还是容器」", st.Size())
	}

	cfg := &config.Config{
		Password:       password,
		Server:         types.HttpServer{Dir: tmp},
		PluginSettings: map[string]json.RawMessage{"text": json.RawMessage(`{"ext":".sccgt"}`)},
	}
	if err := plugins.InitializeWithSettings(cfg.PluginSettings); err != nil {
		t.Fatalf("初始化插件失败：%v", err)
	}
	s := server.NewServer(config.NewContext(context.Background(), cfg), "")
	// 真起一个服务再发真实 HTTP 请求：播放器的形态就是普通 HTTP 客户端，
	// 走 httptest 直调 handler 会跳过路由/中间件，测不到真实链路。
	addr, err := s.Start("test")
	if err != nil {
		t.Fatalf("启动测试服务失败：%v", err)
	}
	defer s.Stop()

	host, port, splitErr := net.SplitHostPort(addr)
	if splitErr != nil {
		t.Fatalf("解析服务地址 %q 失败：%v", addr, splitErr)
	}
	if host == "" || host == "::" {
		host = "127.0.0.1"
	}
	baseURL := "http://" + net.JoinHostPort(host, port)

	get := func(rangeHeader string) *http.Response {
		req, err := http.NewRequest(http.MethodGet, baseURL+"/stream?path="+url.QueryEscape(container), nil)
		if err != nil {
			t.Fatalf("构造请求失败：%v", err)
		}
		if rangeHeader != "" {
			req.Header.Set("Range", rangeHeader)
		}
		res, err := (&http.Client{Timeout: 30 * time.Second}).Do(req)
		if err != nil {
			t.Fatalf("请求 /stream 失败：%v", err)
		}
		return res
	}

	// 2) 带 Range 必须是 206，且 Content-Range 的总长是**明文**长度
	const start, length = 1<<20 - 512, 4096
	res := get("bytes=" + strconv.Itoa(start) + "-" + strconv.Itoa(start+length-1))
	defer res.Body.Close()
	if res.StatusCode != http.StatusPartialContent {
		body, _ := io.ReadAll(res.Body)
		t.Fatalf("Range 请求应返回 206，实际 %d（body=%s）", res.StatusCode, string(body))
	}
	cr := res.Header.Get("Content-Range")
	if cr == "" {
		t.Fatal("206 响应缺少 Content-Range")
	}
	if !strings.HasSuffix(cr, "/"+strconv.Itoa(size)) {
		t.Fatalf("Content-Range 的总长必须是明文长度 %d，实际 %q（容器 %d）", size, cr, st.Size())
	}
	got, err := io.ReadAll(res.Body)
	if err != nil {
		t.Fatalf("读取响应体失败：%v", err)
	}
	if len(got) != length {
		t.Fatalf("应只返回 %d 字节，实际 %d", length, len(got))
	}
	if !bytes.Equal(got, plain[start:start+length]) {
		for i := range got {
			if got[i] != plain[start+i] {
				t.Fatalf("窗口内容不一致：第 %d 字节（全局偏移 %d）期望 %02x 实际 %02x", i, start+i, plain[start+i], got[i])
			}
		}
	}

	// 3) 不带 Range 时返回 200 且是完整明文（播放器/下载器的兜底路径）
	res2 := get("")
	defer res2.Body.Close()
	if res2.StatusCode != http.StatusOK {
		t.Fatalf("不带 Range 应返回 200，实际 %d", res2.StatusCode)
	}
	whole, err := io.ReadAll(res2.Body)
	if err != nil {
		t.Fatalf("读取全量响应失败：%v", err)
	}
	if int64(len(whole)) != int64(size) {
		t.Fatalf("全量响应应为明文长度 %d，实际 %d", size, len(whole))
	}
	if !bytes.Equal(whole, plain) {
		t.Fatal("全量响应与明文不一致")
	}
}
