package server

// peerlink_agent_idem_test.go —— 幂等表的契约锁
//
// 覆盖范围刻意贴近真实事故面（2026-10-04：同一 callId 被执行两次）：
//   - 重复提交必须命中同一个 entry（不得再起一次执行 / 再弹一次审批）
//   - 并发重复必须**共用同一次执行结果**（不是各跑各的）
//   - 失败必须允许重试（不能把 decline / 报错固化下来）
//   - 有界 + 过期要真的生效（长期运行不能无限涨内存）

import (
	"strconv"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/Soltus/encv-go/internal/peerlink"
)

func TestAgentIdemTable_SecondBeginIsNotNew(t *testing.T) {
	tab := newAgentIdemTable()
	now := time.Now()

	e1, new1 := tab.begin("k1", now)
	if !new1 {
		t.Fatal("首次 begin 应为 new")
	}
	e2, new2 := tab.begin("k1", now.Add(time.Millisecond))
	if new2 {
		t.Fatal("相同 key 的第二次 begin 必须是 (同一个 entry, false) —— 否则会重复执行")
	}
	if e1 != e2 {
		t.Fatal("重复提交必须拿到同一个 entry，才能共用结果")
	}
}

func TestAgentIdemTable_DuplicateWaitsAndSharesResult(t *testing.T) {
	tab := newAgentIdemTable()
	now := time.Now()

	e, new := tab.begin("k1", now)
	if !new {
		t.Fatal("首次应 new")
	}

	var wg sync.WaitGroup
	var got peerlink.AgentInvokeOutcome
	wg.Add(1)
	go func() {
		defer wg.Done()
		// 模拟"重复提交"：等在首次执行上
		<-e.done
		got = e.out
	}()

	// 首次执行"慢慢完成"
	time.Sleep(20 * time.Millisecond)
	e.out = peerlink.AgentInvokeOutcome{Result: []byte(`{"ok":1}`)}
	close(e.done)
	wg.Wait()

	if string(got.Result) != `{"ok":1}` {
		t.Fatalf("重复调用应拿到首次执行的结果: %q", string(got.Result))
	}
}

func TestAgentIdemTable_FailureIsNotReused(t *testing.T) {
	tab := newAgentIdemTable()
	now := time.Now()

	e, _ := tab.begin("k1", now)
	e.out = peerlink.AgentInvokeOutcome{Err: errStub}
	close(e.done)
	tab.drop("k1") // 失败后抹掉（handler 在 out.Err != nil 时会这么做）

	e2, new2 := tab.begin("k1", now.Add(time.Second))
	if !new2 {
		t.Fatal("执行失败后应允许重试 —— 幂等表必须已把该 key 抹掉")
	}
	if e2.out.Err != nil {
		t.Fatal("重试不应拿到上次的失败结果")
	}
}

func TestAgentIdemTable_SuccessISReused(t *testing.T) {
	tab := newAgentIdemTable()
	now := time.Now()

	e, _ := tab.begin("k1", now)
	e.out = peerlink.AgentInvokeOutcome{Result: []byte(`{"v":42}`)}
	close(e.done)
	// 成功：不 drop

	e2, new2 := tab.begin("k1", now.Add(time.Second))
	if new2 {
		t.Fatal("成功结果必须被后续重复提交复用（这正是幂等的价值）")
	}
	if string(e2.out.Result) != `{"v":42}` {
		t.Fatalf("应沿用首次成功结果: %q", string(e2.out.Result))
	}
}

func TestAgentIdemTable_TTLExpiry(t *testing.T) {
	tab := newAgentIdemTable()
	now := time.Now()

	tab.begin("k1", now)

	// 超过 TTL 后再来 ⇒ 视为全新请求（旧记录必须先被淘汰）
	_, new2 := tab.begin("k1", now.Add(agentIdemTTL+time.Second))
	if !new2 {
		t.Fatal("超过 TTL 的记录应被淘汰，否则长期运行会白白占内存")
	}
}

func TestAgentIdemTable_BoundedByLimit(t *testing.T) {
	tab := newAgentIdemTable()
	now := time.Now()

	for i := 0; i < agentIdemLimit+20; i++ {
		tab.begin(keyFor(i), now)
	}
	if len(tab.entries) > agentIdemLimit {
		t.Fatalf("幂等表必须有界: got %d > limit %d", len(tab.entries), agentIdemLimit)
	}
	if len(tab.order) != len(tab.entries) {
		t.Fatalf("order 与 entries 不一致: %d vs %d", len(tab.order), len(tab.entries))
	}
}

// TestAgentIdemTable_ConcurrentOnlyOneRunner 最贴近事故场景：
// 多个重复请求同时进来，实际"干活"的只能有一个。
func TestAgentIdemTable_ConcurrentOnlyOneRunner(t *testing.T) {
	tab := newAgentIdemTable()
	now := time.Now()

	var runs int32
	var wg sync.WaitGroup
	for i := 0; i < 10; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			e, isNew := tab.begin("same-key", now)
			if !isNew {
				<-e.done // 重复者只需等待
				return
			}
			atomic.AddInt32(&runs, 1) // 只有"首次者"会走到这里
			e.out = peerlink.AgentInvokeOutcome{Result: []byte(`{"ran":1}`)}
			close(e.done)
		}()
	}
	wg.Wait()

	if runs != 1 {
		t.Fatalf("并发重复提交实际执行了 %d 次，应恰好 1 次", runs)
	}
}

// TestPeerAgentInvoke_IdempotentByCallId 端到端锁：
// **同一 callId 的重复调用必须只真正执行一次**（2026-10-04 的幂等修复接线验证）。
//
// 观测手段：每一次真正执行都会先过 Approver.Require ⇒ 放行时写一条带 CallId 的审计，
// 于是"审计里该 callId 有几条"就等价于"工具被执行了几次"—— 这样不必注入任何替身，
// 就能在真实代码路径上判定幂等是否生效。
func TestPeerAgentInvoke_IdempotentByCallId(t *testing.T) {
	dir := t.TempDir()
	srv := newTestServerWithDirs(t, dir, dir)

	// 先信任该 peer：否则 Require 会挂起等待人工审批（90s），测试就跑不动了。
	srv.agentApproverGet().Trust("peer-idem")

	req := peerlink.AgentInvokeRequest{
		Tool:     "list_mounts",
		FromId:   "peer-idem",
		FromName: "desktop",
		CallId:   "IDEM-CALL-1",
	}

	out1 := srv.PeerAgentInvokeHandler(req)
	if out1.Err != nil {
		t.Fatalf("首次执行不应失败: %v", out1.Err)
	}
	out2 := srv.PeerAgentInvokeHandler(req)
	if out2.Err != nil {
		t.Fatalf("重复调用应当直接返回首次结果（不得报错）: %v", out2.Err)
	}
	if string(out1.Result) != string(out2.Result) {
		t.Fatalf("重复调用的结果必须与首次一致: %q vs %q", out1.Result, out2.Result)
	}

	if n := countAuditByCallId(srv.agentApproverGet(), "IDEM-CALL-1"); n != 1 {
		t.Fatalf("同 callId 两次调用却产生了 %d 条审计 ⇒ 工具被执行了 %d 次，幂等失效", n, n)
	}
}

// TestPeerAgentInvoke_DifferentCallIdStillRuns 反向锁：
// 幂等不能"误伤"——不同 callId 的调用必须**照常执行**，不能被当成重复而吞掉。
func TestPeerAgentInvoke_DifferentCallIdStillRuns(t *testing.T) {
	dir := t.TempDir()
	srv := newTestServerWithDirs(t, dir, dir)
	srv.agentApproverGet().Trust("peer-idem")

	first := srv.PeerAgentInvokeHandler(peerlink.AgentInvokeRequest{
		Tool: "list_mounts", FromId: "peer-idem", CallId: "CALL-A",
	})
	second := srv.PeerAgentInvokeHandler(peerlink.AgentInvokeRequest{
		Tool: "list_mounts", FromId: "peer-idem", CallId: "CALL-B",
	})
	if first.Err != nil || second.Err != nil {
		t.Fatalf("不同 callId 都应正常执行: %v / %v", first.Err, second.Err)
	}
	if n := countAuditByCallId(srv.agentApproverGet(), "CALL-B"); n != 1 {
		t.Fatalf("不同 callId 必须各自执行 ⇒ CALL-B 应有 1 条审计, got %d", n)
	}
}

// helpers

// countAuditByCallId 统计审计中该 callId 的条目数（= 该调用真正走了 Require/执行的次数）。
func countAuditByCallId(ap *peerlink.Approver, callId string) int {
	n := 0
	for _, e := range ap.Audit() {
		if e.CallId == callId {
			n++
		}
	}
	return n
}

var errStub = stubSentinel("stub failure")

type stubSentinel string

func (e stubSentinel) Error() string { return string(e) }

func keyFor(i int) string { return "key-" + strconv.Itoa(i) }
