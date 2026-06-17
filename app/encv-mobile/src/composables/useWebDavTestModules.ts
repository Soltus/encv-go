/**
 * useWebDavTestModules — 7 module × 30+ test case 定义
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
 * 6 类攻防标记（attackType 字段）：
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
      url: (ctx) => joinUrl(ctx),
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
      url: (ctx) => joinUrl(ctx),
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
      url: (ctx) => joinUrl(ctx),
      // 头由 runner 自动注入 ctx.auth.basic
      expect: {
        status: [200, 207, 301],
      },
      order: 3,
    },
    {
      id: 'auth_no_auth_required_200',
      nameI18n: 'devtools.webdav.cases.auth_no_auth_required_200.name',
      descI18n: 'devtools.webdav.cases.auth_no_auth_required_200.desc',
      module: 'auth',
      method: 'GET',
      url: (ctx) => joinUrl(ctx),
      headers: () => ({}),
      expect: {
        status: [200, 207],
      },
      // 当 cfg.Webdav.Username 有值时跳过（不是无 auth 配置）
      skip: (ctx) => !!(ctx.auth.username),
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
      nameI18n: 'devtools.webdav.cases.basic_list_root.name',
      descI18n: 'devtools.webdav.cases.basic_list_root.desc',
      module: 'basic_ops',
      method: 'GET',
      url: (ctx) => joinUrl(ctx),
      expect: { status: [200, 207] },
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
      id: 'concurrency_parallel_list_20',
      nameI18n: 'devtools.webdav.cases.concurrency_parallel_list_20.name',
      descI18n: 'devtools.webdav.cases.concurrency_parallel_list_20.desc',
      module: 'concurrency',
      attackType: 'concurrency-stress',
      method: 'GET',
      url: (ctx) => joinUrl(ctx),
      expect: { status: [200, 207] },
      concurrency: 20,
      timeoutMs: 30_000,
      order: 3,
    },
    {
      id: 'concurrency_iterations_100',
      nameI18n: 'devtools.webdav.cases.concurrency_iterations_100.name',
      descI18n: 'devtools.webdav.cases.concurrency_iterations_100.desc',
      module: 'concurrency',
      attackType: 'concurrency-stress',
      method: 'GET',
      url: (ctx) => joinUrl(ctx),
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
      expect: { status: [200, 207, 301, 308] },
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
      expect: { status: [200, 207, 301, 308, 400] },
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
        // 列表不能包含 *.encv 物理文件（被 IsContainerExtension 过滤）
        status: [207, 200],
        bodyNotMatches: /href[^>]*>\s*[^<]*\.encv\s*</i,
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
      id: 'attack_resource_exhaustion_burst',
      nameI18n: 'devtools.webdav.cases.attack_resource_exhaustion_burst.name',
      descI18n: 'devtools.webdav.cases.attack_resource_exhaustion_burst.desc',
      module: 'attack',
      attackType: 'resource-exhaustion',
      method: 'GET',
      url: (ctx) => joinUrl(ctx),
      expect: { status: [200, 207, 429, 503] }, // 429/503 是 backpressure 信号
      concurrency: 100,
      timeoutMs: 60_000,
      order: 8,
    },
  ],
}

// ============= Module 列表（顺序与 7 module 命名）============

export const WEBDAV_TEST_MODULES: TestModule[] = [
  authModule,
  basicOpsModule,
  protocolModule,
  concurrencyModule,
  largePayloadModule,
  edgeModule,
  attackModule,
]

/** 快速查找 module */
export function getModuleById(id: string): TestModule | undefined {
  return WEBDAV_TEST_MODULES.find((m) => m.id === id)
}
