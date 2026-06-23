# Checklist

## Phase 1: 后端性能采集核心包
- [ ] pkg/tasksystem/performance/metrics.go — PerformanceMetrics / PhaseTiming / Grade 类型
- [ ] pkg/tasksystem/performance/collector.go — 轻量级采集器（time.Now + atomic + sync.Mutex）
- [ ] pkg/tasksystem/performance/collector.go — WrapReader / WrapWriter 透明包装
- [ ] pkg/tasksystem/performance/collector.go — 环形缓冲采样（容量 64，每 500ms 或 1MB 采样）
- [ ] pkg/tasksystem/performance/collector.go — Finalize 计算 avg/peak/p50/p99
- [ ] pkg/tasksystem/performance/calibration.go — RunCalibration（AES-CTR 1MB 测试，跨平台）
- [ ] pkg/tasksystem/performance/calibration.go — CPUScore 计算（aesThroughput / 3000.0）
- [ ] pkg/tasksystem/performance/calibration.go — CPULabel 分级（fast>=1.5 / medium>=0.5 / slow<0.5）
- [ ] pkg/tasksystem/performance/grading.go — GetThresholds（按 taskType + CPUScore 动态阈值）
- [ ] pkg/tasksystem/performance/grading.go — CalculateGrade（评分公式与 bench-report 一致）
- [ ] pkg/tasksystem/performance/history.go — CompareWithHistory（pctChange 计算）
- [ ] 单元测试 collector_test.go（采集 + 采样 + Finalize）
- [ ] 单元测试 calibration_test.go（校准 + CPUScore）
- [ ] 单元测试 grading_test.go（评级 + 动态阈值）

## Phase 2: SQLite 存储扩展
- [ ] performance_metrics 表 schema（task_id FK + JSON payload + 索引）
- [ ] calibration 表 schema（单行表，id=1 约束）
- [ ] SaveMetrics / GetMetrics 方法
- [ ] ListMetricsByPlugin 方法（按 plugin + taskType 查询历史）
- [ ] GetLatestMetrics 方法（上一次运行，用于历史对比）
- [ ] SaveCalibration / GetCalibration 方法
- [ ] 存储测试 performance_test.go（CRUD + 历史查询）

## Phase 3: v2 加解密流程 instrumentation
- [ ] EncryptFileWithPlugin 加 collector 参数（允许 nil，向后兼容）
- [ ] EncryptFileWithPlugin 5 phase 计时点（analyzing/initializing/encrypting/packing/verifying）
- [ ] DecryptContainerWithPlugin 加 collector 参数
- [ ] DecryptContainerWithPlugin 5 phase 计时点
- [ ] io.Copy 改用 collector.WrapReader/WrapWriter 透明包装

## Phase 4: TaskManager 集成 + WS 推送
- [ ] processEncrypt 创建 collector + 启动 phase + Finalize + 评级 + 持久化
- [ ] processDecrypt 同上
- [ ] 启动时调用 RunCalibration + SaveCalibration（若 calibration 表为空）
- [ ] WS task:progress payload 扩展（加 avgThroughput / currentPhaseDurationMs）
- [ ] WS task:completed payload 扩展（加 performanceSummary）
- [ ] 旧版 WS 客户端兼容性（新字段可选，旧字段保留）

## Phase 5: 后端 API
- [ ] GET /api/tasks/:id/performance 返回 PerformanceMetrics
- [ ] GET /api/performance/calibration 返回 CalibrationResult
- [ ] POST /api/performance/calibration 手动重跑校准（dev-only）
- [ ] GET /api/performance/history?plugin=xxx&type=encrypt&limit=10 返回历史趋势
- [ ] server.go 注册 4 条路由

## Phase 6: bench-report 跨平台重写
- [ ] 移除 //go:build windows 约束
- [ ] 移除 memoryGuard 中 kernel32.dll GlobalMemoryStatusEx
- [ ] 改用 runtime.MemStats 轻量采样 + GOMEMLIMIT
- [ ] 改用 performance.RunCalibration() 替代原有 runCalibration
- [ ] 新增 --store flag（benchResult → PerformanceMetrics 入库）
- [ ] Linux/macOS 编译运行验证

## Phase 7: 前端 API + 类型
- [ ] PerformanceMetrics / PhaseTiming / CalibrationResult / PerformanceSummary 类型
- [ ] EncvTask 加 performanceSummary 字段
- [ ] getTaskPerformance API
- [ ] getCalibration API
- [ ] recalibrateCalibration API
- [ ] getPerformanceHistory API

## Phase 8: 前端任务详情性能区块
- [ ] TaskPerformanceSection.vue 组件（源大小/输出大小/加密比率/吞吐量/评级 badge/CPUScore）
- [ ] Phase 耗时时间线（进度条可视化）
- [ ] TaskDetailModal.vue 集成（仅 performanceSummary 存在时显示）
- [ ] 点击"查看完整指标"展开详情（调 getTaskPerformance API）
- [ ] 评级 badge 颜色（excellent=success / good=primary / warn=warning）

## Phase 9: 前端 GroupDetail Performance tab
- [ ] PerformanceTab.vue 组件
- [ ] 硬件校准信息显示（CPUScore + 校准时间）
- [ ] Plugin 聚合表格（pluginName 分组 + 平均吞吐 + 评级分布 + 趋势 pctChange）
- [ ] 历史趋势折线图（吞吐量 vs 时间）
- [ ] GroupDetail.vue 加第 4 个 tab

## Phase 10: 前端报告导出扩展
- [ ] report.json 加 performance 字段（每 case 的 metrics）
- [ ] report.json 加 calibration 字段
- [ ] summary.md 加性能聚合表格
- [ ] 新增 performance.md 顶层文件
- [ ] cases/*.md 加性能指标区块
- [ ] 向后兼容（旧版解析无 performance 字段不报错）

## Phase 11: i18n + 验证
- [ ] i18n/tasks.ts 加性能翻译键（中英文各约 30 个）
- [ ] go build 通过
- [ ] go test ./pkg/tasksystem/... 通过
- [ ] vue-tsc 通过
- [ ] vite build 通过
- [ ] 端到端验证：加密任务 → TaskDetailModal 性能区块 → GroupDetail Performance tab → 导出 zip

## 验收
- [ ] 采集开销 <1%（对比启用前后加密 100MB 文件耗时差异）
- [ ] 启动时校准开销 <300ms
- [ ] SQLite 写入不阻塞任务完成
- [ ] 旧版 WS 客户端兼容
- [ ] 旧版 buildReportZip 解析兼容
- [ ] bench-report 无 --store flag 时行为与原来一致
- [ ] bench-report 在 Linux/macOS 可编译运行
