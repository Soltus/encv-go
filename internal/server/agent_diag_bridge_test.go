package server

// agent_diag_bridge_test.go —— 远程调试诊断工具回归锁（2026-10-04）
//
// 锁三件事：
//  1. get_device_info 给出排障必需的字段（平台/版本/挂载点可用性/互联状态/索引状态）
//  2. 它**不得**把挂载点物理绝对路径吐出去（R14 脱敏 —— 工具结果要过 wire 到对端）
//  3. read_logs 的 level 过滤 / limit 上限 / since 增量都按契约工作
//
// ⚠️ 真机复验状态：这两个工具在**执行端**（安卓 APK 内的 Go 二进制）里，
//    真机端到端验证要等 APK 重新构建安装后补（本环境 adb 无设备、手机在 NAT 后）。

import (
	"context"
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/Soltus/encv-go/internal/config"
	"github.com/Soltus/encv-go/internal/logger"
)

func TestDiagGetDeviceInfo_RequiredFields(t *testing.T) {
	dir := t.TempDir()
	srv := newTestServerWithDirs(t, dir, "")

	raw, err := srv.executeDiagTool(context.Background(), "get_device_info", `{}`)
	if err != nil {
		t.Fatalf("get_device_info 执行失败: %v", err)
	}

	var out map[string]interface{}
	if err := json.Unmarshal([]byte(raw), &out); err != nil {
		t.Fatalf("返回不是 JSON: %v (%s)", err, raw)
	}
	if _, ok := out["error"]; ok {
		t.Fatalf("不应返回错误载荷: %s", raw)
	}

	for _, key := range []string{"platform", "version", "uptimeMs", "now", "mounts", "edge", "searchIndex", "peerlink"} {
		if _, ok := out[key]; !ok {
			t.Errorf("缺少字段 %q", key)
		}
	}
	plat, _ := out["platform"].(map[string]interface{})
	if plat == nil || plat["goos"] == "" || plat["pid"] == nil {
		t.Errorf("platform 缺 goos/pid: %v", out["platform"])
	}
	// 互联未启动 ⇒ running=false（不得谎报连上）
	edge, _ := out["edge"].(map[string]interface{})
	if edge == nil || edge["running"] != false {
		t.Errorf("未启动互联时 edge.running 应为 false: %v", out["edge"])
	}
	// 索引状态必须是四态之一（或 unavailable）
	si, _ := out["searchIndex"].(map[string]interface{})
	state, _ := si["state"].(string)
	switch state {
	case "building", "empty", "partial", "ready", "unavailable":
	default:
		t.Errorf("searchIndex.state 非法: %q", state)
	}
	// 挂载点必须给出可用性与 id（不含物理路径，见下一条用例）
	mounts, _ := out["mounts"].([]interface{})
	if len(mounts) != 1 {
		t.Fatalf("mounts 数 = %d, want 1", len(mounts))
	}
	m0, _ := mounts[0].(map[string]interface{})
	if m0["id"] != "serving" || m0["available"] != true {
		t.Errorf("mounts[0] 形态不对: %v", m0)
	}
}

// TestDiagGetDeviceInfo_WebBundleInstalled —— I4：主应用 SPA 热更包的"装没装/装了哪版"
//
// 用途：云控下发 web 包之后，**远程**确认它到底生没生效（Kotlin 侧只在目录同时有
// index.html 与 version.json 时才会切过去），而不是靠"看起来变了"判断。
func TestDiagGetDeviceInfo_WebBundleInstalled(t *testing.T) {
	root := t.TempDir()
	t.Setenv("ENCV_WEB_BUNDLE_DIR", filepath.Join(root, "web-bundle"))
	dir := config.AppDataDir("web-bundle")

	// ① 没装 ⇒ installed=false
	srv := newTestServerWithDirs(t, t.TempDir(), "")
	raw, err := srv.executeDiagTool(context.Background(), "get_device_info", `{}`)
	if err != nil {
		t.Fatalf("执行失败: %v", err)
	}
	if !strings.Contains(raw, `"webBundle":{"installed":false}`) {
		t.Fatalf("未安装时应 installed=false: %s", raw)
	}

	// ② 装了（index.html + version.json）⇒ installed=true + 版本
	if err := os.MkdirAll(dir, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(dir, "index.html"), []byte("<html>hot</html>"), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(dir, "version.json"), []byte(`{"version":"v-hot-1","updatedAt":"2026-10-05T00:00:00Z"}`), 0o644); err != nil {
		t.Fatal(err)
	}
	raw, err = srv.executeDiagTool(context.Background(), "get_device_info", `{}`)
	if err != nil {
		t.Fatalf("执行失败: %v", err)
	}
	if !strings.Contains(raw, `"installed":true`) || !strings.Contains(raw, "v-hot-1") {
		t.Fatalf("已安装时应回显 installed=true 与版本: %s", raw)
	}
	// ③ 缺 index.html（只有 version.json）⇒ 仍算没装（与 Kotlin 侧判定必须一致）
	if err := os.Remove(filepath.Join(dir, "index.html")); err != nil {
		t.Fatal(err)
	}
	raw, _ = srv.executeDiagTool(context.Background(), "get_device_info", `{}`)
	if !strings.Contains(raw, `"installed":false`) {
		t.Fatalf("缺 index.html 应判未安装（Kotlin 侧同判定）: %s", raw)
	}
}

// TestDiagGetDeviceInfo_NoPhysicalPath —— 脱敏锁：物理绝对路径不得过 wire
func TestDiagGetDeviceInfo_NoPhysicalPath(t *testing.T) {
	dir := t.TempDir()
	srv := newTestServerWithDirs(t, dir, "")

	raw, err := srv.executeDiagTool(context.Background(), "get_device_info", `{}`)
	if err != nil {
		t.Fatalf("执行失败: %v", err)
	}
	if strings.Contains(raw, dir) {
		t.Fatalf("get_device_info 泄漏了挂载点物理路径 %q: %s", dir, raw)
	}
	// 反向验证：这条断言确实有牙齿（dir 是真实存在的临时目录）
	if dir == "" {
		t.Fatal("测试自身失效：dir 为空")
	}
}

func TestDiagReadLogs_LevelFilterAndLimit(t *testing.T) {
	// 造确定性数据：3 条 error + 1 条 info（不清理全局缓冲，避免影响其它用例）
	for i := 0; i < 3; i++ {
		logger.DefaultLogBuffer.Push(map[string]string{
			"level":     "error",
			"message":   "diag-test-error",
			"timestamp": time.Now().Format("15:04:05"),
			"source":    "diag_test",
		})
	}
	logger.DefaultLogBuffer.Push(map[string]string{
		"level":     "info",
		"message":   "diag-test-info",
		"timestamp": time.Now().Format("15:04:05"),
		"source":    "diag_test",
	})

	srv := newTestServerWithDirs(t, t.TempDir(), "")

	// ① level=error ⇒ 只剩 error，且 limit 生效
	raw, err := srv.executeDiagTool(context.Background(), "read_logs", `{"level":"error","limit":2}`)
	if err != nil {
		t.Fatalf("read_logs 执行失败: %v", err)
	}
	var out struct {
		Count   int                 `json:"count"`
		Matched int                 `json:"matched"`
		Items   []map[string]string `json:"items"`
	}
	if err := json.Unmarshal([]byte(raw), &out); err != nil {
		t.Fatalf("返回不是 JSON: %v (%s)", err, raw)
	}
	if out.Count > 2 {
		t.Fatalf("limit=2 时 count = %d，上限失效", out.Count)
	}
	for _, it := range out.Items {
		if it["level"] != "error" {
			t.Fatalf("level 过滤失效，混入 %q: %v", it["level"], it)
		}
	}
	// ② 不过滤时至少能拿到刚写的 error（证明过滤没把合法数据吃掉）
	raw2, _ := srv.executeDiagTool(context.Background(), "read_logs", `{"limit":200}`)
	var out2 struct {
		Items []map[string]string `json:"items"`
	}
	_ = json.Unmarshal([]byte(raw2), &out2)
	gotErr := 0
	gotInfo := 0
	for _, it := range out2.Items {
		switch it["message"] {
		case "diag-test-error":
			gotErr++
		case "diag-test-info":
			gotInfo++
		}
	}
	if gotErr < 1 || gotInfo < 1 {
		t.Fatalf("未过滤时应同时拿到两类日志, got error=%d info=%d", gotErr, gotInfo)
	}
}

// TestDiagReadLogs_Since 增量拉取：since 之后的新日志才返回
func TestDiagReadLogs_Since(t *testing.T) {
	srv := newTestServerWithDirs(t, t.TempDir(), "")

	// 先塞一条"旧"日志（时间戳故意取很早），再用 since 过滤掉它
	logger.DefaultLogBuffer.Push(map[string]string{
		"level":     "error",
		"message":   "diag-test-old",
		"timestamp": "00:00:01",
		"source":    "diag_test",
	})
	raw, err := srv.executeDiagTool(context.Background(), "read_logs", `{"since":"23:59:59","limit":200}`)
	if err != nil {
		t.Fatalf("执行失败: %v", err)
	}
	if strings.Contains(raw, "diag-test-old") {
		// 时间戳 00:00:01 不大于 since 23:59:59 ⇒ 必须被过滤掉
		// （环形缓冲跨天会退化成字符串比较，与 /api/logs/recent 既有实现同语义）
		t.Fatalf("since 过滤失效，旧日志仍返回: %s", raw)
	}
}

// TestListAgentTools_ContainsDiagTools —— 新工具必须进注册表（否则 LLM / 远程调用都看不到）
func TestListAgentTools_ContainsDiagTools(t *testing.T) {
	srv := newTestServerWithDirs(t, t.TempDir(), "")
	tools := srv.ListAgentTools()

	want := map[string]bool{"get_device_info": false, "read_logs": false}
	for _, tl := range tools {
		name, _ := tl["name"].(string)
		if _, ok := want[name]; ok {
			nc, isBool := tl["needConfirm"].(bool)
			if !isBool || nc {
				t.Errorf("%s 应为 needConfirm=false（只读诊断）", name)
			}
			want[name] = true
		}
	}
	for name, found := range want {
		if !found {
			t.Errorf("ListAgentTools 缺少诊断工具 %s", name)
		}
	}
}

// TestExecuteAgentTool_DispatchDiag —— 派发锁：executeAgentTool 必须能路由到诊断工具
func TestExecuteAgentTool_DispatchDiag(t *testing.T) {
	srv := newTestServerWithDirs(t, t.TempDir(), "")

	raw, err := srv.executeAgentTool(context.Background(), "get_device_info", `{}`)
	if err != nil {
		t.Fatalf("executeAgentTool 未路由到诊断工具: %v", err)
	}
	if !strings.Contains(raw, "searchIndex") {
		t.Fatalf("派发结果不像 get_device_info: %s", raw)
	}
	// 未知工具仍要报错（不能因为新增分支就吞掉错误）
	if _, err := srv.executeDiagTool(context.Background(), "no_such_diag_tool", `{}`); err == nil {
		t.Fatal("未知诊断工具应返回 error")
	}
}
