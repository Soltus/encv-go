# Tasks

- [x] Task 1: 修复 bin2c 符号命名参数
  - [x] SubTask 1.1: 修改 bin2c 调用，使用 `echo "${base}" | tr '.' '_'` 命名公式，使 `graph.css` → `graph_css` → `ff_graph_css_data`
  - [x] SubTask 1.2: HTML 文件同样使用该公式，使 `graph.html` → `graph_html` → `ff_graph_html_data`
  - [x] SubTask 1.3: CSS 和 HTML 生成不同符号名，无冲突

- [x] Task 2: 确认 CONFIG_RESOURCE_COMPRESSION 配置
  - [x] SubTask 2.1: FFmpeg 8.0 默认启用（当 zlib+gzip 可用时）
  - [x] SubTask 2.2: 添加 `--disable-resource-compression` 到 configure
  - [x] SubTask 2.3: 添加 Phase 2 后的验证步骤

- [x] Task 3: 添加构建后符号验证
  - [x] SubTask 3.1: 使用 `nm -D` 检查动态符号表
  - [x] SubTask 3.2: 缺失则报错退出
  - [x] SubTask 3.3: 检查 `ff_graph_css_data`、`ff_graph_html_data`、`ff_resman_get_string`

- [x] Task 4: 评估 --allow-multiple-definition 必要性
  - [x] 保留并添加注释

- [x] Task 5: 修复 --gc-sections 垃圾回收清除资源符号
  - [x] SubTask 5.1: 将资源符号加入 version script 的 `global` 段，确保 dlopen(RTLD_NOW) 可解析
  - [x] SubTask 5.2: 添加 `-Wl,-u` 标志将资源符号标记为 GC 根节点，防止 --gc-sections 清除
  - [x] SubTask 5.3: 更新符号验证使用 `nm -D`（因为资源符号现在是导出符号）

- [ ] Task 6: CI 完整构建验证
  - [ ] SubTask 6.1: 清除 `.ffmpeg-build/` 缓存目录
  - [ ] SubTask 6.2: 执行完整构建，确认无编译/链接错误
  - [ ] SubTask 6.3: 验证 `nm -D` 输出包含 `ff_graph_css_data` 和 `ff_graph_html_data`

# Task Dependencies

- Task 5 依赖 Task 1（符号名修复后才能正确引用）
- Task 6 依赖所有前置任务

# 备注

- Task 6 需要在 CI 环境中执行
- 构建缓存清理命令：`rm -rf app/encv-mobile/scripts/.ffmpeg-build/`
- 构建后验证命令：`nm -D <build_dir>/ftools-build/libffmpeg.so | grep ff_graph_css_data`
