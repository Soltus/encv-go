# Tasks

## Phase 1: 后端性能采集核心包（pkg/tasksystem/performance/）

- [ ] Task 1: 创建 `pkg/tasksystem/performance/metrics.go` — PerformanceMetrics / PhaseTiming / Grade 类型定义
- [ ] Task 2: 创建 `pkg/tasksystem/performance/collector.go` — 轻量级采集器（time.Now + atomic + sync.Mutex，WrapReader/WrapWriter 透明包装，环形缓冲采样）
- [ ] Task 3: 创建 `pkg/tasksystem/performance/calibration.go` — RunCalibration（AES-CTR 1MB 测试 → CPUScore，跨平台，~50-200ms）
- [ ] Task 4: 创建 `pkg/tasksystem/performance/grading.go` — GetThresholds（按 taskType + CPUScore 动态阈值）+ CalculateGrade（评分公式与 bench-report 一致）
- [ ] Task 5: 创建 `pkg/tasksystem/performance/history.go` — CompareWithHistory（pctChange 计算）
- [ ] Task 6: 单元测试 `collector_test.go` + `calibration_test.go` + `grading_test.go`

## Phase 2: SQLite 存储扩展

- [ ] Task 7: `pkg/tasksystem/store/sqlite/sqlite.go` 加 performance_metrics 表 + calibration 表 schema
- [ ] Task 8: 实现 SaveMetrics / GetMetrics / ListMetricsByPlugin / GetLatestMetrics 方法
- [ ] Task 9: 实现 SaveCalibration / GetCalibration 方法
- [ ] Task 10: 存储测试 `performance_test.go`（CRUD + 历史查询）

## Phase 3: v2 加解密流程 instrumentation

- [ ] Task 11: `internal/v2/plugins/registry.go` EncryptFileWithPlugin 加 collector 参数 + 5 phase 计时点（analyzing/initializing/encrypting/packing/verifying）
- [ ] Task 12: `internal/v2/plugins/registry.go` DecryptContainerWithPlugin 加 collector 参数 + 5 phase 计时点
- [ ] Task 13: `internal/v2/plugins/registry.go` io.Copy 改用 collector.WrapReader/WrapWriter 透明包装

## Phase 4: TaskManager 集成 + WS 推送

- [ ] Task 14: `internal/service/task_manager.go` processEncrypt 集成 collector（创建/启动/Finalize/评级/持久化）
- [ ] Task 15: `internal/service/task_manager.go` processDecrypt 同上
- [ ] Task 16: 启动时调用 performance.RunCalibration + store.SaveCalibration（若 calibration 表为空）
- [ ] Task 17: WS task:progress payload 扩展（加 avgThroughput / currentPhaseDurationMs）
- [ ] Task 18: WS task:completed payload 扩展（加 performanceSummary）

## Phase 5: 后端 API

- [ ] Task 19: `internal/server/performance_api.go` — GET /api/tasks/:id/performance
- [ ] Task 20: GET /api/performance/calibration + POST /api/performance/calibration（dev-only 重跑）
- [ ] Task 21: GET /api/performance/history?plugin=xxx&type=encrypt&limit=10
- [ ] Task 22: `internal/server/server.go` 注册 4 条路由

## Phase 6: bench-report 跨平台重写

- [ ] Task 23: `cmd/bench-report/main.go` 移除 //go:build windows 约束
- [ ] Task 24: 移除 memoryGuard 中的 kernel32.dll GlobalMemoryStatusEx，改用 runtime.MemStats 轻量采样 + GOMEMLIMIT
- [ ] Task 25: 改用 performance.RunCalibration() 替代原有 runCalibration
- [ ] Task 26: 新增 --store flag，把 benchResult 转换为 PerformanceMetrics 入库
- [ ] Task 27: 验证 bench-report 在 Linux/macOS 可编译运行

## Phase 7: 前端 API + 类型

- [ ] Task 28: `api/encv.ts` 加 PerformanceMetrics / PhaseTiming / CalibrationResult / PerformanceSummary 类型
- [ ] Task 29: `api/encv.ts` EncvTask 加 performanceSummary 字段
- [ ] Task 30: `api/encv.ts` 新增 getTaskPerformance / getCalibration / recalibrateCalibration / getPerformanceHistory API

## Phase 8: 前端任务详情性能区块

- [ ] Task 31: 新建 `components/TaskPerformanceSection.vue`（源大小/输出大小/加密比率/平均吞吐/峰值/P50/P99/评级 badge/CPUScore）
- [ ] Task 32: `TaskPerformanceSection.vue` Phase 耗时时间线（进度条可视化，按 durationMs 比例）
- [ ] Task 33: `TaskDetailModal.vue` 集成 TaskPerformanceSection（仅 performanceSummary 存在时显示）
- [ ] Task 34: 点击"查看完整指标"展开 PerformanceMetrics 详情（调 getTaskPerformance API）

## Phase 9: 前端 GroupDetail Performance tab

- [ ] Task 35: 新建 `components/group-detail/PerformanceTab.vue`
- [ ] Task 36: PerformanceTab 显示硬件校准信息（CPUScore + 校准时间）
- [ ] Task 37: PerformanceTab Plugin 聚合表格（pluginName 分组 + 平均吞吐 + 评级分布 + 趋势 pctChange）
- [ ] Task 38: PerformanceTab 历史趋势折线图（吞吐量 vs 时间，用 Chart.js 或 SVG）
- [ ] Task 39: `GroupDetail.vue` 加第 4 个 tab（Pipeline / Tasks / Diagnostics / Performance）

## Phase 10: 前端报告导出扩展

- [ ] Task 40: `buildReportZip.ts` report.json 加 performance 字段（每 case 的 metrics）+ calibration 字段
- [ ] Task 41: `buildReportZip.ts` summary.md 加性能聚合表格
- [ ] Task 42: `buildReportZip.ts` 新增 performance.md 顶层文件（详细性能分析）
- [ ] Task 43: `buildReportZip.ts` cases/*.md 加性能指标区块

## Phase 11: i18n + 验证

- [ ] Task 44: `i18n/tasks.ts` 加性能相关翻译键（中英文各约 30 个）
- [ ] Task 45: go build + go test ./pkg/tasksystem/... 通过
- [ ] Task 46: vue-tsc + vite build 通过
- [ ] Task 47: 端到端验证：加密任务 → TaskDetailModal 显示性能区块 → GroupDetail Performance tab → 导出 zip 含 performance.md

## Task Dependencies
- Task 2 依赖 Task 1
- Task 3 依赖 Task 1
- Task 4 依赖 Task 1
- Task 5 依赖 Task 1
- Task 6 依赖 Task 2-5
- Task 7 依赖 Task 1
- Task 8 依赖 Task 7
- Task 9 依赖 Task 7
- Task 10 依赖 Task 8-9
- Task 11-13 依赖 Task 2
- Task 14-15 依赖 Task 11-13, Task 8
- Task 16 依赖 Task 3, Task 9
- Task 17-18 依赖 Task 14-15
- Task 19-21 依赖 Task 8-9
- Task 22 依赖 Task 19-21
- Task 23-27 依赖 Task 3
- Task 28-30 依赖 Task 19-22
- Task 31-34 依赖 Task 28-30
- Task 35-39 依赖 Task 28-30
- Task 40-43 依赖 Task 28-30
- Task 44 依赖 Task 31-43
- Task 45-47 依赖 Task 1-44
