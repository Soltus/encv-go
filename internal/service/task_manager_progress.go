package service

// task_manager_progress.go — 拆分自 task_manager.go

import (
	"log/slog"
	"time"
)

func (tm *TaskManager) updateProgress(id string, progress int, phase, speed, eta string) {
	tm.mu.Lock()
	if task, ok := tm.tasks[id]; ok {
		if task.Phase != phase {
			now := time.Now()
			for i := len(task.Steps) - 1; i >= 0; i-- {
				if task.Steps[i].CompletedAt == nil {
					task.Steps[i].CompletedAt = &now
					break
				}
			}
			task.Steps = append(task.Steps, TaskStep{
				Phase:     phase,
				StartedAt: now,
				Detail:    phaseDetailDescription(phase),
			})
		}
		task.Progress = progress
		task.Phase = phase
		task.Speed = speed
		task.Eta = eta
	}
	tm.mu.Unlock()

	// 标记为脏任务，由进度持久化 goroutine 批量写入
	if tm.store != nil {
		tm.markDirty(id)
	}

	if tm.broadcaster != nil {
		tm.broadcaster.Broadcast("task:progress", map[string]interface{}{
			"id":       id,
			"progress": progress,
			"phase":    phase,
			"speed":    speed,
			"eta":      eta,
		})
	}
}

func (tm *TaskManager) markDirty(taskID string) {
	if tm.store == nil {
		return
	}
	tm.dirtyMu.Lock()
	tm.dirtyTasks[taskID] = struct{}{}
	tm.dirtyMu.Unlock()
}

func (tm *TaskManager) progressPersister() {
	defer tm.wg.Done()

	ticker := time.NewTicker(5 * time.Second)
	defer ticker.Stop()

	for {
		select {
		case <-tm.stopCh:
			tm.flushDirtyTasks()
			return
		case <-ticker.C:
			tm.flushDirtyTasks()
		}
	}
}

func (tm *TaskManager) flushDirtyTasks() {
	tm.dirtyMu.Lock()
	if len(tm.dirtyTasks) == 0 {
		tm.dirtyMu.Unlock()
		return
	}
	ids := make([]string, 0, len(tm.dirtyTasks))
	for id := range tm.dirtyTasks {
		ids = append(ids, id)
	}
	tm.dirtyTasks = make(map[string]struct{})
	tm.dirtyMu.Unlock()

	tm.mu.RLock()
	tasksToPersist := make([]*MobileTask, 0, len(ids))
	for _, id := range ids {
		if task, ok := tm.tasks[id]; ok {
			if !isTerminalStatus(task.Status) {
				tasksToPersist = append(tasksToPersist, task)
			}
		}
	}
	tm.mu.RUnlock()

	if len(tasksToPersist) == 0 {
		return
	}

	for _, task := range tasksToPersist {
		tm.persistTaskWithRetry(task)
	}
}

func (tm *TaskManager) persistTaskWithRetry(task *MobileTask) {
	if tm.store == nil || task == nil {
		return
	}
	data := mobileTaskToData(task)
	var err error
	for attempt := 0; attempt < 3; attempt++ {
		err = tm.store.CreateTask(data)
		if err == nil {
			return
		}
		if err = tm.store.UpdateTask(data); err == nil {
			return
		}
		if attempt < 2 {
			time.Sleep(time.Duration(10*(attempt+1)) * time.Millisecond)
		}
	}
	slog.Warn("Failed to persist task after retries",
		"id", task.ID, "error", err)
}
