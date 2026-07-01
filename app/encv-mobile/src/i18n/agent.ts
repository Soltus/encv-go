export default {
  'zh-CN': {
    'agent.title': 'AI 助手',
    'agent.fabLabel': '打开 AI 助手',
    'agent.model': '模型',
    'agent.temperature': '温度',
    'agent.placeholder': '输入消息…',
    'agent.thinking': '正在思考',
    'agent.thinkingMeta': '正在推理…',
    'agent.thought': '已思考',
    'agent.reasoning': '推理',
    'agent.webSearch': '搜索',
    'agent.queries': '个查询',
    'agent.query': '个查询',
    'agent.running': '正在运行',
    'agent.completed': '已完成',
    'agent.failed': '失败',
    'agent.cancelled': '已取消',
    'agent.editing': '正在编辑',
    'agent.collapse': '收起',
    'agent.expand': '展开',
    'agent.copy': '复制',
    'agent.copied': '已复制',
    'agent.copyFailed': '复制失败',
    'agent.send': '发送',
    'agent.stop': '停止',
    'agent.resume': '恢复',
    'agent.newSession': '新会话',
    'agent.confirmReset': '确定清空当前会话并重新开始？',
    'agent.confirmNewSession': '开启新会话？当前会话会保留在历史中。',
    'agent.history': '会话历史',
    'agent.noHistory': '暂无历史会话',
    'agent.deleteSession': '删除会话',
    'agent.confirmDeleteSession': '确定永久删除该会话？此操作不可恢复。',
    'agent.messages': '条消息',
    'agent.rounds': '轮',
    'agent.justNow': '刚刚',
    'agent.minutesAgo': '分钟前',
    'agent.hoursAgo': '小时前',
    'agent.daysAgo': '天前',
    'agent.errorTitle': '请求失败',
    'agent.retry': '重试',
    'agent.loadingModels': '加载模型列表',
    'agent.modelsError': '模型加载失败',
    'agent.noApiKeyHint': '未配置 API Key，请先填写上方 API Key 后保存',
    'agent.noApiKeyTitle': '未配置 API Key',
    'agent.noApiKeyHint2': '当前设备无法解密已存储的 key，请去 AI 设置重新输入。',
    'agent.goToApiKeySettings': '去设置',
    'agent.modelFallbackPlaceholder': '手动输入模型名称',
    'agent.noModelsAvailable': '无可用模型，可手动输入',
    'agent.apiKeyPlaceholder': 'sk-...',
    'agent.inputHint': 'Shift/⌘/Ctrl + Enter 发送，Enter 换行',
    'agent.emptyHint': '向 AI 助手提问、生成文件、或调用工具',

    // ── Task 12: 附件（Composer `+` 按钮） ────────────────
    'agent.attach': '添加附件',
    'agent.removeAttachment': '移除附件',
    'agent.attachmentCount': '已附加 {n} 个文件',
    'agent.imageAttachment': '图片附件',
    'agent.fileAttachment': '文件附件',

    // ── API Key 加密状态（状态反馈 UI） ─────────────────
    'agent.apiKeyStatusEmpty': '未配置',
    'agent.apiKeyStatusPlaintext': '明文',
    'agent.apiKeyStatusEncrypted': '已加密',
    'agent.apiKeyStatusDecrypting': '解密中…',
    'agent.apiKeyStatusEncrypting': '加密中…',
    'agent.apiKeyStatusDecryptFailed': '解密失败',
    'agent.apiKeyStatusDecryptFailedEmpty': '存储的 API Key 无法解密（所有格式尝试均失败）。加密密钥可能已轮换，或存储值已损坏。系统已自动清除旧值，请重新输入后按回车保存。',
    'agent.apiKeyPlaceholderBroken': '已存储加密值但无法解密，请重新输入',
    'agent.apiKeyPlaceholderKeep': '已配置（输入新值将覆盖）',
    'agent.apiKeyMaskHint': '此处原本有加密的 API Key，但当前无法解密显示。请重新输入并保存以覆盖损坏的密文。',
    'agent.apiKeyAutoReset': '旧 API Key 无法解密，已自动清除。请重新输入后按回车键保存。',
    'agent.apiKeyEmpty': '请输入 API Key',
    'agent.apiKeyInvalid': 'API Key 格式看起来不对，应以 sk- 开头',
    'agent.modelErrorNoApiKey': '未配置 API Key：模型列表需要有效的 OpenAI API Key 才能拉取。请在上方填写 API Key 后保存。',
    'agent.modelErrorDecryptFailed': 'API Key 已存储但无法解密：加密密钥可能已轮换或存储值已损坏。请在上方重新输入 API Key 后保存以覆盖。',
    'agent.modelErrorFixApiKey': '↑ 跳转到 API Key 设置',
    'agent.apiKeyStatusEncryptFailed': '加密失败',
    'agent.apiKeyStatusTestFailed': '连通性测试失败',
    'agent.apiKeyStatusRoundtripOk': '加解密往返一致',
    'agent.apiKeyStatusRoundtripMismatch': '往返解密结果不一致',
    'agent.apiKeyActionRoundtrip': '测试加解密',
    'agent.apiKeyBackendLabel': 'Agent API',
    'agent.apiKeyBackendDev': 'dev 网关',
    'agent.apiKeyBackendNative': '本地后端',
    'agent.apiKeyBackendUser': '用户配置',
    'agent.apiKeyBackendFallback': '兜底配置',
    'agent.apiKeyViewLogs': '查看日志',

    // ── AI 设置页错误态（后端离线 / 配置加载失败） ─────
    // 之前 v-if 三态全 false 时页面一片空白，用户完全看不到发生了什么。
    // 错误态必须给"是什么错 + 怎么修"两条信息，而不是只丢一个 spinner 卡死。
    'agent.backendOffline': '后端服务未连接',
    'agent.backendOfflineHint': '请确认 encv-go 服务已启动，或检查网络连接。',
    'agent.configLoadFailed': '加载 AI 配置失败',

    // ── API Key 加密失败时中止保存的提示 ─────
    // 关键：必须在 /api/encrypt-key 失败时立即中止 saveConfig，
    // 否则明文 API Key 会被写入磁盘，破坏加密存储设计。
    'agent.apiKeyEncryptFailedSaveAborted': 'API Key 加密失败，已中止保存（避免明文写入磁盘）',

    'modals.approve': '批准',
    'modals.approveForSession': '本轮批准',
    'modals.decline': '拒绝',
    'modals.cancel': '拒绝并停止',

    'agent.ops.commands': '已运行 {n} 条命令，{ms}ms',
    'agent.ops.files': '已编辑 {n} 个文件',
    'agent.ops.mixed': '已执行 {n} 个操作（{cmd} 命令 + {file} 文件变更）',
    'agent.ops.toolOutputs': '已执行 {n} 个工具',
    'agent.ops.commandsSummary': '{n} 条命令',
    'agent.ops.filesSummary': '{n} 个文件',
    'agent.ops.expandAll': '展开全部',
    'agent.ops.collapseAll': '收起全部',
    'agent.ops.showMore': '显示更多 ({n})',
    'agent.tool.command': '运行命令',
    'agent.tool.fileChange': '编辑文件',
    'agent.tool.readOnly': '读取信息',
    'agent.tool.webSearch': '联网搜索',
    'agent.tool.unknown': '调用工具',
    // ── Task 6: 工具状态徽章 + 错误反馈 ─────────────────────
    // 用于 ToolDetailContent.vue 的 4 状态视觉：
    //   running → 蓝色 spinner 旁的文字
    //   success → 绿色对勾旁的文字
    //   error   → 红色 ⚠️ badge / 错误详情
    //   timeout → 30s 无响应后切换的 errorCode
    //   duration → 卡片底部"耗时 {s} 秒"
    'agent.tool.errorBadge': '工具错误',
    'agent.tool.running': '执行中...',
    'agent.tool.success': '执行成功',
    'agent.tool.timeout': '执行超时',
    'agent.tool.copyError': '复制错误',
    'agent.tool.duration': '耗时 {s} 秒',
    'agent.tool.errorDetails': '错误详情',
    // ── Task 7: 缩放控件按钮 tooltip ─────────────────────
    // AgentChat 右上角浮动按钮组 "A- / A / A+"
    'agent.zoom.in': '放大',
    'agent.zoom.out': '缩小',
    'agent.zoom.reset': '重置',

    // ── Plan / Todo 块 ─────────────────────────────────
    'agent.plan': '计划',
    'agent.planEmpty': '（暂无计划）',
    'agent.planStatusPending': '待办',
    'agent.planStatusInProgress': '进行中',
    'agent.planStatusCompleted': '已完成',
    'agent.streaming': '加载中',

    // ── 活跃态细分文案（active 集合下区分显示用） ─────
    'agent.statusRunning': '正在运行',
    'agent.statusEditing': '正在编辑',
    'agent.statusThinking': '正在思考',

    // ── Task 7: 上下文自动压缩分隔线 ─────────────────
    // 后端在 messages token 数越过 80% 窗口时调用 LLM summary 压缩老消息，
    // 推送 EventCompaction 事件，前端渲染为不可展开的水平分隔线。
    'agent.contextCompaction': '上下文已自动压缩',

    // ── Task 10: Slash 命令菜单（"/" 触发） ─────────────────
    // 触发条件：textarea 内容以 "/" 开头时弹出，分组"功能" + "技能"。
    // 功能项固定 3 条（attach / plan-mode / permission-mode），
    // 技能项从后端 /api/skills 动态拉取。
    'agent.slashMenuTitle': 'Slash 命令',
    'agent.slashMenuFeatures': '功能',
    'agent.slashMenuSkills': '技能',
    'agent.slashMenuNoMatches': '无匹配项',
    'agent.slashMenuHint': '↑↓ 选择 · Enter 应用 · Esc 关闭',

    // ── Task 22: Agent Task Message（subagent 子任务列表） ─────────
    // 后端 SubagentDispatch 事件触发：AI 把复杂任务拆给多个 subagent
    // 并行 / 串行处理，前端把子任务列表渲染为可折叠块。折叠阈值与
    // codex-web MessageBlocks.tsx:68-69 对齐（7 行 / 520 字符）。
    'agent.agentTask': '子任务',
    'agent.subTaskProgress': '{done}/{total}',
    'agent.agentTaskEmpty': '（暂无子任务）',

    // ── Task 26: LAN Access（局域网访问地址面板） ─────────────────
    // 后端 /api/network/lan-access 枚举当前可被同 WiFi 设备访问的
    // URL，前端在 AgentChat 顶部折叠面板展示。面板用途：用户把
    // http://192.168.x.x:5245/ 输入手机/平板浏览器也能用 AI 助手。
    'agent.lanAccess': '网络访问地址',
    'agent.lanAccessHelp': '同 WiFi 下的设备可用此地址访问',
    'agent.lanAccessEmpty': '未发现可用的网络接口',
    'agent.lanAccessRefresh': '刷新',
    'agent.lanAccessCopy': '复制',
    'agent.lanAccessCopied': '已复制 {url}',
    'agent.lanAccessCopyFailed': '复制失败',
    'agent.lanAccessInterface': '接口：{name}',
    'agent.lanAccessUse': '使用',
    'agent.lanAccessUseTitle': '使用此地址作为后端 baseUrl',
    'agent.lanAccessUseSuccess': '已切换到 {url}',
    'agent.lanAccessUseFailed': '切换失败',

    // ── Task 25: Sync Doctor（脱敏诊断按钮） ─────────────
    // 后端 /api/sync/doctor 返回的 DoctorReport 报告由用户在
    // Settings 面板中点击「运行 sync 诊断」拉取；展示原文 JSON
    // 给用户用于 bug 报告（报告中所有 API key/token/password
    // 已被后端 Redact，无需在前端再次脱敏）。
    'agent.syncDoctor': '运行 sync 诊断',
    'agent.syncDoctorRunning': '正在生成诊断报告…',
    'agent.syncDoctorResult': '诊断结果',
    'agent.syncDoctorCopy': '复制 JSON',
    'agent.syncDoctorCopied': '诊断 JSON 已复制',
    'agent.syncDoctorCopyFailed': '复制失败',
    'agent.syncDoctorFailed': '诊断失败：{msg}',
    'agent.syncDoctorEmpty': '未发现问题',

    // ── Agent Mock Mode（剧本回放，不计费、不调真实 LLM） ─────────
    'agent.mockBadge': '模拟',
    'agent.mockBadgeTooltip': '当前为 mock 模式（不计费），场景：{scenario}',
    'agent.mockMode': '模拟模式',
    'agent.mockModeOff': '关闭（真实 API）',
    'agent.mockModeBuiltin': '内置剧本',
    'agent.mockModeCustom': '自定义剧本',
    'agent.mockModeSet': '已切换到：{mode}',
    'agent.mockModeSetFailed': '切换 mock 模式失败',
    // ── Mock 模式预设输入控件（覆盖在输入框上方的 chip 列表） ─────
    // 由后端 mock_presets 事件驱动，每条预设点击后会调 useAgent().pickMockPreset
    // 发送 userText。后端在剧本任一阶段都可以推新事件实现 mid-scenario 更新。
    // "覆盖式 UI" 语义：chip 在 mock 模式开启期间永远显示，流结束不清空。
    'agent.mockPresetBarAria': 'Mock 模式预设输入',
    'agent.mockPresetBarDefaultScenario': '剧本',
    'agent.mockPresetBarHint': '点击直接发送',
    'agent.mockPresetBarPickerScenario': '剧本库',
    // ── v2 多轮/分支剧本（参考 .trae/specs/agent-tools-scenarios-v2/spec.md）───
    // branchChoicePrompt：MockBranchChoiceBar 头部 prompt 行文案
    'agent.branchChoicePrompt': '请选择操作：',
    // roundProgress：MockBranchChoiceBar 头部 round 胶囊，"第 N/M 轮"
    // round 是 1-based 显示（后端传 0-based，内部 +1）
    'agent.roundProgress': '第 {round}/{total} 轮',
    // roundPausedHint：MockBranchChoiceBar 底部小字提示
    'agent.roundPausedHint': '点击 chip 继续或键入文本',
    // toolDenied：工具被白名单/黑名单拒绝时的提示
    'agent.toolDenied': '工具被拒绝',
    // toolRequiresConfirm：工具需要用户确认（v2 8 个剧本里没有，但保留供未来扩展）
    'agent.toolRequiresConfirm': '工具需要确认',
    // batchRenamePreview：batch_rename_wizard 剧本预览阶段提示
    'agent.batchRenamePreview': '改名预览（{count} 个文件）',
    // batchRenameConfirm：batch_rename_wizard 剧本确认阶段按钮文案
    'agent.batchRenameConfirm': '确认改名',
    // editMetadataTitle：edit_metadata_wizard 剧本步骤标题
    'agent.editMetadataTitle': '修改元数据',
    // commandTimeout：command_run 工具执行超时提示
    'agent.commandTimeout': '命令执行超时',
    // commandDenied：command_run 工具命令不在白名单时提示
    'agent.commandDenied': '命令不在白名单',
    // ── v2 工具快捷动作 chip 行 ───────────────────────────
    // 让用户一键触发 v2 工具演示（pre-fill 输入框）
    'agent.v2Chip.search': '🔍 搜索',
    'agent.v2Chip.searchTitle': 'search_files：递归 + glob + AND/OR/NOT',
    'agent.v2Chip.searchPrompt': '帮我递归搜索所有大于 100MB 的 mp4 文件，按修改时间倒序排前 10 个',
    'agent.v2Chip.read': '📖 读文件',
    'agent.v2Chip.readTitle': 'read_file_v2：分页 + 二进制检测',
    'agent.v2Chip.readPrompt': '用 read_file_v2 读取 clip.mp4 的前 200 行',
    'agent.v2Chip.metadata': 'ℹ️ 元数据',
    'agent.v2Chip.metadataTitle': 'get_metadata：基础字段 + ffprobe 媒体探测',
    'agent.v2Chip.metadataPrompt': '用 get_metadata 探测 vacation_2024.mp4 的分辨率、时长、编码',
    'agent.v2Chip.editMetadata': '🏷️ 改元数据',
    'agent.v2Chip.editMetadataTitle': 'edit_metadata：4 轮向导式写入',
    'agent.v2Chip.editMetadataPrompt': '帮我把 song.mp3 的 ID3 title 改成 "Vacation 2024"',
    'agent.v2Chip.batchRename': '🔄 批量改名',
    'agent.v2Chip.batchRenameTitle': 'batch_rename：dry_run 预览 → 确认 → 执行',
    'agent.v2Chip.batchRenamePrompt': '把 /photos 下所有 .JPG 后缀改成 .jpg（先用 dry_run 预览）',
    'agent.v2Chip.command': '⚙️ 跑命令',
    'agent.v2Chip.commandTitle': 'command_run：受限 shell（白名单 + 超时 + 输出截断）',
    'agent.v2Chip.commandPrompt': '用 ffprobe 查看 vacation_2024.mp4 的完整元数据 JSON',
    // ── v2 剧本演示入口（弹窗列表） ────────────────────────
    'agent.v2Scenarios.btn': 'v2 剧本',
    'agent.v2Scenarios.btnTitle': '演示 8 个 v2 mock 剧本（branch + multi-round）',
    'agent.v2Scenarios.title': 'v2 剧本演示',
    'agent.v2Scenarios.hint': '点击剧本后会自动切到「内置剧本」mock 模式，并发送 trigger 关键词。所有剧本都通过真实 ToolRegistry 派发，会执行 search_files / command_run / batch_rename 等真实工具调用。',
    'agent.v2Scenarios.busy': '当前正在请求中，请稍候',
    'agent.v2Scenarios.mock': 'MOCK',
    'agent.v2Scenarios.triggerKw': '关键词',
    'agent.v2Scenarios.groupSearch': '搜索类',
    'agent.v2Scenarios.groupRead': '读 / 元数据 / shell',
    'agent.v2Scenarios.groupWrite': '写类',
    'agent.v2Scenarios.groupBranch': '分支类',
    'agent.v2Scenarios.s.recursiveMp4': '递归 + glob *.mp4 + size > 100MB',
    'agent.v2Scenarios.s.logicalQuery': 'AND(size_gt + mtime_after + ext_eq) 复合查询',
    'agent.v2Scenarios.s.contentRegex': 'content_regex "ERROR.*timeout" 全文搜索',
    'agent.v2Scenarios.s.readFileV2': 'read_file_v2 分页读取（演示 start_line/end_line）',
    'agent.v2Scenarios.s.getMetadata': 'get_metadata 探测视频/音频元数据（需 ffprobe）',
    'agent.v2Scenarios.s.commandRun': 'command_run ffprobe 受限 shell（白名单+超时）',
    'agent.v2Scenarios.s.editMetadata': '4 轮多轮：选文件→选字段→输入值→确认',
    'agent.v2Scenarios.s.batchRename': 'dry_run 预览 → 确认 → 真实执行',
    'agent.v2Scenarios.s.branchEncrypt': '3 选 1 分支：加密 / 解密 / 取消',
    'agent.v2Scenarios.s.branchVideo': '视频 / 音频 / 其他 多分支 + 跨分支多轮',
    // ── v2 工具调用 badge ──────────────────────────────────
    'agent.v2Badge': 'v2',
    'agent.v2BadgeTitle': 'v2 工具（递归搜索 / 元数据 / 受限 shell 等）',

    // ── Task 4: 工具卡片状态/耗时文案 ────────────────────────────
    // 5 个 ToolStatus → i18n 语义化文案（覆盖 raw 英文 tag）
    'agent.toolStatusPending': '等待中',
    'agent.toolStatusRunning': '执行中',
    'agent.toolStatusSuccess': '成功',
    'agent.toolStatusFailed': '失败',
    'agent.toolStatusCancelled': '已取消',
    // 耗时：ms 占位符会被 formatDuration(毫秒) 替换为 "1.2s" / "850ms" 友好格式
    'agent.toolDuration': '耗时 {ms}',
    'agent.toolDurationLong': '耗时较长',

    // FileReferenceChip 操作
    'agent.fileRefCopyPath': '复制路径',
    'agent.fileRefCopyRelative': '复制相对路径',
    'agent.fileRefOpenInFiles': '在文件中打开',

    // 滚动到底部按钮
    'agent.scrollToBottom': '滚动到底部',

    // 关闭按钮
    'agent.close': '关闭',

    // Tool cards 标题
    'agent.toolCards.fileContentTitle': '文件内容',
    'agent.toolCards.fileListTitle': '文件列表',
    'agent.toolCards.fileStatTitle': '文件信息',
    'agent.toolCards.mountsTitle': '挂载点',
    'agent.toolCards.parseFailed': '数据解析失败',
  },
  en: {
    'agent.title': 'AI Assistant',
    'agent.fabLabel': 'Open AI assistant',
    'agent.model': 'Model',
    'agent.temperature': 'Temp',
    'agent.placeholder': 'Type a message…',
    'agent.thinking': 'Thinking',
    'agent.thinkingMeta': 'Thinking…',
    'agent.thought': 'Thought',
    'agent.reasoning': 'Reasoning',
    'agent.webSearch': 'Search',
    'agent.queries': 'queries',
    'agent.query': 'query',
    'agent.running': 'Running',
    'agent.completed': 'Completed',
    'agent.failed': 'Failed',
    'agent.cancelled': 'Cancelled',
    'agent.editing': 'Editing',
    'agent.collapse': 'Collapse',
    'agent.expand': 'Expand',
    'agent.copy': 'Copy',
    'agent.copied': 'Copied',
    'agent.copyFailed': 'Copy failed',
    'agent.send': 'Send',
    'agent.stop': 'Stop',
    'agent.resume': 'Resume',
    'agent.newSession': 'New session',
    'agent.confirmReset': 'Clear current session and start over?',
    'agent.confirmNewSession': 'Start a new session? The current one will be saved to history.',
    'agent.history': 'History',
    'agent.noHistory': 'No history yet',
    'agent.deleteSession': 'Delete session',
    'agent.confirmDeleteSession': 'Permanently delete this session? This cannot be undone.',
    'agent.messages': 'messages',
    'agent.rounds': 'rounds',
    'agent.justNow': 'just now',
    'agent.minutesAgo': 'm ago',
    'agent.hoursAgo': 'h ago',
    'agent.daysAgo': 'd ago',
    'agent.errorTitle': 'Request failed',
    'agent.retry': 'Retry',
    'agent.loadingModels': 'Loading models',
    'agent.modelsError': 'Failed to load',
    'agent.noApiKeyHint': 'API Key not configured. Please enter it above and save.',
    'agent.modelFallbackPlaceholder': 'Enter model name manually',
    'agent.noModelsAvailable': 'No models available, type manually',
    'agent.apiKeyPlaceholder': 'sk-...',
    'agent.inputHint': 'Shift/⌘/Ctrl + Enter to send, Enter for newline',
    'agent.emptyHint': 'Ask the assistant, generate files, or invoke tools',

    // ── Task 12: attachments (Composer `+` button) ─────────────
    'agent.attach': 'Attach',
    'agent.removeAttachment': 'Remove attachment',
    'agent.attachmentCount': '{n} attachment(s) attached',
    'agent.imageAttachment': 'Image attachment',
    'agent.fileAttachment': 'File attachment',

    // ── API Key encryption status (status feedback UI) ───────────
    'agent.apiKeyStatusEmpty': 'Not set',
    'agent.apiKeyStatusPlaintext': 'Plaintext',
    'agent.apiKeyStatusEncrypted': 'Encrypted',
    'agent.apiKeyStatusDecrypting': 'Decrypting…',
    'agent.apiKeyStatusEncrypting': 'Encrypting…',
    'agent.apiKeyStatusDecryptFailed': 'Decrypt failed',
    'agent.apiKeyStatusDecryptFailedEmpty': 'Stored API key cannot be decrypted (all formats exhausted). The encryption key may have rotated, or the stored value is corrupted. The old value has been auto-cleared — please re-enter your key and press Enter to save.',
    'agent.apiKeyPlaceholderBroken': 'Stored encrypted value cannot be decrypted. Re-enter to overwrite.',
    'agent.apiKeyPlaceholderKeep': 'Already configured (typing a new value will overwrite).',
    'agent.apiKeyMaskHint': 'A previously-encrypted API Key is stored here but cannot be decrypted. Re-enter and save to overwrite the corrupted value.',
    'agent.apiKeyAutoReset': 'Old API key was unreadable and has been auto-cleared. Please re-enter your key below and press Enter to save.',
    'agent.apiKeyEmpty': 'Please enter an API Key',
    'agent.apiKeyInvalid': 'API Key does not look valid (should start with sk-)',
    'agent.modelErrorNoApiKey': 'API Key not configured: the model list requires a valid OpenAI API Key. Fill in the API Key above and save.',
    'agent.modelErrorDecryptFailed': 'Stored API Key cannot be decrypted: the encryption key may have rotated or the stored value is corrupted. Re-enter the API Key above and save to overwrite.',
    'agent.modelErrorFixApiKey': '↑ Jump to API Key setting',
    'agent.apiKeyStatusEncryptFailed': 'Encrypt failed',
    'agent.apiKeyStatusTestFailed': 'Connectivity test failed',
    'agent.apiKeyStatusRoundtripOk': 'Round-trip OK',
    'agent.apiKeyStatusRoundtripMismatch': 'Round-trip mismatch',
    'agent.apiKeyActionRoundtrip': 'Test encrypt/decrypt',
    'agent.apiKeyBackendLabel': 'Agent API',
    'agent.apiKeyBackendDev': 'dev gateway',
    'agent.apiKeyBackendNative': 'local backend',
    'agent.apiKeyBackendUser': 'user config',
    'agent.apiKeyBackendFallback': 'fallback config',
    'agent.apiKeyViewLogs': 'View logs',

    // ── AI settings error state (backend offline / config load failed) ─────
    // Previously when all 3 v-if conditions were false, the page was completely
    // blank. The error state must say "what went wrong + how to fix" — not
    // just an eternal spinner.
    'agent.backendOffline': 'Backend service not connected',
    'agent.backendOfflineHint': 'Please confirm encv-go is running, or check the network connection.',
    'agent.configLoadFailed': 'Failed to load AI configuration',

    // API Key encryption failure → abort save (avoid writing plaintext to disk)
    'agent.apiKeyEncryptFailedSaveAborted': 'API Key encryption failed, save aborted (to avoid writing plaintext to disk)',

    'modals.approve': 'Approve',
    'modals.approveForSession': 'Approve for session',
    'modals.decline': 'Decline',
    'modals.cancel': 'Decline & stop',

    'agent.ops.commands': 'Ran {n} command(s) in {ms}ms',
    'agent.ops.files': 'Edited {n} file(s)',
    'agent.ops.mixed': 'Performed {n} operation(s) ({cmd} commands + {file} file changes)',
    'agent.ops.toolOutputs': 'Ran {n} tool(s)',
    'agent.ops.commandsSummary': '{n} command(s)',
    'agent.ops.filesSummary': '{n} file(s)',
    'agent.ops.expandAll': 'Expand all',
    'agent.ops.collapseAll': 'Collapse all',
    'agent.ops.showMore': 'Show more ({n})',
    'agent.tool.command': 'Run command',
    'agent.tool.fileChange': 'Edit file',
    'agent.tool.readOnly': 'Read info',
    'agent.tool.webSearch': 'Web search',
    'agent.tool.unknown': 'Invoke tool',
    // ── Task 6: tool status badges + error feedback ─────────────────────
    // Used in ToolDetailContent.vue 4-state visuals:
    //   running → label next to blue spinner
    //   success → label next to green checkmark
    //   error   → red ⚠️ badge / error details
    //   timeout → errorCode after 30s of no response
    //   duration → footer "Took {s}s" on tool cards
    'agent.tool.errorBadge': 'Tool Error',
    'agent.tool.running': 'Running...',
    'agent.tool.success': 'Success',
    'agent.tool.timeout': 'Timeout',
    'agent.tool.copyError': 'Copy Error',
    'agent.tool.duration': 'Took {s}s',
    'agent.tool.errorDetails': 'Error Details',
    // ── Task 7: zoom control button tooltips ─────────────────────
    // Floating "A- / A / A+" button group in AgentChat top-right
    'agent.zoom.in': 'Zoom In',
    'agent.zoom.out': 'Zoom Out',
    'agent.zoom.reset': 'Reset',

    // ── Plan / Todo block ─────────────────────────────
    'agent.plan': 'Plan',
    'agent.planEmpty': '(no plan yet)',
    'agent.planStatusPending': 'Pending',
    'agent.planStatusInProgress': 'In progress',
    'agent.planStatusCompleted': 'Done',
    'agent.streaming': 'Loading',

    // ── Active-state sub-labels (used to differentiate within the active set) ─────
    'agent.statusRunning': 'Running',
    'agent.statusEditing': 'Editing',
    'agent.statusThinking': 'Thinking',

    // ── Task 7: context auto-compression divider ─────────────────
    // Backend triggers auto-compaction when the running messages
    // exceed 80% of the model context window. The front-end
    // renders a non-expandable horizontal divider at the position
    // the compacted messages used to occupy.
    'agent.contextCompaction': 'Context auto-compressed',

    // ── Task 10: Slash command menu ("/" trigger) ─────────────────
    // Trigger: textarea content starts with "/". Two groups: "Features"
    // (static, 3 items: attach / plan-mode / permission-mode) and
    // "Skills" (dynamic, fetched from backend /api/skills on mount).
    'agent.slashMenuTitle': 'Slash commands',
    'agent.slashMenuFeatures': 'Features',
    'agent.slashMenuSkills': 'Skills',
    'agent.slashMenuNoMatches': 'No matches',
    'agent.slashMenuHint': '↑↓ navigate · Enter apply · Esc close',

    // ── Task 22: Agent Task Message (subagent sub-task list) ─────────
    // Backend SubagentDispatch event: AI splits complex tasks across
    // multiple subagents (parallel/serial) and the front-end renders
    // the sub-task list as a foldable block. Collapse thresholds
    // align with codex-web MessageBlocks.tsx:68-69 (7 lines / 520 chars).
    'agent.agentTask': 'Sub-tasks',
    'agent.subTaskProgress': '{done}/{total}',
    'agent.agentTaskEmpty': '(no sub-tasks)',

    // ── Task 26: LAN Access (LAN access URL panel) ─────────────────
    // Backend /api/network/lan-access enumerates URLs reachable from
    // a peer on the same WiFi. The Settings panel surfaces them so
    // the user can type http://192.168.x.x:5245/ into a phone/tablet
    // browser on the same network and reach the AI assistant.
    'agent.lanAccess': 'LAN access',
    'agent.lanAccessHelp': 'Devices on the same WiFi can use these addresses',
    'agent.lanAccessEmpty': 'No usable network interface found',
    'agent.lanAccessRefresh': 'Refresh',
    'agent.lanAccessCopy': 'Copy',
    'agent.lanAccessCopied': 'Copied {url}',
    'agent.lanAccessCopyFailed': 'Copy failed',
    'agent.lanAccessInterface': 'Interface: {name}',
    'agent.lanAccessUse': 'Use',
    'agent.lanAccessUseTitle': 'Use this address as backend baseUrl',
    'agent.lanAccessUseSuccess': 'Switched to {url}',
    'agent.lanAccessUseFailed': 'Switch failed',

    // ── Task 25: Sync Doctor (redacted diagnostic) ─────────────
    // Triggered from the Settings panel; the report is shown
    // raw to the user for bug-reporting purposes (all secrets
    // have already been Redacted server-side).
    'agent.syncDoctor': 'Run sync doctor',
    'agent.syncDoctorRunning': 'Generating diagnostic report…',
    'agent.syncDoctorResult': 'Diagnostic result',
    'agent.syncDoctorCopy': 'Copy JSON',
    'agent.syncDoctorCopied': 'Diagnostic JSON copied',
    'agent.syncDoctorCopyFailed': 'Copy failed',
    'agent.syncDoctorFailed': 'Doctor failed: {msg}',
    'agent.syncDoctorEmpty': 'No issues detected',

    // ── Agent Mock Mode (scripted replay, no cost, no real LLM) ─────────
    'agent.mockBadge': 'Mock',
    'agent.mockBadgeTooltip': 'Mock mode active (no cost), scenario: {scenario}',
    'agent.mockMode': 'Mock Mode',
    'agent.mockModeOff': 'Off (Real API)',
    'agent.mockModeBuiltin': 'Built-in Scenarios',
    'agent.mockModeCustom': 'Custom Scenarios',
    'agent.mockModeSet': 'Switched to: {mode}',
    'agent.mockModeSetFailed': 'Failed to switch mock mode',
    // Mock mode preset chips (chips overlaid on the input area)
    'agent.mockPresetBarAria': 'Mock mode preset inputs',
    'agent.mockPresetBarDefaultScenario': 'Scenario',
    'agent.mockPresetBarHint': 'Click to send',
    'agent.mockPresetBarPickerScenario': 'Scenario Library',
    // v2 multi-round / branch scenarios (see .trae/specs/agent-tools-scenarios-v2/spec.md)
    'agent.branchChoicePrompt': 'Choose an action:',
    'agent.roundProgress': 'Round {round}/{total}',
    'agent.roundPausedHint': 'Click a chip or type to continue',
    'agent.toolDenied': 'Tool denied',
    'agent.toolRequiresConfirm': 'Tool requires confirmation',
    'agent.batchRenamePreview': 'Rename preview ({count} files)',
    'agent.batchRenameConfirm': 'Confirm rename',
    'agent.editMetadataTitle': 'Edit metadata',
    'agent.commandTimeout': 'Command timeout',
    'agent.commandDenied': 'Command not in whitelist',
    // ── v2 工具快捷动作 chip 行 ───────────────────────────
    'agent.v2Chip.search': '🔍 Search',
    'agent.v2Chip.searchTitle': 'search_files: recursive + glob + AND/OR/NOT',
    'agent.v2Chip.searchPrompt': 'Recursively find all mp4 files larger than 100MB, sort by mtime desc, top 10',
    'agent.v2Chip.read': '📖 Read',
    'agent.v2Chip.readTitle': 'read_file_v2: pagination + binary detection',
    'agent.v2Chip.readPrompt': 'Use read_file_v2 to read the first 200 lines of clip.mp4',
    'agent.v2Chip.metadata': 'ℹ️ Metadata',
    'agent.v2Chip.metadataTitle': 'get_metadata: basic fields + ffprobe media probe',
    'agent.v2Chip.metadataPrompt': 'Use get_metadata to detect resolution/duration/codec of vacation_2024.mp4',
    'agent.v2Chip.editMetadata': '🏷️ Edit meta',
    'agent.v2Chip.editMetadataTitle': 'edit_metadata: 4-round wizard write',
    'agent.v2Chip.editMetadataPrompt': 'Change the ID3 title of song.mp3 to "Vacation 2024"',
    'agent.v2Chip.batchRename': '🔄 Rename',
    'agent.v2Chip.batchRenameTitle': 'batch_rename: dry_run preview → confirm → execute',
    'agent.v2Chip.batchRenamePrompt': 'Rename all .JPG files under /photos to .jpg (dry_run first)',
    'agent.v2Chip.command': '⚙️ Shell',
    'agent.v2Chip.commandTitle': 'command_run: restricted shell (whitelist + timeout + truncation)',
    'agent.v2Chip.commandPrompt': 'Use ffprobe to dump full metadata JSON of vacation_2024.mp4',
    // ── v2 剧本演示入口（弹窗列表） ────────────────────────
    'agent.v2Scenarios.btn': 'v2 Scenarios',
    'agent.v2Scenarios.btnTitle': 'Demo 8 v2 mock scenarios (branch + multi-round)',
    'agent.v2Scenarios.title': 'v2 Scenarios',
    'agent.v2Scenarios.hint': 'Clicking a scenario auto-switches to "built-in mock" mode and sends the trigger keyword. All scenarios go through the real ToolRegistry and execute real tools (search_files / command_run / batch_rename).',
    'agent.v2Scenarios.mock': 'MOCK',
    'agent.v2Scenarios.triggerKw': 'kw',
    'agent.v2Scenarios.groupSearch': 'Search',
    'agent.v2Scenarios.groupRead': 'Read / metadata / shell',
    'agent.v2Scenarios.groupWrite': 'Write',
    'agent.v2Scenarios.groupBranch': 'Branch',
    'agent.v2Scenarios.s.recursiveMp4': 'Recursive + glob *.mp4 + size > 100MB',
    'agent.v2Scenarios.s.logicalQuery': 'AND(size_gt + mtime_after + ext_eq) compound query',
    'agent.v2Scenarios.s.contentRegex': 'content_regex "ERROR.*timeout" full-text search',
    'agent.v2Scenarios.s.readFileV2': 'read_file_v2 paginated read (start_line/end_line)',
    'agent.v2Scenarios.s.getMetadata': 'get_metadata video/audio probe (requires ffprobe)',
    'agent.v2Scenarios.s.commandRun': 'command_run ffprobe restricted shell',
    'agent.v2Scenarios.s.editMetadata': '4-round multi-turn: pick file → field → value → confirm',
    'agent.v2Scenarios.s.batchRename': 'dry_run preview → confirm → real execution',
    'agent.v2Scenarios.s.branchEncrypt': '3-way branch: encrypt / decrypt / cancel',
    'agent.v2Scenarios.s.branchVideo': 'video / audio / other multi-branch + cross-branch multi-round',
    // ── v2 工具调用 badge ──────────────────────────────────
    'agent.v2Badge': 'v2',
    'agent.v2BadgeTitle': 'v2 tool (recursive search / metadata / restricted shell)',

    // ── Task 4: tool card status / duration i18n ─────────────
    'agent.toolStatusPending': 'Pending',
    'agent.toolStatusRunning': 'Running',
    'agent.toolStatusSuccess': 'Success',
    'agent.toolStatusFailed': 'Failed',
    'agent.toolStatusCancelled': 'Cancelled',
    // Duration: {ms} placeholder is replaced by formatDuration() output
    // e.g. "1.2s" / "850ms" / "1m 23s"
    'agent.toolDuration': 'took {ms}',
    'agent.toolDurationLong': 'took a long time',

    // FileReferenceChip actions
    'agent.fileRefCopyPath': 'Copy path',
    'agent.fileRefCopyRelative': 'Copy relative path',
    'agent.fileRefOpenInFiles': 'Open in Files',

    // Scroll to bottom button
    'agent.scrollToBottom': 'Scroll to bottom',

    // Close button
    'agent.close': 'Close',

    // Tool cards titles
    'agent.toolCards.fileContentTitle': 'File Content',
    'agent.toolCards.fileListTitle': 'File List',
    'agent.toolCards.fileStatTitle': 'File Info',
    'agent.toolCards.mountsTitle': 'Mounts',
    'agent.toolCards.parseFailed': 'Parse failed',
  },
}
