package service

// task_manager_fts_rebuild.go — FTS 索引重建任务处理器
//
// 2026-07-03 新增（spec fts-rebuild-task）
//
// 职责：
//   - 包装 FTSRebuilder.RebuildWithProgress 调用
//   - 把 progress callback 转成 updateProgress（自动广播 task:progress WS 事件）
//   - 完成时设 Status=completed + Progress=100 + 广播 task:completed
//   - 失败时 failTask + 广播 task:completed with error
//
// 不负责的事：
//   - 实际的扫描 / BulkInsert 逻辑（在 server 层 FTSRebuilderImpl 实现）
//   - servingDir 解析（FTSRebuilder 自己持有 servingDir 引用）
//   - cancelFn 注册（在 RebuildWithProgress 内部用 context.WithCancel）

import (
	"context"
	"fmt"
	"log/slog"
	"time"
)

// processRebuildFTSIndex 处理 FTS 索引重建任务。
//
// 流程：
//   1. 检查 ftsRebuilder 是否注入（未注入 failTask）
//   2. 创建可取消的 context（注册到 task.cancelFn）
//   3. 调用 RebuildWithProgress，传入 progress callback
//   4. progress callback 内调 updateProgress（自动广播 task:progress）
//   5. 完成：锁内设 Status=completed + Progress=100 + 持久化 + 广播 task:completed
//   6. 失败：failTask（自动持久化 + 广播 task:completed with error）
//
// progress callback 约定：
//   - progress: 0-100（FTSRebuilder 内部分阶段推送）
//   - phase: "scanning" / "indexing" / "finalizing" 等
//   - speed: "X files/s" 或 "X KB/s"
//   - eta: "Xs" / "Xm" / "Xh"
func (tm *TaskManager) processRebuildFTSIndex(task *MobileTask) {
	taskID := task.ID

	if tm.ftsRebuilder == nil {
		tm.failTask(taskID, "fts rebuilder not configured")
		return
	}

	// 创建可取消的 context（用户点取消时 cancelFn 触发）
	ctx, cancel := context.WithCancel(context.Background())
	tm.mu.Lock()
	task.cancelFn = cancel
	tm.mu.Unlock()
	defer cancel()

	slog.Info("FTS rebuild task started", "id", taskID)

	// progress callback：把 FTSRebuilder 的进度转成 updateProgress
	// （updateProgress 内部会广播 task:progress WS 事件 + 标记 dirty 持久化）
	progressCb := func(progress int, phase, speed, eta string) {
		// 检查取消状态（cancelFn 触发后不再推送进度，避免覆盖 cancelling 状态）
		if ctx.Err() != nil {
			return
		}
		// 检查任务状态（可能已被 Cancel 设为 cancelling）
		tm.mu.RLock()
		currentStatus := task.Status
		tm.mu.RUnlock()
		if currentStatus == "cancelling" || currentStatus == "cancelled" {
			return
		}
		tm.updateProgress(taskID, progress, phase, speed, eta)
	}

	start := time.Now()
	err := tm.ftsRebuilder.RebuildWithProgress(ctx, progressCb)
	elapsed := time.Since(start)

	// 处理取消场景
	if ctx.Err() != nil || task.Status == "cancelling" {
		tm.mu.Lock()
		task.Status = "cancelled"
		task.Phase = "cancelled"
		task.Speed = ""
		task.Eta = ""
		now := time.Now()
		task.CompletedAt = &now
		taskToPersist := task
		tm.mu.Unlock()

		tm.saveTaskSingle(taskToPersist)
		slog.Info("FTS rebuild task cancelled", "id", taskID, "elapsed", elapsed)
		if tm.broadcaster != nil {
			tm.broadcaster.Broadcast("task:completed", map[string]interface{}{
				"id":     taskID,
				"status": "cancelled",
			})
		}
		return
	}

	if err != nil {
		tm.failTask(taskID, fmt.Sprintf("FTS rebuild failed: %v", err))
		slog.Error("FTS rebuild task failed", "id", taskID, "elapsed", elapsed, "error", err)
		return
	}

	// 成功完成
	tm.mu.Lock()
	var taskToPersist *MobileTask
	task.Status = "completed"
	task.Progress = 100
	task.Phase = "completed"
	task.Speed = ""
	task.Eta = ""
	now := time.Now()
	task.CompletedAt = &now
	taskToPersist = task
	tm.mu.Unlock()

	if taskToPersist != nil {
		tm.saveTaskSingle(taskToPersist)
	}

	slog.Info("FTS rebuild task completed", "id", taskID, "elapsed", elapsed)
	if tm.broadcaster != nil {
		tm.broadcaster.Broadcast("task:completed", map[string]interface{}{
			"id":     taskID,
			"status": "completed",
		})
		// 通知前端 FTS 索引已更新（前端可刷新 stats）
		tm.broadcaster.Broadcast("fts:index-updated", map[string]interface{}{
			"taskId": taskID,
		})
	}
}
