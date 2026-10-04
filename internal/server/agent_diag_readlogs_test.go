package server

// agent_diag_readlogs_test.go —— 远程读日志的**过滤契约**（2026-10-05）
//
// 目标（docs/remote-logs.md）：
//   1. 远程拿到的日志与 DevLogs **同款** ⇒ source=all 要同时含 Go 日志与前端日志；
//   2. 等级（多级别）+ 关键词 + 来源 + 时间增量，**全部在执行端过滤**
//      ⇒ 调用方只拿到结果，链路上不搬运无关日志（这是本工具存在的理由）。
//
// 反向锁同样重要：不传过滤条件时必须全量返回（不得因为"没传关键词"就一条都不给）。

import (
	"encoding/json"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/Soltus/encv-go/internal/logger"
	"github.com/gin-gonic/gin"
)

// httptestPostJSON 走本机路由发一个 JSON POST（上报端点不在 peerlink 注册点，
// 所以这里单独构造一个只挂该端点的 gin engine）。
func httptestPostJSON(t *testing.T, _ *gin.Engine, path, body string) *httptest.ResponseRecorder {
	t.Helper()
	gin.SetMode(gin.TestMode)
	r := gin.New()
	s := &Server{}
	r.POST(path, s.handleAPILogsFrontendGin)
	rec := httptest.NewRecorder()
	req := httptest.NewRequest("POST", path, strings.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	r.ServeHTTP(rec, req)
	return rec
}

// resetLogBuffers 让每个用例在**干净**的缓冲上断言。
//
// ⚠️ 两个缓冲都是包级全局：不隔离的话，前一个用例推进去的日志会让计数断言随机失真
//    （首次跑就是这样红的：matched 期望 1 实得 3）。
func resetLogBuffers(t *testing.T) {
	t.Helper()
	oldGo, oldFe := logger.DefaultLogBuffer, frontendLogBuffer
	logger.DefaultLogBuffer = logger.NewRingBuffer(500)
	frontendLogBuffer = logger.NewRingBuffer(300)
	t.Cleanup(func() {
		logger.DefaultLogBuffer, frontendLogBuffer = oldGo, oldFe
	})
}

func pushGoLogs(t *testing.T, pairs ...[2]string) {
	t.Helper()
	for _, p := range pairs {
		logger.DefaultLogBuffer.Push(map[string]string{
			"timestamp": p[0], "level": p[1], "message": p[1] + " message", "source": "ws_log_handler",
		})
	}
}

func pushFrontendLogs(t *testing.T, pairs ...[2]string) {
	t.Helper()
	for _, p := range pairs {
		frontendLogBuffer.Push(map[string]string{
			"timestamp": p[0], "level": p[1], "message": "[scan] " + p[1], "source": "frontend", "origin": "console." + p[1],
		})
	}
}

func readLogsResult(t *testing.T, argsJSON string) map[string]any {
	t.Helper()
	s := &Server{}
	raw, err := s.diagReadLogs(argsJSON)
	if err != nil {
		t.Fatalf("read_logs 执行失败: %v", err)
	}
	var out map[string]any
	if err := json.Unmarshal([]byte(raw), &out); err != nil {
		t.Fatalf("响应不是 JSON: %v (%s)", err, raw)
	}
	return out
}

func itemsOf(out map[string]any) []map[string]string {
	raw, _ := json.Marshal(out["items"])
	var items []map[string]string
	_ = json.Unmarshal(raw, &items)
	return items
}

func sourcesOf(items []map[string]string) map[string]int {
	m := map[string]int{}
	for _, it := range items {
		m[it["source"]]++
	}
	return m
}

// TestDiagReadLogs_AllIsDevLogsEquivalent —— source=all 必须两源都给（DevLogs 同款）
func TestDiagReadLogs_AllIsDevLogsEquivalent(t *testing.T) {
	resetLogBuffers(t)
	pushGoLogs(t, [2]string{"10:00:01", "error"}, [2]string{"10:00:02", "info"})
	pushFrontendLogs(t, [2]string{"10:00:03", "warn"})

	out := readLogsResult(t, `{"source":"all","limit":200}`)
	got := sourcesOf(itemsOf(out))
	if got["ws_log_handler"] == 0 || got["frontend"] == 0 {
		t.Fatalf("source=all 必须同时含后端与前端日志（DevLogs 同款）, got %+v", got)
	}
}

// TestDiagReadLogs_SourceFilter —— 只想要某一源时必须真的过滤掉另一源
func TestDiagReadLogs_SourceFilter(t *testing.T) {
	resetLogBuffers(t)
	pushGoLogs(t, [2]string{"11:00:01", "error"})
	pushFrontendLogs(t, [2]string{"11:00:02", "error"})

	if got := sourcesOf(itemsOf(readLogsResult(t, `{"source":"frontend","limit":200}`))); got["ws_log_handler"] != 0 {
		t.Fatalf("source=frontend 不得返回后端日志, got %+v", got)
	}
	if got := sourcesOf(itemsOf(readLogsResult(t, `{"source":"go","limit":200}`))); got["frontend"] != 0 {
		t.Fatalf("source=go 不得返回前端日志, got %+v", got)
	}
}

// TestDiagReadLogs_KeywordFilterIsCaseInsensitive —— 关键词过滤（大小写不敏感）
func TestDiagReadLogs_KeywordFilterIsCaseInsensitive(t *testing.T) {
	resetLogBuffers(t)
	pushFrontendLogs(t, [2]string{"12:00:01", "error"}) // message = "[scan] error"

	out := readLogsResult(t, `{"keyword":"SCAN","limit":200}`)
	items := itemsOf(out)
	if len(items) == 0 {
		t.Fatal("大写关键词应命中含小写 scan 的日志（大小写不敏感）")
	}
	if n, _ := out["matched"].(float64); n != 1 {
		t.Fatalf("matched 应为 1, got %v", out["matched"])
	}
	// 不命中的关键词 ⇒ 一条都不给
	if items := itemsOf(readLogsResult(t, `{"keyword":"zzz-no-such","limit":200}`)); len(items) != 0 {
		t.Fatalf("无命中应返回空, got %d 条", len(items))
	}
}

// TestDiagReadLogs_LevelsMulti —— 多级别过滤（levels 优先于 level）
func TestDiagReadLogs_LevelsMulti(t *testing.T) {
	resetLogBuffers(t)
	pushGoLogs(t, [2]string{"13:00:01", "debug"}, [2]string{"13:00:02", "info"}, [2]string{"13:00:03", "warn"}, [2]string{"13:00:04", "error"})

	items := itemsOf(readLogsResult(t, `{"levels":["warn","error"],"source":"go","limit":200}`))
	if len(items) != 2 {
		t.Fatalf("levels=[warn,error] 应只剩 2 条, got %d", len(items))
	}
	for _, it := range items {
		if it["level"] != "warn" && it["level"] != "error" {
			t.Fatalf("混入其它级别: %+v", it)
		}
	}
	// 旧参数 level 单值仍然可用
	if items := itemsOf(readLogsResult(t, `{"level":"error","source":"go","limit":200}`)); len(items) != 1 {
		t.Fatalf("level=error 应只剩 1 条, got %d", len(items))
	}
}

// TestDiagReadLogs_NoFilterReturnsAll —— 反向锁：不传过滤条件 ⇒ 全量（不得一条都不给）
func TestDiagReadLogs_NoFilterReturnsAll(t *testing.T) {
	resetLogBuffers(t)
	pushGoLogs(t, [2]string{"14:00:01", "info"})
	out := readLogsResult(t, `{"limit":200}`)
	if len(itemsOf(out)) == 0 {
		t.Fatal("不带任何过滤时必须返回日志（空 keyword/levels 不得变成'全不匹配'）")
	}
	if n, _ := out["matched"].(float64); n < 1 {
		t.Fatalf("matched 应 >=1, got %v", out["matched"])
	}
}

// TestDiagReadLogs_LimitAndTruncated —— 上限与"还有更多"提示
func TestDiagReadLogs_LimitAndTruncated(t *testing.T) {
	resetLogBuffers(t)
	for i := 0; i < 10; i++ {
		ts := "15:00:0" + string(rune('0'+i))
		pushGoLogs(t, [2]string{ts, "info"})
	}
	out := readLogsResult(t, `{"source":"go","limit":3}`)
	items := itemsOf(out)
	if len(items) != 3 {
		t.Fatalf("limit=3 ⇒ 应返回 3 条, got %d", len(items))
	}
	if tr, _ := out["truncated"].(bool); !tr {
		t.Fatal("命中多于 limit 时 truncated 必须为 true（调用方据此继续拉）")
	}
	// 返回的必须是**最近**的 3 条（时间正序）
	if items[0]["timestamp"] != "15:00:07" {
		t.Fatalf("应返回最近 3 条, got first=%s", items[0]["timestamp"])
	}
}

// TestFrontendLogsEndpoint_BatchLimits —— 上报端点的条数与长度上限（防把缓冲撑爆）
func TestFrontendLogsEndpoint_BatchLimits(t *testing.T) {
	resetLogBuffers(t)
	gin.SetMode(gin.TestMode)
	r, s := newPeerlinkRouter()
	_ = s
	items := make([]map[string]string, 0, 120)
	for i := 0; i < 120; i++ {
		items = append(items, map[string]string{"level": "info", "message": strings.Repeat("x", 1500)})
	}
	body, _ := json.Marshal(map[string]any{"items": items})
	rec := httptestPostJSON(t, r, "/api/logs/frontend", string(body))
	if rec.Code != 200 {
		t.Fatalf("上报端点期望 200, got %d: %s", rec.Code, rec.Body.String())
	}
	var out struct {
		Received int `json:"received"`
	}
	if err := json.Unmarshal(rec.Body.Bytes(), &out); err != nil {
		t.Fatal(err)
	}
	if out.Received != frontendLogBatchMax {
		t.Fatalf("单批应最多接收 %d 条, got %d", frontendLogBatchMax, out.Received)
	}
	// 超长 message 必须被截断（回环也不能把环形缓冲撑爆）
	for _, e := range frontendLogBuffer.Snapshot() {
		if e["source"] == "frontend" && len([]rune(e["message"])) > frontendLogMsgMaxChars+20 {
			t.Fatalf("message 未被截断: len=%d", len([]rune(e["message"])))
		}
	}
}
