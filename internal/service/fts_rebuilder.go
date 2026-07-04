package service

// fts_rebuilder.go — FTS 索引重建器抽象层
//
// 2026-07-03 新增：FTS 索引重建任务化（spec fts-rebuild-task）
//
// 设计原则：
//   - service 层定义 FTSRebuilder interface，避免反向依赖 server 包
//   - 与 FileTaskHandler / RollbackManager / MountResolver 注入模式一致
//   - server 层在 NewServer 后调 taskManager.SetFTSRebuilder(&impl) 注入实现
//   - processTask 的 case "rebuild_fts_index" 调用 tm.ftsRebuilder.RebuildWithProgress
//
// 进度回调约定（progress 0-100）：
//   - 0-10:  初始化（清空旧索引、设置 building 标志）
//   - 10-50: 扫描文件系统（scanDirForIndex）
//   - 50-95: 分批 BulkInsert（每批 1000 条）
//   - 100:   完成（MarkBuilt）
//
// eta / speed 字段约定：
//   - speed: 用 "X files/s" 或 "X KB/s" 表示扫描/插入速率
//   - eta:   用 "Xs" / "Xm" / "Xh" 表示剩余时间

import "context"

// FTSRebuilder FTS 索引重建器接口。
//
// 实现方：server 层包装 GetFullTextIndex() + scanDirForIndex + BulkInsert。
// 调用方：service 层 processRebuildFTSIndex（通过 SetFTSRebuilder 注入）。
//
// 注意：
//   - RebuildWithProgress 必须支持 ctx 取消（cancelFn 触发时立即返回）
//   - progressCb 由调用方提供，实现方在每个阶段调用以推送进度
//   - progressCb 在持锁状态下不会被调用（实现方需在锁外调用）
type FTSRebuilder interface {
	// RebuildWithProgress 重建 FTS 索引并通过 progressCb 推送进度。
	//
	// 参数：
	//   - ctx: 取消控制（cancelFn 触发时立即返回 context.Canceled）
	//   - progressCb: 进度回调（progress 0-100, phase, speed, eta）
	//
	// 返回：
	//   - error: 重建失败原因（nil = 成功）
	//
	// 实现要点：
	//   1. 清空旧索引（idx.Clear）
	//   2. 扫描 servingDir（scanDirForIndex，分批 yield 调用 progressCb）
	//   3. 分批 BulkInsert（每批 1000 条，调 progressCb）
	//   4. MarkBuilt 标记完成
	//   5. 全程检查 ctx.Err() 支持取消
	RebuildWithProgress(
		ctx context.Context,
		progressCb func(progress int, phase, speed, eta string),
	) error
}
