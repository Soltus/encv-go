package service

import (
	"os"
	"testing"
)

// TestMain 把任务持久化目录隔离到临时目录。
//
// 背景（2026-09-27 排查）：TaskManager 的持久化路径是
//
//	config.AppDataDir("tasks")/.encv-tasks.json
//
// 这是**全局应用数据目录**，不是 servingDir。此前本包没有任何隔离，
// 于是：
//  1. 每个测试都往同一个全局 JSON 追加任务，条数越跑越多（实测累积到 55 条）；
//  2. 依赖「预置任务文件」的用例（TestRemoveTask_PersistenceAfterReload、
//     TestTaskPersistence_RoundTrip）把 JSON 写进自己的 t.TempDir()，
//     而 TaskManager 去全局目录读 → 永远读不到，稳定失败。
//
// config.AppDataDir 支持 ENCV_TASKS_DIR 覆盖，故在 TestMain 里统一指到临时目录：
// 既让这两个用例的「写文件 → 重载」语义成立，也让整包不再互相污染、不再污染开发机。
func TestMain(m *testing.M) {
	dir, err := os.MkdirTemp("", "encv-tasks-test-*")
	if err != nil {
		panic("failed to create isolated tasks dir: " + err.Error())
	}
	// 注意：不能用 t.Setenv（TestMain 里没有 *testing.T）；
	// 这里手动设置并在结束后清理，效果等价于整包范围的隔离。
	if err := os.Setenv("ENCV_TASKS_DIR", dir); err != nil {
		panic("failed to set ENCV_TASKS_DIR: " + err.Error())
	}

	code := m.Run()

	_ = os.Unsetenv("ENCV_TASKS_DIR")
	_ = os.RemoveAll(dir)
	os.Exit(code)
}
