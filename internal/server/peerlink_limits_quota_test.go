package server

// peerlink_limits_quota_test.go —— 远端读**按流量**限流的契约锁
//
// 2026-10-04：远端读从"60 次/分钟"改为"按字节计费"。
// 动机：远端读是分片的（单片上限 4MB），按次数限制既不防流量（把单片调大就失控），
// 又会误伤正常使用（读一个大文件要几十片，次数一下就用完）。
// 这组用例锁住的核心语义是：**被限的是搬了多少字节，不是调了多少次**。

import (
	"testing"
	"time"
)

func TestRateQuota_LimitsByBytesNotByCalls(t *testing.T) {
	const budget = 1000
	q := newRateQuota(budget, time.Minute)
	now := time.Now()

	// ① 小额调用可以很多次（次数不该是限制维度）
	for i := 0; i < 50; i++ {
		if ok, _ := q.allow("k", now.Add(time.Duration(i)*time.Millisecond)); !ok {
			t.Fatalf("第 %d 次小额(1B)调用就被限 ⇒ 退化成了按次数限制", i+1)
		}
		q.consume("k", now.Add(time.Duration(i)*time.Millisecond), 1)
	}

	// ② 累计到额度后必须拦住（此时才 50+ 次调用，但字节已接近上限）
	q.consume("k", now.Add(time.Millisecond*51), budget-int64(50))
	if ok, _ := q.allow("k", now.Add(time.Millisecond*52)); ok {
		t.Fatal("累计字节达到额度后仍放行 ⇒ 流量限制未生效")
	}
}

func TestRateQuota_OneBigCallCanExhaustBudget(t *testing.T) {
	const budget = 4096
	q := newRateQuota(budget, time.Minute)
	now := time.Now()

	// 一次"大分片"就能把额度吃光 —— 这正是旧实现（按次数）漏掉的场景
	if ok, _ := q.allow("k", now); !ok {
		t.Fatal("首次调用应放行")
	}
	q.consume("k", now, budget)
	if ok, wait := q.allow("k", now.Add(time.Second)); ok {
		t.Fatal("一次大分片吃光额度后，后续必须被拦（旧按次数实现会放行）")
	} else if wait <= 0 {
		t.Fatalf("超限时应给出正的等待时长, got %v", wait)
	}
}

func TestRateQuota_RecoversAfterWindow(t *testing.T) {
	q := newRateQuota(1024, 100*time.Millisecond)
	now := time.Now()

	q.consume("k", now, 1024)
	if ok, _ := q.allow("k", now); ok {
		t.Fatal("额度用尽时应拒绝")
	}
	// 窗口滑过之后应恢复可用（不能永久封死）
	if ok, _ := q.allow("k", now.Add(150*time.Millisecond)); !ok {
		t.Fatal("窗口过期后额度应恢复")
	}
}

func TestRateQuota_ZeroBytesIsFree(t *testing.T) {
	q := newRateQuota(10, time.Minute)
	now := time.Now()
	for i := 0; i < 100; i++ {
		q.consume("k", now.Add(time.Duration(i)*time.Millisecond), 0)
	}
	if ok, _ := q.allow("k", now); !ok {
		t.Fatal("0 字节的调用不应消耗额度（空响应/探测类请求不该被计费）")
	}
}

// TestFileQuota_BudgetMatchesPlaybackUse 额度取值必须能支撑正当用途：
// 中等码率（~1MB/s）连续播一分钟 ≈ 60MB ⇒ 额度不能低于它，否则"在线播放"会被卡死。
func TestFileQuota_BudgetMatchesPlaybackUse(t *testing.T) {
	const oneMinuteOf1MBps = 60 << 20
	if rateLimitFileBytes < oneMinuteOf1MBps {
		t.Fatalf("远端读额度 %d 字节/分钟 支撑不了一分钟的 ~1MB/s 播放（需 ≥ %d）",
			rateLimitFileBytes, oneMinuteOf1MBps)
	}
}
