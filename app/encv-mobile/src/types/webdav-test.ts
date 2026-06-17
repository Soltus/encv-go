/**
 * WebDAV 自动化测试类型定义
 *
 * 🆕 2026-06-17：声明式重构（multi-mount-storage-refactor spec 续）
 *   之前：硬编码 switch case + 18 case 一次跑完
 *   现在：TestDescriptor 数据驱动 + 7 module 独立 runner + 6 类攻防场景
 */

// ============ Manifest（后端返回）============

export interface WebDavManifestEntry {
  /** 相对 mount root 的路径，不含前导 / */
  virtual_path: string
  name: string
  is_dir: boolean
  size: number
  mod_time: number
  /** 物理容器路径（仅虚拟文件有） */
  container?: string
}

export interface WebDavContainerMapping {
  virtual_path: string
  container_path: string
  mount_name: string
}

export interface WebDavMountManifestInner {
  index_ready: boolean
  index_stats: {
    totalFiles: number
    totalDirs: number
    containers: number
    source: string
  }
  virtual_tree: WebDavManifestEntry[]
  container_map: WebDavContainerMapping[]
  registered_container_exts: string[]
}

export interface WebDavMountManifest {
  name: string
  mount_path: string
  root_path: string
  webdav_path: string
  is_default: boolean
  manifest: WebDavMountManifestInner
}

export interface WebDavManifestResponse {
  mounts: WebDavMountManifest[]
  server_base: string
}

// ============ Runner 共享上下文 ============

export interface WebDavAuth {
  username?: string
  password?: string
}

export interface WebDavTestContext {
  /** 后端拉取的完整 manifest（按 mount 分组） */
  manifest: WebDavManifestResponse
  /** 后端 API base URL（如 http://127.0.0.1:2025） */
  serverBaseUrl: string
  /** webdav path（如 /webdav 或 /d/automation） */
  webdavPath: string
  /** Basic Auth 信息（从 local-info 拉取） */
  auth: WebDavAuth
  /** 当前选中的 mount（用于 test module 内构造 URL） */
  activeMount: WebDavMountManifest
  /** 共享状态：跨 case 传递中间结果（如 attack / concurrency） */
  shared: Record<string, unknown>
  /** runtime 注入的 abort signal（用户取消时中断） */
  abortSignal?: AbortSignal
}

// ============ Test 模块与 case ============

/** 6 类攻防类型（attack / edge module 内部 case 分类） */
export type AttackType =
  | 'container-visibility'  // 容器可见性泄露
  | 'cross-mount-escape'    // 跨 mount 逃逸
  | 'index-rebuild-race'    // index 重建 race
  | 'resource-exhaustion'   // 资源耗尽
  | 'protocol-consistency'  // 协议一致性 / 边界
  | 'concurrency-stress'    // 并发压力
  | 'large-payload'         // 大文件传输
  | 'slow-network'          // 慢网络 / 超时

export type HttpMethod =
  | 'GET' | 'POST' | 'PUT' | 'DELETE' | 'HEAD'
  | 'OPTIONS' | 'PROPFIND' | 'PROPPATCH' | 'MKCOL'
  | 'COPY' | 'MOVE' | 'LOCK' | 'UNLOCK' | 'PATCH'

/** 期望失败结构（customAssert 返回） */
export interface AssertionFailure {
  message: string
  expected?: unknown
  actual?: unknown
}

export interface TestExpectations {
  /** 期望 status code（number 或数组） */
  status?: number | number[]
  /** 排除的 status code */
  statusNotIn?: number[]
  /** body 必须匹配（regex 或 substring） */
  bodyMatches?: RegExp | string
  /** body 必须不匹配（regex 或 substring） */
  bodyNotMatches?: RegExp | string
  /** 响应时间限制（ms） */
  responseTimeMs?: { max: number }
  /** 必须包含的 header（key/value 包含关系） */
  headerContains?: Record<string, string>
}

/** TestDescriptor 是声明式 test case 定义。
 * Runner 解释此结构执行 + 验证，避免硬编码 switch。 */
export interface TestDescriptor {
  /** 唯一 id（module 内） */
  id: string
  /** i18n key（test runner 用 t() 翻译） */
  nameI18n: string
  /** i18n key（详细描述） */
  descI18n: string
  /** 所属 module id */
  module: string
  /** 6 类攻防标记（attack / edge module 用） */
  attackType?: AttackType
  /** HTTP 方法 */
  method: HttpMethod
  /** URL（可动态构造） */
  url: string | ((ctx: WebDavTestContext) => string)
  /** 请求头 */
  headers?: Record<string, string> | ((ctx: WebDavTestContext) => Record<string, string>)
  /** 请求 body */
  body?: string | null | ((ctx: WebDavTestContext) => string | null)
  /** 期望 */
  expect: TestExpectations
  /** 自定义断言（高级 case 用） */
  customAssert?: (response: Response, body: string, ctx: WebDavTestContext) => AssertionFailure | null
  /** 并发数（默认 1） */
  concurrency?: number
  /** 重复次数（默认 1） */
  iterations?: number
  /** 超时（ms，默认 15000） */
  timeoutMs?: number
  /** 动态 skip（manifest 缺失某个资源时跳过） */
  skip?: boolean | ((ctx: WebDavTestContext) => boolean)
  /** 运行前钩子（用于挂定时器 / 准备外部状态）；返回 cleanup 函数 */
  beforeRun?: (ctx: WebDavTestContext) => Promise<(() => void) | void> | (() => void) | void
  /** 运行后钩子（不影响 result status） */
  afterRun?: (ctx: WebDavTestContext) => Promise<void> | void
  /** 顺序号（用于排序） */
  order?: number
}

/** TestModule 是 test case 集合（auth / basic / protocol / ...） */
export interface TestModule {
  id: string
  nameI18n: string
  descI18n: string
  icon: string
  color: string
  cases: TestDescriptor[]
}

// ============ Test Run 结果与持久化 ============

export type TestCaseStatus = 'pending' | 'running' | 'success' | 'failure' | 'skipped' | 'timed_out'

export interface TestCaseResult {
  id: string
  name: string
  module: string
  status: TestCaseStatus
  durationMs: number
  httpStatus?: number
  error?: string
  errorKind?: 'http_4xx' | 'http_5xx' | 'timeout' | 'network' | 'assertion' | 'unknown'
  /** 错误详情（用于 UI 展开查看） */
  details?: string
  /** 并发/重复 case 包含的多次响应 */
  iterations?: { status: number; durationMs: number; passed: boolean }[]
}

export interface TestRun {
  id: string
  startedAt: string
  completedAt?: string
  module: string
  totalCases: number
  passed: number
  failed: number
  skipped: number
  results: TestCaseResult[]
}
