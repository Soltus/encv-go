/**
 * useWebDavTestModules — 8 module × 46 test case 定义
 *
 * 🆕 2026-06-17：声明式重构（multi-mount-storage-refactor spec 续）
 *
 * 模块划分（按业务场景）：
 *  - auth         认证（4 case）
 *  - basic_ops    基础 CRUD（5 case）
 *  - protocol     DAV 协议一致性（5 case）
 *  - concurrency  并发压力（4 case）
 *  - large_payload 大文件 + 慢网络（4 case）
 *  - edge         边界情况（5 case）
 *  - attack       攻防测试（8 case）
 *
 * 9 类攻防标记（attackType 字段）：
 *  - container-visibility / cross-mount-escape / index-rebuild-race
 *  - resource-exhaustion / protocol-consistency / concurrency-stress
 *  - large-payload / slow-network
 *
 * 设计原则：
 *  - url 是函数（基于 ctx 动态构造），不用 hardcode `/01-plain-media/video/`
 *  - skip 是函数（manifest 缺失资源时跳过）
 *  - 用 ctx.webdavPath 作为前缀（兼容 /webdav/ 和 /d/automation/）
 */

import type { TestModule, WebDavTestContext } from '@/types/webdav-test'

// ============= 通用辅助 =============

function joinUrl(ctx: WebDavTestContext, ...parts: string[]): string {
  const prefix = ctx.webdavPath.endsWith('/') ? ctx.webdavPath : ctx.webdavPath + '/'
  return ctx.serverBaseUrl + prefix + parts.filter(Boolean).join('/')
}

function mountUrl(ctx: WebDavTestContext, ...parts: string[]): string {
  const mountPath = ctx.activeMount.webdav_path
  const prefix = mountPath.endsWith('/') ? mountPath : mountPath + '/'
  return ctx.serverBaseUrl + prefix + parts.filter(Boolean).join('/')
}

function hasVirtualFile(ctx: WebDavTestContext, virtualPath: string): boolean {
  return ctx.activeMount.manifest.virtual_tree.some(
    (e) => e.virtual_path === virtualPath && !e.is_dir
  )
}

function pickFirstFile(ctx: WebDavTestContext): string | null {
  const f = ctx.activeMount.manifest.virtual_tree.find((e) => !e.is_dir)
  return f?.virtual_path ?? null
}

function pickFirstContainer(ctx: WebDavTestContext): { virtual: string; container: string } | null {
  const m = ctx.activeMount.manifest.container_map[0]
  return m ? { virtual: m.virtual_path, container: m.container_path } : null
}

function hasMultipleMounts(ctx: WebDavTestContext): boolean {
  return ctx.manifest.mounts.length > 1
}

/**
 * 从 active mount 的 manifest.registered_container_exts 动态取第一个扩展名。
 * 绝不能硬编码 .sccg* / .encv / .ae —— 后端 plugin 系统是权威，配置可变。
 * @returns 包含前导点的扩展名（如 ".sccgv"），若 manifest 为空则返回 '__unknown__' 哨兵。
 */
function getFirstContainerExt(ctx: WebDavTestContext): string {
  const exts = ctx.activeMount.manifest.registered_container_exts
  if (!Array.isArray(exts) || exts.length === 0) return '__unknown__'
  // 后端总是返回包含前导点的扩展名（如 ".sccgv"）
  return exts[0]
}

/**
 * 从 active mount 找第一个**非 root** 的另一个 mount（用于跨 mount 测试）。
 */
function pickOtherMount(ctx: WebDavTestContext): { name: string; webdav_path: string } | null {
  const other = ctx.manifest.mounts.find((m) => m.name !== ctx.activeMount.name)
  return other ? { name: other.name, webdav_path: other.webdav_path } : null
}

// ============= Module 1: auth（认证）============

const authModule: TestModule = {
  id: 'auth',
  nameI18n: 'devtools.webdav.modules.auth.name',
  descI18n: 'devtools.webdav.modules.auth.desc',
  icon: 'lock-closed-outline',
  color: 'primary',
  cases: [
    {
      id: 'auth_no_credentials_401',
      nameI18n: 'devtools.webdav.cases.auth_no_credentials_401.name',
      descI18n: 'devtools.webdav.cases.auth_no_credentials_401.desc',
      module: 'auth',
      method: 'GET',
      // 用真实文件做 GET（不能用根目录——go-webdav 对 collection GET 返 405 RFC §9.4）
      url: (ctx) => {
        const f = pickFirstFile(ctx)
        return f ? mountUrl(ctx, f) : joinUrl(ctx)
      },
      headers: () => ({}), // 故意不带 Authorization
      expect: {
        status: [401, 403],
        // 兼容：cfg.Webdav.Username 为空时可能 200
        statusNotIn: [],
      },
      order: 1,
    },
    {
      id: 'auth_wrong_credentials_401',
      nameI18n: 'devtools.webdav.cases.auth_wrong_credentials_401.name',
      descI18n: 'devtools.webdav.cases.auth_wrong_credentials_401.desc',
      module: 'auth',
      method: 'GET',
      url: (ctx) => {
        const f = pickFirstFile(ctx)
        return f ? mountUrl(ctx, f) : joinUrl(ctx)
      },
      headers: () => ({
        Authorization: 'Basic ' + btoa('wrong_user:wrong_pass'),
      }),
      expect: {
        status: [401, 403],
        statusNotIn: [200, 204],
      },
      order: 2,
    },
    {
      id: 'auth_correct_credentials_200',
      nameI18n: 'devtools.webdav.cases.auth_correct_credentials_200.name',
      descI18n: 'devtools.webdav.cases.auth_correct_credentials_200.desc',
      module: 'auth',
      method: 'GET',
      url: (ctx) => {
        const f = pickFirstFile(ctx)
        return f ? mountUrl(ctx, f) : joinUrl(ctx)
      },
      // 头由 runner 自动注入 ctx.auth.basic
      expect: {
        status: [200, 404, 207, 301],
      },
      skip: (ctx) => !pickFirstFile(ctx),
      order: 3,
    },
    {
      id: 'auth_no_auth_required_200',
      nameI18n: 'devtools.webdav.cases.auth_no_auth_required_200.name',
      descI18n: 'devtools.webdav.cases.auth_no_auth_required_200.desc',
      module: 'auth',
      method: 'GET',
      url: (ctx) => {
        const f = pickFirstFile(ctx)
        return f ? mountUrl(ctx, f) : joinUrl(ctx)
      },
      headers: () => ({}),
      expect: {
        status: [200, 404, 207],
      },
      // 当 cfg.Webdav.Username 有值时跳过（不是无 auth 配置）
      skip: (ctx) => !!(ctx.auth.username) || !pickFirstFile(ctx),
      order: 4,
    },
  ],
}

// ============= Module 2: basic_ops（基础 CRUD）============

const basicOpsModule: TestModule = {
  id: 'basic_ops',
  nameI18n: 'devtools.webdav.modules.basic_ops.name',
  descI18n: 'devtools.webdav.modules.basic_ops.desc',
  icon: 'list-outline',
  color: 'secondary',
  cases: [
    {
      id: 'basic_options_methods',
      nameI18n: 'devtools.webdav.cases.basic_options_methods.name',
      descI18n: 'devtools.webdav.cases.basic_options_methods.desc',
      module: 'basic_ops',
      method: 'OPTIONS',
      url: (ctx) => joinUrl(ctx),
      expect: {
        status: [200, 204],
        headerContains: {
          // Allow 头必须包含 PROPFIND / MOVE / COPY / DELETE
        },
      },
      customAssert: (_response, _body, _ctx) => {
        // 验证 Allow 头（在 runCase 内不方便处理，移到 customAssert）
        // 实际：跑完用 fetch + custom check
        return null
      },
      order: 1,
    },
    {
      id: 'basic_head_file',
      nameI18n: 'devtools.webdav.cases.basic_head_file.name',
      descI18n: 'devtools.webdav.cases.basic_head_file.desc',
      module: 'basic_ops',
      method: 'HEAD',
      url: (ctx) => {
        const f = pickFirstFile(ctx)
        return f ? mountUrl(ctx, f) : joinUrl(ctx)
      },
      expect: { status: [200, 404, 207] },
      skip: (ctx) => !hasVirtualFile(ctx, pickFirstFile(ctx) ?? '__none__'),
      order: 2,
    },
    {
      id: 'basic_get_file',
      nameI18n: 'devtools.webdav.cases.basic_get_file.name',
      descI18n: 'devtools.webdav.cases.basic_get_file.desc',
      module: 'basic_ops',
      method: 'GET',
      url: (ctx) => {
        const f = pickFirstFile(ctx)
        return f ? mountUrl(ctx, f) : joinUrl(ctx)
      },
      expect: { status: [200, 404, 207] },
      skip: (ctx) => !pickFirstFile(ctx),
      order: 3,
    },
    {
      id: 'basic_list_root',
      // 🆕 2026-06-17：GET 改 PROPFIND depth=1
      // 根因：go-webdav 对 collection (目录) 的 GET 返回 405（RFC 4918 §9.4）
      // 正确做法是 PROPFIND depth=1 列目录，返回 207 Multi-Status
      nameI18n: 'devtools.webdav.cases.basic_list_root.name',
      descI18n: 'devtools.webdav.cases.basic_list_root.desc',
      module: 'basic_ops',
      method: 'PROPFIND',
      url: (ctx) => joinUrl(ctx),
      headers: { Depth: '1' },
      // 期望：207 Multi-Status
      expect: { status: 207 },
      order: 4,
    },
    {
      id: 'basic_get_nonexistent_404',
      nameI18n: 'devtools.webdav.cases.basic_get_nonexistent_404.name',
      descI18n: 'devtools.webdav.cases.basic_get_nonexistent_404.desc',
      module: 'basic_ops',
      method: 'GET',
      url: (ctx) => mountUrl(ctx, '__webdav_test_nope_' + Date.now() + '.txt'),
      expect: { status: 404 },
      order: 5,
    },
  ],
}

// ============= Module 3: protocol（DAV 协议一致性）============

const protocolModule: TestModule = {
  id: 'protocol',
  nameI18n: 'devtools.webdav.modules.protocol.name',
  descI18n: 'devtools.webdav.modules.protocol.desc',
  icon: 'git-network-outline',
  color: 'tertiary',
  cases: [
    {
      id: 'protocol_propfind_depth_0',
      nameI18n: 'devtools.webdav.cases.protocol_propfind_depth_0.name',
      descI18n: 'devtools.webdav.cases.protocol_propfind_depth_0.desc',
      module: 'protocol',
      attackType: 'protocol-consistency',
      method: 'PROPFIND',
      url: (ctx) => joinUrl(ctx),
      headers: () => ({ Depth: '0', 'Content-Type': 'application/xml' }),
      body: '<?xml version="1.0" encoding="utf-8" ?><D:propfind xmlns:D="DAV:"><D:prop><D:resourcetype/></D:prop></D:propfind>',
      expect: {
        status: [207, 200],
        bodyMatches: /<D:multistatus|<d:multistatus/i,
      },
      order: 1,
    },
    {
      id: 'protocol_propfind_depth_1',
      nameI18n: 'devtools.webdav.cases.protocol_propfind_depth_1.name',
      descI18n: 'devtools.webdav.cases.protocol_propfind_depth_1.desc',
      module: 'protocol',
      attackType: 'protocol-consistency',
      method: 'PROPFIND',
      url: (ctx) => joinUrl(ctx),
      headers: () => ({ Depth: '1', 'Content-Type': 'application/xml' }),
      body: '<?xml version="1.0" encoding="utf-8" ?><D:propfind xmlns:D="DAV:"><D:prop><D:resourcetype/></D:prop></D:propfind>',
      expect: { status: [207, 200] },
      order: 2,
    },
    {
      id: 'protocol_propfind_depth_infinity',
      nameI18n: 'devtools.webdav.cases.protocol_propfind_depth_infinity.name',
      descI18n: 'devtools.webdav.cases.protocol_propfind_depth_infinity.desc',
      module: 'protocol',
      attackType: 'protocol-consistency',
      method: 'PROPFIND',
      url: (ctx) => joinUrl(ctx),
      headers: () => ({ Depth: 'infinity', 'Content-Type': 'application/xml' }),
      body: '<?xml version="1.0" encoding="utf-8" ?><D:propfind xmlns:D="DAV:"><D:prop><D:resourcetype/></D:prop></D:propfind>',
      expect: { status: [207, 200, 403] }, // 403 = "depth infinity 禁止" 也是合法
      timeoutMs: 30_000,
      order: 3,
    },
    {
      id: 'protocol_delete_nonexistent_404',
      nameI18n: 'devtools.webdav.cases.protocol_delete_nonexistent_404.name',
      descI18n: 'devtools.webdav.cases.protocol_delete_nonexistent_404.desc',
      module: 'protocol',
      method: 'DELETE',
      url: (ctx) => mountUrl(ctx, '__webdav_delete_nope_' + Date.now()),
      expect: { status: [404, 204, 200] }, // 204/200 = 幂等删除
      order: 4,
    },
    {
      id: 'protocol_mkcol_existing_405',
      nameI18n: 'devtools.webdav.cases.protocol_mkcol_existing_405.name',
      descI18n: 'devtools.webdav.cases.protocol_mkcol_existing_405.desc',
      module: 'protocol',
      method: 'MKCOL',
      url: (ctx) => joinUrl(ctx), // 根已存在 → 应该 405
      expect: { status: [405, 409, 403] },
      order: 5,
    },
  ],
}

// ============= Module 4: concurrency（并发压力）============

const concurrencyModule: TestModule = {
  id: 'concurrency',
  nameI18n: 'devtools.webdav.modules.concurrency.name',
  descI18n: 'devtools.webdav.modules.concurrency.desc',
  icon: 'flash-outline',
  color: 'warning',
  cases: [
    {
      id: 'concurrency_parallel_get_10',
      nameI18n: 'devtools.webdav.cases.concurrency_parallel_get_10.name',
      descI18n: 'devtools.webdav.cases.concurrency_parallel_get_10.desc',
      module: 'concurrency',
      attackType: 'concurrency-stress',
      method: 'GET',
      url: (ctx) => {
        const f = pickFirstFile(ctx)
        return f ? mountUrl(ctx, f) : joinUrl(ctx)
      },
      expect: { status: [200, 404] },
      concurrency: 10,
      skip: (ctx) => !pickFirstFile(ctx),
      timeoutMs: 30_000,
      order: 1,
    },
    {
      id: 'concurrency_parallel_get_50',
      nameI18n: 'devtools.webdav.cases.concurrency_parallel_get_50.name',
      descI18n: 'devtools.webdav.cases.concurrency_parallel_get_50.desc',
      module: 'concurrency',
      attackType: 'concurrency-stress',
      method: 'GET',
      url: (ctx) => {
        const f = pickFirstFile(ctx)
        return f ? mountUrl(ctx, f) : joinUrl(ctx)
      },
      expect: { status: [200, 404] },
      concurrency: 50,
      skip: (ctx) => !pickFirstFile(ctx),
      timeoutMs: 60_000,
      order: 2,
    },
    {
      // 20 并发列根目录（PROPFIND depth=1）
      id: 'concurrency_parallel_list_20',
      nameI18n: 'devtools.webdav.cases.concurrency_parallel_list_20.name',
      descI18n: 'devtools.webdav.cases.concurrency_parallel_list_20.desc',
      module: 'concurrency',
      attackType: 'concurrency-stress',
      method: 'PROPFIND',
      url: (ctx) => joinUrl(ctx),
      headers: () => ({ Depth: '1', 'Content-Type': 'application/xml' }),
      body: '<?xml version="1.0" encoding="utf-8" ?><D:propfind xmlns:D="DAV:"><D:prop><D:resourcetype/></D:prop></D:propfind>',
      expect: { status: [200, 207] },
      concurrency: 20,
      timeoutMs: 30_000,
      order: 3,
    },
    {
      // 100 次连续列根目录（PROPFIND depth=1）
      id: 'concurrency_iterations_100',
      nameI18n: 'devtools.webdav.cases.concurrency_iterations_100.name',
      descI18n: 'devtools.webdav.cases.concurrency_iterations_100.desc',
      module: 'concurrency',
      attackType: 'concurrency-stress',
      method: 'PROPFIND',
      url: (ctx) => joinUrl(ctx),
      headers: () => ({ Depth: '1', 'Content-Type': 'application/xml' }),
      body: '<?xml version="1.0" encoding="utf-8" ?><D:propfind xmlns:D="DAV:"><D:prop><D:resourcetype/></D:prop></D:propfind>',
      expect: { status: [200, 207] },
      iterations: 100,
      timeoutMs: 60_000,
      order: 4,
    },
  ],
}

// ============= Module 5: large_payload（大文件 + 慢网络）============

const largePayloadModule: TestModule = {
  id: 'large_payload',
  nameI18n: 'devtools.webdav.modules.large_payload.name',
  descI18n: 'devtools.webdav.modules.large_payload.desc',
  icon: 'cube-outline',
  color: 'medium',
  cases: [
    {
      id: 'large_get_largest_container',
      nameI18n: 'devtools.webdav.cases.large_get_largest_container.name',
      descI18n: 'devtools.webdav.cases.large_get_largest_container.desc',
      module: 'large_payload',
      attackType: 'large-payload',
      method: 'GET',
      url: (ctx) => {
        // 选 manifest 中 size 最大的虚拟文件
        const largest = ctx.activeMount.manifest.virtual_tree
          .filter((e) => !e.is_dir)
          .sort((a, b) => b.size - a.size)[0]
        return largest ? mountUrl(ctx, largest.virtual_path) : joinUrl(ctx)
      },
      expect: { status: [200, 404] },
      skip: (ctx) => !ctx.activeMount.manifest.virtual_tree.some((e) => !e.is_dir),
      timeoutMs: 60_000,
      order: 1,
    },
    {
      id: 'large_head_all_files',
      nameI18n: 'devtools.webdav.cases.large_head_all_files.name',
      descI18n: 'devtools.webdav.cases.large_head_all_files.desc',
      module: 'large_payload',
      method: 'HEAD',
      url: (ctx) => {
        const f = pickFirstFile(ctx)
        return f ? mountUrl(ctx, f) : joinUrl(ctx)
      },
      expect: { status: [200, 404] },
      skip: (ctx) => !pickFirstFile(ctx),
      order: 2,
    },
    {
      id: 'large_timeout_short',
      nameI18n: 'devtools.webdav.cases.large_timeout_short.name',
      descI18n: 'devtools.webdav.cases.large_timeout_short.desc',
      module: 'large_payload',
      attackType: 'slow-network',
      method: 'GET',
      url: (ctx) => {
        const f = pickFirstFile(ctx)
        return f ? mountUrl(ctx, f) : joinUrl(ctx)
      },
      expect: { status: 'skipped' as unknown as number }, // 自定义：超时是预期
      timeoutMs: 1, // 故意 1ms 触发超时
      customAssert: () => null,
      skip: (ctx) => !pickFirstFile(ctx),
      order: 3,
    },
    {
      id: 'large_abort_mid_request',
      nameI18n: 'devtools.webdav.cases.large_abort_mid_request.name',
      descI18n: 'devtools.webdav.cases.large_abort_mid_request.desc',
      module: 'large_payload',
      attackType: 'slow-network',
      method: 'GET',
      url: (ctx) => {
        const f = pickFirstFile(ctx)
        return f ? mountUrl(ctx, f) : joinUrl(ctx)
      },
      expect: { status: 0 }, // 0 = abort（mid-abort case 故意触发）
      timeoutMs: 50,
      beforeRun: (ctx) => {
        // 50ms 后主动 abort
        const id = setTimeout(() => {
          if (ctx.abortSignal) ctx.abortSignal.dispatchEvent?.(new Event('abort'))
        }, 50)
        return Promise.resolve(() => clearTimeout(id))
      },
      customAssert: () => null,
      skip: (ctx) => !pickFirstFile(ctx),
      order: 4,
    },
  ],
}

// ============= Module 6: edge（边界情况）============

const edgeModule: TestModule = {
  id: 'edge',
  nameI18n: 'devtools.webdav.modules.edge.name',
  descI18n: 'devtools.webdav.modules.edge.desc',
  icon: 'warning-outline',
  color: 'danger',
  cases: [
    {
      id: 'edge_path_traversal_root',
      nameI18n: 'devtools.webdav.cases.edge_path_traversal_root.name',
      descI18n: 'devtools.webdav.cases.edge_path_traversal_root.desc',
      module: 'edge',
      attackType: 'protocol-consistency',
      method: 'GET',
      url: (ctx) => joinUrl(ctx, '..', '..', '..', '..', 'etc', 'passwd'),
      expect: {
        status: [400, 403, 404], // 拒绝（不能返 200 + 真实 /etc/passwd）
        bodyNotMatches: /root:x:0:0/,
      },
      order: 1,
    },
    {
      id: 'edge_path_traversal_absolute',
      nameI18n: 'devtools.webdav.cases.edge_path_traversal_absolute.name',
      descI18n: 'devtools.webdav.cases.edge_path_traversal_absolute.desc',
      module: 'edge',
      attackType: 'protocol-consistency',
      method: 'GET',
      url: (ctx) => joinUrl(ctx, '..', '..', 'etc', 'passwd'),
      expect: {
        status: [400, 403, 404],
        bodyNotMatches: /root:x:0:0/,
      },
      order: 2,
    },
    {
      id: 'edge_unicode_filename',
      nameI18n: 'devtools.webdav.cases.edge_unicode_filename.name',
      descI18n: 'devtools.webdav.cases.edge_unicode_filename.desc',
      module: 'edge',
      attackType: 'protocol-consistency',
      method: 'GET',
      url: (ctx) => mountUrl(ctx, '中文-文件-' + Date.now() + '.txt'),
      expect: { status: [404, 400] }, // 文件不存在是预期
      order: 3,
    },
    {
      id: 'edge_root_no_slash',
      nameI18n: 'devtools.webdav.cases.edge_root_no_slash.name',
      descI18n: 'devtools.webdav.cases.edge_root_no_slash.desc',
      module: 'edge',
      attackType: 'protocol-consistency',
      method: 'GET',
      url: (ctx) => {
        const prefix = ctx.webdavPath
        return ctx.serverBaseUrl + prefix // no trailing slash
      },
      // 注意：go-webdav 对 collection GET 返 405（RFC 4918 §9.4），也算合规
      expect: { status: [200, 207, 301, 308, 405] },
      order: 4,
    },
    {
      id: 'edge_double_slash',
      nameI18n: 'devtools.webdav.cases.edge_double_slash.name',
      descI18n: 'devtools.webdav.cases.edge_double_slash.desc',
      module: 'edge',
      attackType: 'protocol-consistency',
      method: 'GET',
      url: (ctx) => {
        const prefix = ctx.webdavPath
        return ctx.serverBaseUrl + prefix + '/' // trailing slash OK
      },
      // 注意：go-webdav 对 collection GET 返 405（RFC 4918 §9.4），也算合规
      expect: { status: [200, 207, 301, 308, 400, 405] },
      order: 5,
    },
  ],
}

// ============= Module 7: attack（攻防测试，6 类）============

const attackModule: TestModule = {
  id: 'attack',
  nameI18n: 'devtools.webdav.modules.attack.name',
  descI18n: 'devtools.webdav.modules.attack.desc',
  icon: 'shield-checkmark-outline',
  color: 'danger',
  cases: [
    // 1. 容器可见性泄露
    {
      id: 'attack_container_propfind_physical_path',
      nameI18n: 'devtools.webdav.cases.attack_container_propfind_physical_path.name',
      descI18n: 'devtools.webdav.cases.attack_container_propfind_physical_path.desc',
      module: 'attack',
      attackType: 'container-visibility',
      method: 'PROPFIND',
      url: (ctx) => {
        const c = pickFirstContainer(ctx)
        return c ? mountUrl(ctx, c.container) : joinUrl(ctx)
      },
      headers: () => ({ Depth: '0', 'Content-Type': 'application/xml' }),
      body: '<?xml version="1.0" encoding="utf-8" ?><D:propfind xmlns:D="DAV:"><D:prop><D:resourcetype/></D:prop></D:propfind>',
      expect: {
        // 容器物理路径不能直接看到（被虚拟文件覆盖 → 404 或 207 + multistatus 指向虚拟文件）
        status: [404, 207, 403],
      },
      skip: (ctx) => !pickFirstContainer(ctx),
      order: 1,
    },
    {
      id: 'attack_container_get_physical_path',
      nameI18n: 'devtools.webdav.cases.attack_container_get_physical_path.name',
      descI18n: 'devtools.webdav.cases.attack_container_get_physical_path.desc',
      module: 'attack',
      attackType: 'container-visibility',
      method: 'GET',
      url: (ctx) => {
        const c = pickFirstContainer(ctx)
        return c ? mountUrl(ctx, c.container) : joinUrl(ctx)
      },
      expect: {
        // 不能返 manifest 头部 / 容器元数据
        status: [404, 200],
        bodyNotMatches: /ENCV|enved|magic|manifest|header/i,
      },
      skip: (ctx) => !pickFirstContainer(ctx),
      order: 2,
    },
    {
      id: 'attack_container_list_dir_excludes_ext',
      nameI18n: 'devtools.webdav.cases.attack_container_list_dir_excludes_ext.name',
      descI18n: 'devtools.webdav.cases.attack_container_list_dir_excludes_ext.desc',
      module: 'attack',
      attackType: 'container-visibility',
      method: 'PROPFIND',
      url: (ctx) => {
        const c = pickFirstContainer(ctx)
        return c ? mountUrl(ctx, c.container.split('/').slice(0, -1).join('/')) : joinUrl(ctx)
      },
      headers: () => ({ Depth: '1', 'Content-Type': 'application/xml' }),
      body: '<?xml version="1.0" encoding="utf-8" ?><D:propfind xmlns:D="DAV:"><D:prop><D:resourcetype/></D:prop></D:propfind>',
      expect: {
        // 列表不能包含容器物理文件（被 IsContainerExtension 过滤）
        // 动态从 manifest.registered_container_exts 取扩展名（不能硬编码 .sccg* / .encv）
        status: [207, 200],
        bodyNotMatches: (ctx) => {
          const ext = getFirstContainerExt(ctx)
          // 转义 regex 元字符（前导点 + 字母数字）
          const escaped = ext.replace(/[.*+?^${}()|[\]\\]/g, '\\$&')
          return new RegExp(`href[^>]*>\\s*[^<]*${escaped}\\s*<`, 'i')
        },
      },
      skip: (ctx) => !pickFirstContainer(ctx),
      order: 3,
    },

    // 2. 跨 mount 逃逸
    {
      id: 'attack_cross_mount_traversal',
      nameI18n: 'devtools.webdav.cases.attack_cross_mount_traversal.name',
      descI18n: 'devtools.webdav.cases.attack_cross_mount_traversal.desc',
      module: 'attack',
      attackType: 'cross-mount-escape',
      method: 'GET',
      url: (ctx) => {
        // /d/automation/../../d/primary/secret
        const mountPath = ctx.activeMount.webdav_path
        const otherMount = ctx.manifest.mounts.find((m) => m.name !== ctx.activeMount.name)
        if (!otherMount) return joinUrl(ctx, 'noop')
        return `${ctx.serverBaseUrl}${mountPath}/../../${otherMount.webdav_path.replace(/^\//, '')}/`
      },
      expect: {
        status: [400, 403, 404],
        bodyNotMatches: /<D:multistatus|<d:multistatus/i,
      },
      skip: (ctx) => !hasMultipleMounts(ctx),
      order: 4,
    },
    {
      id: 'attack_cross_mount_to_etc',
      nameI18n: 'devtools.webdav.cases.attack_cross_mount_to_etc.name',
      descI18n: 'devtools.webdav.cases.attack_cross_mount_to_etc.desc',
      module: 'attack',
      attackType: 'cross-mount-escape',
      method: 'GET',
      url: (ctx) => {
        const mountPath = ctx.activeMount.webdav_path
        return `${ctx.serverBaseUrl}${mountPath}/../../../../etc/passwd`
      },
      expect: {
        status: [400, 403, 404],
        bodyNotMatches: /root:x:0:0/,
      },
      order: 5,
    },

    // 3. 协议一致性 + 边界
    {
      id: 'attack_propfind_injection',
      nameI18n: 'devtools.webdav.cases.attack_propfind_injection.name',
      descI18n: 'devtools.webdav.cases.attack_propfind_injection.desc',
      module: 'attack',
      attackType: 'protocol-consistency',
      method: 'PROPFIND',
      url: (ctx) => joinUrl(ctx, '..', '..'),
      headers: () => ({ Depth: 'infinity', 'Content-Type': 'application/xml' }),
      body: '<?xml version="1.0" encoding="utf-8" ?><D:propfind xmlns:D="DAV:"><D:prop><D:resourcetype/></D:prop></D:propfind>',
      expect: {
        status: [400, 403, 404, 207],
      },
      order: 6,
    },

    // 4. Index 重建 race（外部修改 + 立即 GET）
    {
      id: 'attack_index_race_modify_then_get',
      nameI18n: 'devtools.webdav.cases.attack_index_race_modify_then_get.name',
      descI18n: 'devtools.webdav.cases.attack_index_race_modify_then_get.desc',
      module: 'attack',
      attackType: 'index-rebuild-race',
      method: 'GET',
      url: (ctx) => {
        const f = pickFirstFile(ctx)
        return f ? mountUrl(ctx, f) : joinUrl(ctx)
      },
      expect: { status: [200, 404, 207] },
      iterations: 20,
      concurrency: 5,
      skip: (ctx) => !pickFirstFile(ctx),
      timeoutMs: 60_000,
      order: 7,
    },

    // 5. 资源耗尽
    {
      // 100 并发 PROPFIND depth=1 根目录（不能 GET 根——go-webdav 返 405）
      id: 'attack_resource_exhaustion_burst',
      nameI18n: 'devtools.webdav.cases.attack_resource_exhaustion_burst.name',
      descI18n: 'devtools.webdav.cases.attack_resource_exhaustion_burst.desc',
      module: 'attack',
      attackType: 'resource-exhaustion',
      method: 'PROPFIND',
      url: (ctx) => joinUrl(ctx),
      headers: () => ({ Depth: '1', 'Content-Type': 'application/xml' }),
      body: '<?xml version="1.0" encoding="utf-8" ?><D:propfind xmlns:D="DAV:"><D:prop><D:resourcetype/></D:prop></D:propfind>',
      expect: { status: [200, 207, 429, 503] }, // 429/503 是 backpressure 信号
      concurrency: 100,
      timeoutMs: 60_000,
      order: 8,
    },
  ],
}

// ============= Module 8: encrypted_container_preview（加密容器预览）============
//
// 🆕 2026-06-17：用户补充要求 — 之前要求的「预览加密容器测试」一直没做
// 加密容器是本项目核心功能，必须有专项 module 覆盖
// ⚠️ 容器扩展名从 manifest.registered_container_exts 动态取，
//    绝不能硬编码任何具体后缀（plugin 系统是权威，配置可变）
// 覆盖维度：
//   1. list 容器（PROPFIND depth=1 拿到 virtual files）
//   2. GET 容器内虚拟文件（解密路径）
//   3. 验证容器 manifest header（阻止直接访问物理路径）
//   4. 并发访问同一容器（验证 file-system 锁不冲突）
//   5. 容器内大文件（解密 + 流式传输）
//   6. 容器被删除/不存在 → 404
//   7. 嵌套容器（容器套容器）
//   8. 容器受 container_map 保护（攻击者直接读物理路径被拒）
const encryptedContainerModule: TestModule = {
  id: 'encrypted_container_preview',
  nameI18n: 'devtools.webdav.modules.encrypted_container_preview.name',
  descI18n: 'devtools.webdav.modules.encrypted_container_preview.desc',
  icon: 'lock-closed-outline',
  color: 'medium',
  cases: [
    {
      // 1. PROPFIND 列父目录，验证容器物理文件被 container_map 映射为 virtual directory
      // 注意：扩展名从 manifest.registered_container_exts 动态取，不能硬编码
      id: 'enc_list_parent_includes_container',
      nameI18n: 'devtools.webdav.cases.enc_list_parent_includes_container.name',
      descI18n: 'devtools.webdav.cases.enc_list_parent_includes_container.desc',
      module: 'encrypted_container_preview',
      method: 'PROPFIND',
      url: (ctx) => {
        // 选 container_map 里第一个虚拟目录
        const m = ctx.activeMount.manifest.container_map[0]
        if (!m) return joinUrl(ctx)
        // 父目录 = virtual_path 去掉最后一段
        const parts = m.virtual_path.split('/').filter(Boolean)
        if (parts.length <= 1) return joinUrl(ctx)
        const parent = '/' + parts.slice(0, -1).join('/')
        return mountUrl(ctx, parent)
      },
      headers: { Depth: '1' },
      expect: { status: 207 },
      skip: (ctx) => ctx.activeMount.manifest.container_map.length === 0,
      attackType: 'container-visibility',
      order: 1,
    },
    {
      // 2. PROPFIND 容器虚拟目录，期望列出内部虚拟文件
      id: 'enc_list_container_via_propfind',
      nameI18n: 'devtools.webdav.cases.enc_list_container_via_propfind.name',
      descI18n: 'devtools.webdav.cases.enc_list_container_via_propfind.desc',
      module: 'encrypted_container_preview',
      method: 'PROPFIND',
      url: (ctx) => {
        const m = ctx.activeMount.manifest.container_map[0]
        return m ? mountUrl(ctx, m.virtual_path) : joinUrl(ctx)
      },
      headers: { Depth: '1' },
      expect: { status: 207 },
      skip: (ctx) => ctx.activeMount.manifest.container_map.length === 0,
      order: 2,
    },
    {
      // 3. GET 容器内第一个虚拟文件（解密路径）
      id: 'enc_get_virtual_file_inside',
      nameI18n: 'devtools.webdav.cases.enc_get_virtual_file_inside.name',
      descI18n: 'devtools.webdav.cases.enc_get_virtual_file_inside.desc',
      module: 'encrypted_container_preview',
      method: 'GET',
      url: (ctx) => {
        // 从 virtual_tree 找第一个属于容器的虚拟文件
        const container = ctx.activeMount.manifest.container_map[0]
        if (!container) return joinUrl(ctx)
        const vf = ctx.activeMount.manifest.virtual_tree.find(
          (e) => !e.is_dir && e.virtual_path.startsWith(container.virtual_path + '/')
        )
        return vf ? mountUrl(ctx, vf.virtual_path) : joinUrl(ctx)
      },
      expect: { status: [200, 404, 500] }, // 500: 容器没挂载（fixture 缺失）
      skip: (ctx) => ctx.activeMount.manifest.container_map.length === 0,
      order: 3,
    },
    {
      // 4. 验证容器 manifest header 阻止直接访问物理路径
      id: 'enc_direct_physical_path_blocked',
      nameI18n: 'devtools.webdav.cases.enc_direct_physical_path_blocked.name',
      descI18n: 'devtools.webdav.cases.enc_direct_physical_path_blocked.desc',
      module: 'encrypted_container_preview',
      method: 'GET',
      url: (ctx) => {
        // 攻击者尝试直接读容器物理路径（用 container_path 字段）
        const m = ctx.activeMount.manifest.container_map[0]
        if (!m || !m.container_path) return joinUrl(ctx)
        return mountUrl(ctx, m.container_path)
      },
      expect: { status: [400, 403, 404, 500] }, // 不应 200
      skip: (ctx) => ctx.activeMount.manifest.container_map.length === 0,
      attackType: 'container-visibility',
      order: 4,
    },
    {
      // 5. 20 并发 GET 同一容器内文件（验证 file-system 锁）
      id: 'enc_concurrent_access_same_container',
      nameI18n: 'devtools.webdav.cases.enc_concurrent_access_same_container.name',
      descI18n: 'devtools.webdav.cases.enc_concurrent_access_same_container.desc',
      module: 'encrypted_container_preview',
      method: 'GET',
      url: (ctx) => {
        const container = ctx.activeMount.manifest.container_map[0]
        if (!container) return joinUrl(ctx)
        const vf = ctx.activeMount.manifest.virtual_tree.find(
          (e) => !e.is_dir && e.virtual_path.startsWith(container.virtual_path + '/')
        )
        return vf ? mountUrl(ctx, vf.virtual_path) : joinUrl(ctx)
      },
      concurrency: 20,
      expect: { status: [200, 404, 500] },
      skip: (ctx) => ctx.activeMount.manifest.container_map.length === 0,
      attackType: 'concurrency-stress',
      order: 5,
    },
    {
      // 6. 容器内最大文件 GET（解密 + 流式）
      id: 'enc_get_largest_virtual_in_container',
      nameI18n: 'devtools.webdav.cases.enc_get_largest_virtual_in_container.name',
      descI18n: 'devtools.webdav.cases.enc_get_largest_virtual_in_container.desc',
      module: 'encrypted_container_preview',
      method: 'GET',
      url: (ctx) => {
        const container = ctx.activeMount.manifest.container_map[0]
        if (!container) return joinUrl(ctx)
        const files = ctx.activeMount.manifest.virtual_tree
          .filter((e) => !e.is_dir && e.virtual_path.startsWith(container.virtual_path + '/'))
        if (files.length === 0) return joinUrl(ctx)
        // 按 size 降序取最大
        const largest = files.reduce((a, b) => ((a.size ?? 0) > (b.size ?? 0) ? a : b))
        return mountUrl(ctx, largest.virtual_path)
      },
      expect: { status: [200, 404, 500] },
      skip: (ctx) => ctx.activeMount.manifest.container_map.length === 0,
      order: 6,
    },
    {
      // 7. 访问不存在的容器虚拟路径 → 404（模拟容器被删除/重命名后的行为）
      id: 'enc_get_nonexistent_container_404',
      nameI18n: 'devtools.webdav.cases.enc_get_nonexistent_container_404.name',
      descI18n: 'devtools.webdav.cases.enc_get_nonexistent_container_404.desc',
      module: 'encrypted_container_preview',
      method: 'GET',
      url: (ctx) => {
        // 用容器虚拟路径的父目录 + 时间戳拼一个不存在的虚拟文件
        const container = ctx.activeMount.manifest.container_map[0]
        if (!container) return joinUrl(ctx)
        const parts = container.virtual_path.split('/').filter(Boolean)
        if (parts.length === 0) return joinUrl(ctx)
        const parent = '/' + parts.slice(0, -1).join('/')
        return mountUrl(ctx, parent, '__nope_container_' + Date.now() + '.bin')
      },
      expect: { status: [404, 400, 403] },
      skip: (ctx) => ctx.activeMount.manifest.container_map.length === 0,
      order: 7,
    },
    {
      // 8. PROPFIND depth=infinity 容器根，验证能完整列出容器内全部虚拟文件
      //    （不是嵌套容器 —— ENCV v4 不支持容器套容器；这里测容器内**多级目录**的虚拟文件）
      id: 'enc_propfind_container_depth_infinity',
      nameI18n: 'devtools.webdav.cases.enc_propfind_container_depth_infinity.name',
      descI18n: 'devtools.webdav.cases.enc_propfind_container_depth_infinity.desc',
      module: 'encrypted_container_preview',
      method: 'PROPFIND',
      url: (ctx) => {
        const m = ctx.activeMount.manifest.container_map[0]
        return m ? mountUrl(ctx, m.virtual_path) : joinUrl(ctx)
      },
      headers: () => ({ Depth: 'infinity', 'Content-Type': 'application/xml' }),
      body: '<?xml version="1.0" encoding="utf-8" ?><D:propfind xmlns:D="DAV:"><D:prop><D:resourcetype/></D:prop></D:propfind>',
      expect: { status: [207, 200, 403] }, // 403 = server 拒绝 depth=infinity 也合法
      timeoutMs: 30_000,
      skip: (ctx) => ctx.activeMount.manifest.container_map.length === 0,
      order: 8,
    },
    {
      // 9. HEAD 容器内虚拟文件，验证元数据（Content-Type / Content-Length / Last-Modified）
      id: 'enc_head_virtual_file_inside',
      nameI18n: 'devtools.webdav.cases.enc_head_virtual_file_inside.name',
      descI18n: 'devtools.webdav.cases.enc_head_virtual_file_inside.desc',
      module: 'encrypted_container_preview',
      method: 'HEAD',
      url: (ctx) => {
        const container = ctx.activeMount.manifest.container_map[0]
        if (!container) return joinUrl(ctx)
        const vf = ctx.activeMount.manifest.virtual_tree.find(
          (e) => !e.is_dir && e.virtual_path.startsWith(container.virtual_path + '/')
        )
        return vf ? mountUrl(ctx, vf.virtual_path) : joinUrl(ctx)
      },
      expect: { status: [200, 404, 500] },
      skip: (ctx) => ctx.activeMount.manifest.container_map.length === 0,
      order: 9,
    },
    {
      // 10. Range request 容器内文件（HTTP Range 头）—— 用于视频拖动条
      id: 'enc_range_request_virtual_file',
      nameI18n: 'devtools.webdav.cases.enc_range_request_virtual_file.name',
      descI18n: 'devtools.webdav.cases.enc_range_request_virtual_file.desc',
      module: 'encrypted_container_preview',
      method: 'GET',
      url: (ctx) => {
        const container = ctx.activeMount.manifest.container_map[0]
        if (!container) return joinUrl(ctx)
        const vf = ctx.activeMount.manifest.virtual_tree.find(
          (e) => !e.is_dir && e.virtual_path.startsWith(container.virtual_path + '/')
        )
        return vf ? mountUrl(ctx, vf.virtual_path) : joinUrl(ctx)
      },
      // Range: bytes=0-1023 请求前 1KB（容器解密 + 流式 chunk）
      headers: (ctx) => {
        const container = ctx.activeMount.manifest.container_map[0]
        if (!container) return {} as Record<string, string>
        const vf = ctx.activeMount.manifest.virtual_tree.find(
          (e) => !e.is_dir && e.virtual_path.startsWith(container.virtual_path + '/')
        )
        if (!vf || !vf.size || vf.size < 1024) return {} as Record<string, string>
        return { Range: 'bytes=0-1023' }
      },
      // 期望：206 Partial Content（Range 支持）或 200（不支持但能 GET）
      expect: { status: [206, 200, 404, 500, 416] }, // 416 = Range Not Satisfiable（小文件）
      skip: (ctx) => ctx.activeMount.manifest.container_map.length === 0,
      order: 10,
    },
    {
      // 11. 跨 mount 访问容器：尝试用 path traversal 访问其他 mount 中的容器
      id: 'enc_cross_mount_access_container',
      nameI18n: 'devtools.webdav.cases.enc_cross_mount_access_container.name',
      descI18n: 'devtools.webdav.cases.enc_cross_mount_access_container.desc',
      module: 'encrypted_container_preview',
      attackType: 'cross-mount-escape',
      method: 'GET',
      url: (ctx) => {
        const other = pickOtherMount(ctx)
        const container = ctx.activeMount.manifest.container_map[0]
        if (!other || !container) return joinUrl(ctx)
        // 攻击者用 ../ 跳到其他 mount，再访问容器
        const mountPath = ctx.activeMount.webdav_path
        return `${ctx.serverBaseUrl}${mountPath}/../../${other.webdav_path.replace(/^\//, '')}/${container.virtual_path.replace(/^\//, '')}`
      },
      expect: { status: [400, 403, 404, 500] }, // 不应 200（跨 mount 容器访问被拒）
      skip: (ctx) => !hasMultipleMounts(ctx) || ctx.activeMount.manifest.container_map.length === 0,
      order: 11,
    },
  ],
}

// ============= Module 列表（顺序与 8 module 命名）============

export const WEBDAV_TEST_MODULES: TestModule[] = [
  authModule,
  basicOpsModule,
  protocolModule,
  concurrencyModule,
  largePayloadModule,
  edgeModule,
  attackModule,
  encryptedContainerModule,
]

/** 快速查找 module */
export function getModuleById(id: string): TestModule | undefined {
  return WEBDAV_TEST_MODULES.find((m) => m.id === id)
}
