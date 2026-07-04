/**
 * 错误分析器 — 根据错误信息推断分类、生成修复建议、构造错误链路
 *
 * 设计目标：在后端尚未提供结构化错误之前，前端能够从原始错误字符串推断
 * 1. 错误分类（网络/权限/不支持的版本/插件崩溃等）
 * 2. 错误链路（提交 → 网络 → HTTP → 后端 → 插件）
 * 3. 修复建议（按问题类型给出可操作的步骤）
 *
 * 关键设计：
 * - 规则数组按 specificity 排序：先尝试最具体的匹配
 * - 每个规则独立返回分析结果，多个规则叠加时取首个最高优先级
 * - unknown 兜底：永远返回某种分析，永不返空
 */

export type ErrorPhase = "submission" | "network" | "http" | "backend" | "plugin" | "unknown";
export type ErrorSeverity = "info" | "warning" | "error";

export type ErrorCategory =
  | "network"
  | "auth"
  | "not_found"
  | "server_error"
  | "unsupported_version"
  | "unsupported_cipher"
  | "unsupported_compression"
  | "wrong_password"
  | "file_not_found"
  | "permission_denied"
  | "oom"
  | "timeout"
  | "plugin_crash"
  | "invalid_request"
  | "unknown";

export interface ErrorChainStep {
  phase: ErrorPhase;
  title: string;
  detail: string;
  severity: ErrorSeverity;
}

export interface FixSuggestion {
  title: string;
  detail: string;
  codeHint?: string;
  docUrl?: string;
}

export interface ErrorAnalysis {
  category: ErrorCategory;
  phase: ErrorPhase;
  summary: string;
  technicalExplanation: string;
  chain: ErrorChainStep[];
  fixes: FixSuggestion[];
}

// ==================== 规则定义 ====================

interface ErrorRule {
  id: string;
  /** 匹配关键字（任一命中即匹配，大小写不敏感） */
  keywords: string[];
  /** 排除关键字（任一命中即跳过） */
  exclude?: string[];
  category: ErrorCategory;
  phase: ErrorPhase;
  summary: string;
  technicalExplanation: string;
  fixes: FixSuggestion[];
}

const RULES: ErrorRule[] = [
  // 高优先级：超时
  {
    id: "timeout",
    keywords: ["timeout", "timed out", "etimedout", "aborted"],
    category: "timeout",
    phase: "network",
    summary: "请求超时",
    technicalExplanation: "客户端等待后端响应超过阈值。可能是后端处理慢、网络抖动或插件死锁。",
    fixes: [
      { title: "检查后端进程是否存活", detail: "通过日志面板或 adb logcat 确认 encv-go 主进程无 GC 暂停或死锁。" },
      { title: "增加客户端超时阈值", detail: "真机弱网环境可考虑放宽到 60s+。" },
      { title: "复测", detail: "网络抖动通常是偶发，retry 一次。" },
    ],
  },
  // 网络层失败
  {
    id: "network",
    keywords: ["failed to fetch", "network request failed", "econnrefused", "econnreset", "enetunreach", "websocket", "socket hang up"],
    category: "network",
    phase: "network",
    summary: "网络层失败",
    technicalExplanation: "HTTP 请求未到达后端或响应未返回。可能是 WebView native proxy、CORS、DNS 或 encv-go 未启动。",
    fixes: [
      { title: "确认 encv-go 后端运行中", detail: "检查 2025 端口是否监听：adb shell ss -tlnp | grep 2025" },
      { title: "检查 WebView proxy", detail: "在 Settings → 开发者选项 → 网络日志看 fetch 链路。" },
      {
        title: "真机 native fetch 限制",
        detail: "useProxiedFetch.ts 在 native 模式下替换了原生 fetch，确认 headers 已带 Accept: text/event-stream。",
      },
    ],
  },
  // 权限（必须在 auth 之前——"permission denied" 子串更具体）
  {
    id: "permission",
    keywords: ["eacces", "not allowed", "sandbox", "mandatory access"],
    exclude: ["cipher", "compression"],
    category: "permission_denied",
    phase: "submission",
    summary: "权限被拒",
    technicalExplanation: "后端无法访问该路径。Android 11+ scoped storage 限制了 /Android/data/ 等目录的访问。",
    fixes: [
      { title: "使用 /storage/emulated/0/encv-automation", detail: "这是 app 可写的安全路径，避开了 scoped storage 限制。" },
      { title: "真机检查 storage 权限", detail: "Settings → 应用权限 → 存储。" },
      { title: "dev 模式用 /mock 路径", detail: "dev 模式 vite 提供 /mock/* 虚拟路径，无需磁盘权限。" },
    ],
  },
  // 401/403 鉴权（"permission denied" 子串被 permission 优先匹配，auth 只保留 401/403 等无歧义信号）
  {
    id: "auth",
    keywords: ["401", "unauthorized", "403", "forbidden"],
    category: "auth",
    phase: "http",
    summary: "鉴权失败",
    technicalExplanation: "后端拒绝了请求。可能是会话过期、API key 错误或权限不足。",
    fixes: [
      { title: "检查全局密码是否已设置", detail: "Settings → 加密设置 → 全局密码。自动化测试密码为 automation-test-pwd。" },
      { title: "重新登录", detail: "如果使用远程后端，token 可能过期。" },
    ],
  },
  // 文件不存在（必须在 not_found 之前——"no such file or directory" 子串更具体）
  {
    id: "file_not_found",
    keywords: ["source file not found", "file does not exist", "cannot find file", "os.stat", "no such file or directory"],
    category: "file_not_found",
    phase: "submission",
    summary: "源文件不存在",
    technicalExplanation: "提交任务时源文件路径在磁盘上找不到。可能是 Mock 未生成、路径被安全边界改写后失效、或用户数据已被移动。",
    fixes: [
      { title: "重新生成 Mock", detail: '点击"生成 Mock 数据"按钮，等待 toast 成功后再运行测试。' },
      { title: "检查安全边界", detail: "withSafetyBoundary 把源路径改写到 encv-automation 命名空间，确认该目录存在。" },
      { title: "用 adb shell 检查", detail: "adb shell ls /storage/emulated/0/encv-automation/01-plain-media/ 看到 sample.mp4 等。" },
    ],
  },
  // 404
  {
    id: "not_found",
    keywords: ["404", "not found", "no such file", "enoent"],
    exclude: ["file", "or directory"],
    category: "not_found",
    phase: "http",
    summary: "资源未找到",
    technicalExplanation: "请求的资源不存在。可能是 mock 文件未生成、路径拼写错误或后端路由缺失。",
    fixes: [
      { title: "先生成 Mock 数据", detail: '在自动化测试页面点击"生成 Mock 数据"，确认 encv-automation 目录非空。' },
      { title: "检查源文件路径", detail: "用 adb shell ls /storage/emulated/0/encv-automation/01-plain-media/video/sample.mp4 确认存在。" },
      { title: "检查后端路由", detail: "curl http://127.0.0.1:2025/health 确认后端可达。" },
    ],
  },
  // 不支持的版本
  {
    id: "unsupported_version",
    keywords: [
      "unsupported version",
      "version not supported",
      "invalid version",
      "unknown version",
      "version 2",
      "version 3",
      "deprecated",
    ],
    exclude: ["cipher", "compression"],
    category: "unsupported_version",
    phase: "plugin",
    summary: "插件不支持此容器版本",
    technicalExplanation: "当前 plugin 不支持测试用例的容器版本（ECv2/ECv3 已废弃，仅部分老 plugin 保留）。",
    fixes: [
      { title: "关闭 includeDeprecated 开关", detail: "在测试运行器配置中把 includeDeprecated 设为 false，跳过 ECv2/ECv3 用例。" },
      { title: "升级 plugin", detail: "在 plugin 仓库检查最新版本是否支持 ECv4。" },
      { title: "标记 might-fail", detail: "ECv2/ECv3 用例 expectedBehavior 已设为 might-fail，可视为预期失败。" },
    ],
  },
  // 不支持的 cipher mode
  {
    id: "unsupported_cipher",
    keywords: ["cipher", "aes-256", "aes-128", "ciphermode", "c0", "c1"],
    category: "unsupported_cipher",
    phase: "plugin",
    summary: "Cipher 模式不兼容",
    technicalExplanation: "plugin 不支持此 cipher mode（c0=AES-128, c1=AES-256）。可能是 ECv4 容器用 c1 但 plugin 限制为 c0。",
    fixes: [
      { title: "检查 plugin 的 cipher 支持", detail: "查看 plugin 元数据 supportedCipherModes 或文档。" },
      { title: "只测 c0", detail: "暂时把测试用例的 cipher 维度缩为 [0]，定位后扩展。" },
      { title: "对比 ECv4 容器 spec", detail: "ECv4 容器支持 c0/c1，下游 plugin 必须升级才能解密 c1 容器。" },
    ],
  },
  // 不支持的 compression
  {
    id: "unsupported_compression",
    keywords: ["compression", "zstd", "compress"],
    category: "unsupported_compression",
    phase: "plugin",
    summary: "压缩模式不兼容",
    technicalExplanation: "plugin 不支持 zstd 压缩或解压缩失败。ECv4 容器压缩模式 none/zstd。",
    fixes: [
      { title: "检查后端是否安装 zstd 库", detail: "encv-go 主进程依赖 zstd C 库，缺失会导致 zstd 解压失败。" },
      { title: "只测 none", detail: '暂时把测试用例的 compression 维度缩为 ["none"]，定位后扩展。' },
      { title: "对比 ECv4 spec", detail: "ECv4 容器 spec 规定 compression 字段为 none/zstd，plugin 端必须同时支持。" },
    ],
  },
  // 密码错
  {
    id: "wrong_password",
    keywords: ["wrong password", "invalid password", "bad password", "decrypt failed", "gcm auth fail", "authentication failed"],
    category: "wrong_password",
    phase: "backend",
    summary: "密码错误",
    technicalExplanation: "解密时密码不匹配。GCM 模式会通过 auth tag 校验密码，错误时抛出此异常。",
    fixes: [
      { title: "确认密码正确", detail: "自动化测试统一使用 automation-test-pwd。手动测试时确认密码输入无误。" },
      {
        title: "检查 global vs independent",
        detail: "plugin 的 passwordStrategy 决定是否走全局密码。global 模式下必须先在 Settings 设置。",
      },
      { title: "检查二次密码", detail: "ECv4 容器支持 secondary password（独立 AES key），缺失会导致 auth 失败。" },
    ],
  },
  // OOM
  {
    id: "oom",
    keywords: ["out of memory", "oom", "cannot allocate", "killed", "sigkill"],
    category: "oom",
    phase: "backend",
    summary: "内存不足",
    technicalExplanation: "后端进程被系统 OOM killer 终止。可能是处理大文件（≥1GB）时内存峰值过高。",
    fixes: [
      { title: "减小测试文件", detail: "boundary 测试包含 1MB 文件，可暂时跳过 large-1mb.dat。" },
      { title: "分批运行", detail: "把用例拆成 5-10 个一组，避免并发提交。" },
      { title: "检查 GC 配置", detail: "GOGC 调高（如 200）可减少 GC 频率。" },
    ],
  },
  // 插件崩溃
  {
    id: "plugin_crash",
    keywords: ["plugin", "crash", "panic", "nil pointer", "segfault", "signal 11", "signal 6", "abort"],
    category: "plugin_crash",
    phase: "plugin",
    summary: "插件进程崩溃",
    technicalExplanation: "plugin 子进程异常退出（Go panic / native crash）。plugin 进程独立于主进程，崩溃不会影响主进程。",
    fixes: [
      { title: "查看 plugin 日志", detail: "DevLogs 面板过滤 plugin-* 标签，或 adb logcat | grep plugin" },
      { title: "复现最小用例", detail: "只跑这一个 plugin + taskType + version，定位具体输入。" },
      { title: "上报 plugin 仓库", detail: "附上调用栈 + 输入文件，给 plugin 维护者。" },
    ],
  },
  // HTTP 5xx
  {
    id: "server_5xx",
    keywords: ["500", "502", "503", "504", "internal server error", "bad gateway", "service unavailable"],
    category: "server_error",
    phase: "http",
    summary: "后端 5xx 错误",
    technicalExplanation: "后端 panic 或未处理异常。可能是 nil 解引用、类型断言失败或 plugin 通信错误。",
    fixes: [
      { title: "查看后端日志", detail: "DevLogs 面板查看 ERROR 级别日志，或 adb logcat | grep encv-go" },
      { title: "复测", detail: "5xx 可能是 panic 后服务自动恢复，retry 通常成功。" },
      { title: "上报", detail: "若可稳定复现，附 taskId + 复现步骤上报。" },
    ],
  },
];

// ==================== 主分析函数 ====================

/**
 * 根据原始错误字符串生成完整分析。
 * 永远返回非空对象（最坏情况返 unknown + 兜底修复建议）。
 */
export function analyzeError(
  rawMessage: string,
  options?: {
    httpStatus?: number;
    phase?: ErrorPhase;
  }
): ErrorAnalysis {
  const lower = rawMessage.toLowerCase();
  const matched = RULES.find(rule => {
    if (rule.exclude?.some(kw => lower.includes(kw.toLowerCase()))) return false;
    return rule.keywords.some(kw => lower.includes(kw.toLowerCase()));
  });

  if (matched) {
    return {
      category: matched.category,
      phase: options?.phase ?? matched.phase,
      summary: matched.summary,
      technicalExplanation: matched.technicalExplanation,
      chain: buildChain(matched.phase, matched.summary, rawMessage, options),
      fixes: matched.fixes,
    };
  }

  // HTTP 状态码兜底
  const status = options?.httpStatus;
  if (status && status >= 500) {
    return analyzeError("500 internal server error", options);
  }
  if (status === 404) {
    return analyzeError("404 not found", options);
  }
  if (status === 401 || status === 403) {
    return analyzeError("403 forbidden", options);
  }

  // 兜底：unknown
  return {
    category: "unknown",
    phase: options?.phase ?? "unknown",
    summary: "未分类错误",
    technicalExplanation: "无法从错误信息推断根因。可能是新引入的失败模式，需要查看后端日志定位。",
    chain: buildChain("unknown", "未分类", rawMessage, options),
    fixes: [
      { title: "查看后端日志", detail: "DevLogs 面板查看完整调用栈或 adb logcat | grep encv-go。" },
      { title: "复测", detail: "若非稳定复现，可能是环境波动。" },
      { title: "上报", detail: "附原始错误字符串 + 复现步骤。" },
    ],
  };
}

/**
 * 构造错误链路：根据 phase 推断出多步链路
 * 即使后端只返回 1 行 message，也展示完整 phase 链路
 */
function buildChain(
  failedPhase: ErrorPhase,
  failedSummary: string,
  rawMessage: string,
  options?: { httpStatus?: number }
): ErrorChainStep[] {
  const allPhases: Array<{ phase: ErrorPhase; title: string; defaultDetail: string }> = [
    { phase: "submission", title: "提交", defaultDetail: "前端调用 createTask(sourcePath, targetPath, ...)" },
    { phase: "network", title: "网络传输", defaultDetail: "fetch /api/tasks POST 请求" },
    { phase: "http", title: "HTTP 响应", defaultDetail: options?.httpStatus ? `HTTP ${options.httpStatus}` : "HTTP 2xx/4xx/5xx" },
    { phase: "backend", title: "后端处理", defaultDetail: "encv-go 解析请求并路由" },
    { phase: "plugin", title: "插件执行", defaultDetail: "plugin 进程加解密 / 转码" },
  ];
  const failedIdx = allPhases.findIndex(p => p.phase === failedPhase);

  return allPhases.map((p, idx) => {
    if (idx < failedIdx) {
      return { phase: p.phase, title: p.title, detail: p.defaultDetail, severity: "info" };
    }
    if (idx === failedIdx) {
      return { phase: p.phase, title: p.title, detail: rawMessage || failedSummary, severity: "error" };
    }
    return { phase: p.phase, title: p.title, detail: "（未到达）", severity: "info" };
  });
}

// ==================== 辅助：分类元数据 ====================

/** 错误分类的显示元数据（图标、颜色、标签） */
export const CATEGORY_META: Record<ErrorCategory, { label: string; color: string; severity: ErrorSeverity }> = {
  network: { label: "网络", color: "#8B7355", severity: "error" },
  auth: { label: "鉴权", color: "#A0522D", severity: "error" },
  not_found: { label: "未找到", color: "#8B1E3F", severity: "error" },
  server_error: { label: "服务器", color: "#5B0F1F", severity: "error" },
  unsupported_version: { label: "版本", color: "#5C5470", severity: "warning" },
  unsupported_cipher: { label: "密码学", color: "#2B3A67", severity: "warning" },
  unsupported_compression: { label: "压缩", color: "#2B3A67", severity: "warning" },
  wrong_password: { label: "密码", color: "#8B1E3F", severity: "error" },
  file_not_found: { label: "文件", color: "#8B1E3F", severity: "error" },
  permission_denied: { label: "权限", color: "#A0522D", severity: "error" },
  oom: { label: "内存", color: "#5B0F1F", severity: "error" },
  timeout: { label: "超时", color: "#8B7355", severity: "error" },
  plugin_crash: { label: "插件", color: "#5B0F1F", severity: "error" },
  invalid_request: { label: "请求", color: "#8B1E3F", severity: "error" },
  unknown: { label: "未知", color: "#5C5470", severity: "warning" },
};
