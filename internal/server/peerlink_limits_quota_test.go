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

// TestQuotaBytesPerMin_... 额度换算策略（2026-10-04 用户指令）：
// **实测链路带宽 × 90%，且不低于 1Mbps**。固定值对百兆盒子太宽、对万兆服务器太紧，毫无意义。
func TestQuotaBytesPerMin_FloorIsOneMbps(t *testing.T) {
	const oneMbpsBytesPerMin = 125000 * 60 // 1Mbps = 125000 B/s

	// 实测不出来（0）⇒ 走保底档
	if got := quotaBytesPerMinFor(0); got != oneMbpsBytesPerMin {
		t.Fatalf("实测失败时应回落到 1Mbps 保底（%d）, got %d", oneMbpsBytesPerMin, got)
	}
	// 实测极低（0.5Mbps ⇒ 90% 后 0.45Mbps）⇒ 仍不得低于 1Mbps
	if got := quotaBytesPerMinFor(0.5); got != oneMbpsBytesPerMin {
		t.Fatalf("带宽过低时必须保底 1Mbps, got %d", got)
	}
}

func TestQuotaBytesPerMin_IsNinetyPercentOfLink(t *testing.T) {
	// 100Mbps × 90% = 90Mbps = 11.25 MB/s ⇒ 每分钟 675,000,000 字节
	const want = int64(90 * 1e6 / 8 * 60)
	if got := quotaBytesPerMinFor(100); got != want {
		t.Fatalf("100Mbps 带宽应得 %d 字节/分钟（90%%）, got %d", want, got)
	}
	// 万兆网卡（本机 eth0 报的就是 10000）也不能溢出/翻车
	if got := quotaBytesPerMinFor(10000); got <= 0 {
		t.Fatalf("万兆带宽应得到正的额度, got %d", got)
	}
}

func TestQuotaBytesPerMin_Monotonic(t *testing.T) {
	prev := int64(0)
	for _, mbps := range []float64{0, 1, 10, 100, 1000, 10000} {
		got := quotaBytesPerMinFor(mbps)
		if got < prev {
			t.Fatalf("额度必须随带宽单调不减：%v Mbps ⇒ %d < 上一个 %d", mbps, got, prev)
		}
		prev = got
	}
}

// TestMeasureLinkMbps_EnvOverride 容器里 veth 网卡 speed 常虚高（本机报 10Gbps），
// 必须提供环境变量覆盖口，否则照着虚高值算等于不限流。
func TestMeasureLinkMbps_EnvOverride(t *testing.T) {
	t.Setenv(envLinkMbpsOverride, "42")
	if got := measureLinkMbps(); got != 42 {
		t.Fatalf("环境变量覆盖未生效: %v", got)
	}
	t.Setenv(envLinkMbpsOverride, "not-a-number")
	_ = measureLinkMbps() // 非法值应被忽略，退回实测，不得 panic
}
