package kernel

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"time"
)

// jsonRoundTrip 借助 encoding/json 把 any round-trip 到 dst
// （用于 SubscribeTyped 把 map / 任意类型转换为订阅方声明的结构）
func jsonRoundTrip(in any, dst any) error {
	if in == nil {
		return nil
	}
	raw, err := json.Marshal(in)
	if err != nil {
		return fmt.Errorf("marshal: %w", err)
	}
	return json.Unmarshal(raw, dst)
}

// ─── Bus：in-process pub/sub（无端口、无外部依赖） ───────────────────────
//
// 用途：
//   - service 间事件解耦（"文件已加密" → "通知 agent + 触发自动化测试 + 更新 UI 缓存"）
//   - SSE 事件桥接（agent 推事件 → bus 推 topic → SSE handler 订阅）
//   - WebManager 完成通知（worker 完成 → bus 推 topic → 上层刷新）
//
// 不使用外部 broker（Redis/NATS）— 单一进程内 in-memory delivery。
// 跨进程场景由上层（如 SSE fanout）解决，本内核只做进程内。
type Event struct {
	Topic   string         // 例 "file.encrypted"、"agent.tool.done"
	Payload any            // 任意值（应可 JSON 序列化）
	Ctx     ServiceContext // 发布时的 ctx（用于日志 / 追踪）
	At      time.Time      // 发布时间
}

type Subscriber func(Event) error

// ─── 发布 ────────────────────────────────────────────────

// Publish 同步发布：所有订阅者的 fn 串行执行（保证顺序）。
// 任一订阅者返回 error，Publish 立即返回该 error（不继续调用后续订阅者）。
//
// 用法：
//
//	kernel.Publish(ctx, "file.encrypted", gin.H{"path": path})
//
// 对于不需要顺序的扇出场景，用 PublishAsync。
func Publish(ctx ServiceContext, topic string, payload any) error {
	if topic == "" {
		return errors.New("kernel: empty topic")
	}
	busMu.RLock()
	subs := append([]busSub(nil), subscribers[topic]...)
	busMu.RUnlock()

	ev := Event{Topic: topic, Payload: payload, Ctx: ctx, At: time.Now()}
	for _, sub := range subs {
		// 检查订阅者 ctx（已被取消的不再调用）
		select {
		case <-sub.ctx.Done():
			continue
		default:
		}
		if err := sub.fn(ev); err != nil {
			return fmt.Errorf("kernel: subscriber %s failed: %w", sub.id, err)
		}
	}
	return nil
}

// PublishAsync 异步发布：fire-and-forget。订阅者在独立 goroutine 跑。
// 不会阻塞调用方，error 仅 slog 记录（不返回）。
func PublishAsync(ctx ServiceContext, topic string, payload any) {
	go func() {
		if err := Publish(ctx, topic, payload); err != nil {
			// 内置 logger 兜底（如果项目用了其他 logger，可替换为依赖注入）
			fmt.Printf("[kernel] PublishAsync %s: %v\n", topic, err)
		}
	}()
}

// ─── 订阅 ────────────────────────────────────────────────

// Subscribe 订阅一个 topic。
// 返回 unsub 函数，调用即可取消订阅。
// ctx 用于控制订阅生命周期：ctx cancel 后订阅者不再被调用（但 unsub 仍需调用以释放资源）。
func Subscribe(ctx context.Context, topic string, fn Subscriber) func() {
	if ctx == nil {
		ctx = context.Background()
	}
	if fn == nil {
		panic("kernel: Subscribe with nil fn")
	}
	sub := busSub{
		id:  nextID(),
		fn:  fn,
		ctx: ctx,
	}
	busMu.Lock()
	subscribers[topic] = append(subscribers[topic], sub)
	busMu.Unlock()

	return func() {
		busMu.Lock()
		defer busMu.Unlock()
		list := subscribers[topic]
		for i, s := range list {
			if s.id == sub.id {
				subscribers[topic] = append(list[:i], list[i+1:]...)
				return
			}
		}
	}
}

// SubscribeTyped 类型安全订阅（payload 自动 unmarshal 到指定类型）。
//
//	kernel.SubscribeTyped[*MyEvent](ctx, "file.encrypted", func(e kernel.Event, payload *MyEvent) error {
//	    log.Printf("got %+v", payload)
//	    return nil
//	})
//
// 内部走 json round-trip：
//   - payload 已经是 T 类型 → 直接传给 fn
//   - payload 是 map / 其他类型 → json.Marshal + Unmarshal 到 T
func SubscribeTyped[T any](ctx context.Context, topic string, fn func(Event, T) error) func() {
	return Subscribe(ctx, topic, func(ev Event) error {
		var typed T
		switch p := ev.Payload.(type) {
		case T:
			typed = p
		default:
			// 借助 json round-trip 把 any 转成 T
			if err := remarshalJSON(p, &typed); err != nil {
				return fmt.Errorf("kernel: SubscribeTyped unmarshal: %w", err)
			}
		}
		return fn(ev, typed)
	})
}

func remarshalJSON(in any, dst any) error {
	// 标准库 import 在 bus_test.go 里验证；这里用 inline import 避免环形
	return jsonRoundTrip(in, dst)
}
