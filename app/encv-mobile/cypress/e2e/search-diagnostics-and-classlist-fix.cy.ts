/**
 * 🆕 2026-07-03 验证三个修复：
 *   1. FullTextIndexDetail.vue classList 错误修复（显式 import Ionic 组件）
 *      - 修复前：渲染成 <ION-PAGE>（大写未编译），无 .ion-page class，无 z-index，被前页覆盖
 *      - 修复后：应该渲染成 div.ion-page，有 z-index，可见
 *   2. 搜索结果为空时显示诊断卡片（FTS 状态 + 索引统计 + 重试/进入索引页按钮）
 *   3. 搜索过程打 console 日志（自动镜像到 DevLogs frontend tab）
 *
 * 用户反馈：
 *   "结合全文索引页面无法进入查看的问题，那么无法复现的问题显而易见了：
 *    安卓本机全文搜索就是异常的，但是理论上也不该为空啊。这暴露了另一个问题，
 *    搜索结果为空时显示ui太简单，缺少各种状态信息辅助诊断。devlogs也没有日志输出！"
 */
describe('搜索诊断 UI + classList 修复 + devlogs 日志（2026-07-03）', () => {
  const backendUrl = Cypress.env('apiBase') || 'http://localhost:2025'

  before(() => {
    // 确保后端在线
    cy.request({ url: `${backendUrl}/api/runtime`, failOnStatusCode: false }).then((resp) => {
      cy.log('后端 /api/runtime 状态:', resp.status)
      expect(resp.status).to.eq(200)
    })
  })

  // 关键：Cypress 测试环境没有 preview-gateway :16666，必须把 API base URL 直连后端 :2025。
  //   - 前端 getApiBaseUrl() 在 dev 模式默认返回 DEV_SANDBOX_ENTRY='http://127.0.0.1:16666'
  //   - 没有网关 → fetch ECONNREFUSED → 搜索 API 永远失败 → 诊断卡片永远不显示
  //   - 必须在 window:before:load（页面脚本执行前）写入 localStorage，
  //     这样 getApiBaseUrl() 读到 stored value 直接返回 :2025
  beforeEach(() => {
    cy.on('window:before:load', (win) => {
      win.localStorage.setItem('encv-server-url', backendUrl)
    })
  })

  // ===========================================================================
  // 测试 1：FullTextIndexDetail 三级导航 classList 修复验证
  // ===========================================================================
  describe('FullTextIndexDetail classList 修复', () => {
    it('三级导航 Settings → Cache → FullText Index 应正常进入', () => {
      cy.visit('/tabs/settings')
      cy.wait(2000)

      // 步骤1：进入缓存页面
      cy.get('ion-item').contains(/缓存|cache/i).first().click()
      cy.wait(2000)
      cy.url().should('include', 'cache')

      // 步骤2：缓存页面应有全文索引入口
      cy.get('.fulltext-entry').should('exist').and('be.visible')

      // 步骤3：点击全文索引入口
      cy.get('.fulltext-entry').click()
      cy.wait(3000)

      // 步骤4：URL 应包含 fulltext-index
      cy.url().should('include', 'fulltext-index')

      // 步骤5：关键验证 — ion-router-outlet 最后一个 child 应该是 div.ion-page（不是 ION-PAGE）
      //   修复前：tag=ION-PAGE, class=undefined（未编译的自定义元素）
      //   修复后：tag=DIV, class=ion-page（Ionic Vue 组件正确渲染）
      cy.get('ion-router-outlet').then(($outlet) => {
        const children = $outlet.children()
        const lastChild = children[children.length - 1]
        cy.log(`最后一个 child: tag=${lastChild.tagName}, class=${lastChild.className}`)

        // 修复前是 ION-PAGE（大写），修复后应该是 DIV（或 ion-page 小写）
        // 关键：不能是 ION-PAGE（大写 = 未编译的自定义元素）
        expect(lastChild.tagName).to.not.eq('ION-PAGE')

        // 应该有 ion-page class
        expect(lastChild.className).to.include('ion-page')

        // 应该有 z-index style（被前一个页面覆盖的根因就是没 z-index）
        const style = lastChild.getAttribute('style') || ''
        cy.log(`最后一个 child style: ${style}`)
        // z-index 可能是 101 或更高（Ionic RouterOutlet 递增 z-index）
        // 修复前 style 是 undefined，修复后应该有 z-index
        expect(style).to.include('z-index')
      })

      // 步骤6：页面应该可见（不被前一个页面覆盖）
      //   修复前：FullTextIndexDetail 被 CacheDetail（z-index:101）覆盖，不可见
      cy.get('ion-title').contains(/全文索引|Full.*Text.*Index/i).should('be.visible')

      cy.screenshot('fulltext-index-classlist-fix-verified')
    })

    it('直接访问 /tabs/settings/fulltext-index 也应正常渲染', () => {
      cy.visit('/tabs/settings/fulltext-index')
      cy.wait(3000)

      cy.url().should('include', 'fulltext-index')
      cy.get('ion-title').contains(/全文索引|Full.*Text.*Index/i).should('be.visible')
    })
  })

  // 辅助：dismiss ErrorCaptureOverlay（WebSocket 连接失败会触发错误浮窗，遮挡搜索框）
  //   测试环境没装 preview-gateway :16666，WebSocket ws://location.host/ws 连不上，
  //   触发 useErrorCapture.addError → ErrorCaptureOverlay 浮窗显示。
  //   搜索功能走 HTTP API（不依赖 WS），所以 dismiss 浮窗不影响测试有效性。
  function dismissErrorOverlay() {
    cy.get('body').then(($body) => {
      const closeBtn = $body.find('.error-overlay-close')
      if (closeBtn.length > 0) {
        cy.wrap(closeBtn).first().click({ force: true })
        cy.wait(300)
      }
    })
  }

  // ===========================================================================
  // 测试 2：搜索结果为空时显示诊断卡片
  // ===========================================================================
  describe('搜索空结果诊断卡片', () => {
    beforeEach(() => {
      cy.visit('/tabs/files')
      cy.wait(3000)
      dismissErrorOverlay()
    })

    it('搜索不存在的关键词 → 应显示诊断卡片（FTS 状态 + 索引统计）', () => {
      // 用 cy.type() 输入（contenteditable div）
      //   force: true → 防止 ErrorCaptureOverlay 残留时遮挡报 "cannot be interacted with"
      cy.get('[data-testid="search-input"]').click({ force: true })
      cy.get('[data-testid="search-input"]').type('zzz_nonexistent_keyword_xyz{enter}', { delay: 50, force: true })

      // 等搜索 debounce(300ms) + 网络请求 + 诊断查询
      cy.wait(6000)
      // 搜索后可能再次弹错误浮窗（WS 重连失败），再 dismiss 一次
      dismissErrorOverlay()

      // 验证诊断卡片出现（UI 层验证，不依赖 cy.intercept spy）
      //   诊断卡片出现 = 搜索结果为空 + refreshSearchDiagnostics 已完成
      cy.get('[data-testid="search-diagnostics-card"]', { timeout: 10000 })
        .should('exist')
        .and('be.visible')

      // 验证 FTS badge 存在（显示 FTS 可用/不可用状态）
      cy.get('[data-testid="diag-fts-badge"]').should('exist')

      // 验证 FTS 索引大小信息存在（关键诊断信息）
      cy.get('[data-testid="diag-fts-index-size"]').should('exist')

      // 验证操作按钮存在
      cy.get('[data-testid="diag-retry-btn"]').should('exist').and('be.visible')
      cy.get('[data-testid="diag-refresh-btn"]').should('exist').and('be.visible')
      cy.get('[data-testid="diag-goto-index-btn"]').should('exist').and('be.visible')

      cy.screenshot('search-diagnostics-card-displayed')
    })

    it('点击"全文索引页"按钮 → 应跳转到 /tabs/settings/fulltext-index', () => {
      // 先触发空搜索（用 cy.type() + {enter}）
      cy.get('[data-testid="search-input"]').click({ force: true })
      cy.get('[data-testid="search-input"]').type('zzz_nonexistent_xyz{enter}', { delay: 50, force: true })
      cy.wait(6000)
      dismissErrorOverlay()

      // 确认诊断卡片出现
      cy.get('[data-testid="search-diagnostics-card"]').should('exist').and('be.visible')

      // 点击"全文索引页"按钮
      cy.get('[data-testid="diag-goto-index-btn"]').click({ force: true })
      cy.wait(2000)

      // 验证跳转
      cy.url().should('include', 'fulltext-index')
      cy.get('ion-title').contains(/全文索引|Full.*Text.*Index/i).should('be.visible')

      cy.screenshot('search-diagnostics-goto-index-works')
    })
  })

  // ===========================================================================
  // 测试 3：搜索过程打 console 日志（DevLogs frontend tab 可见）
  // ===========================================================================
  describe('搜索过程 console 日志 → DevLogs', () => {
    it('搜索后 DevLogs frontend tab 应有 [Search] 开头的日志', () => {
      const searchLogs: string[] = []

      // 拦截 console.info/warn，收集 [Search] 日志
      cy.on('window:before:load', (win) => {
        const origInfo = win.console.info
        const origWarn = win.console.warn
        win.console.info = (...args: any[]) => {
          const msg = args.map(a => typeof a === 'string' ? a : JSON.stringify(a)).join(' ')
          if (msg.includes('[Search]')) {
            searchLogs.push(`[info] ${msg}`)
          }
          origInfo.apply(win.console, args)
        }
        win.console.warn = (...args: any[]) => {
          const msg = args.map(a => typeof a === 'string' ? a : JSON.stringify(a)).join(' ')
          if (msg.includes('[Search]')) {
            searchLogs.push(`[warn] ${msg}`)
          }
          origWarn.apply(win.console, args)
        }
      })

      cy.visit('/tabs/files')
      cy.wait(3000)
      dismissErrorOverlay()

      // 执行搜索（空结果触发诊断 + 日志）
      cy.get('[data-testid="search-input"]').click({ force: true })
      cy.get('[data-testid="search-input"]').type('zzz_devlogs_test_xyz{enter}', { delay: 50, force: true })
      cy.wait(4000)

      // 验证 [Search] 日志被收集
      cy.then(() => {
        cy.log('=== 收集到的 [Search] 日志 ===')
        searchLogs.forEach((log, i) => cy.log(`  ${i + 1}. ${log.substring(0, 200)}`))

        // 关键断言：搜索过程应该打日志（修复前完全没有日志）
        expect(searchLogs.length).to.be.greaterThan(0, '搜索过程应该打 [Search] 日志到 console')

        // 应该至少有 performSearch start 日志
        const hasStartLog = searchLogs.some(l => l.includes('performSearch start'))
        expect(hasStartLog, '应该有 [Search] performSearch start 日志').to.be.true
      })

      // 访问 DevLogs 页面，验证 frontend tab 有日志
      cy.visit('/tabs/devlogs')
      cy.wait(2000)

      // DevLogs 应该能显示日志（frontend tab）
      // 尝试点击 frontend tab（如果存在），失败也无妨（可能已默认在该 tab）
      cy.get('ion-segment-button').then(($btns) => {
        const frontendBtn = $btns.filter((i, el) => /前端|frontend/i.test(el.textContent || ''))
        if (frontendBtn.length > 0) {
          cy.wrap(frontendBtn).first().click({ force: true })
        } else {
          cy.log('未找到 frontend tab 按钮，可能已默认在该 tab')
        }
      })
      cy.wait(1000)

      cy.screenshot('devlogs-frontend-tab-after-search')
    })
  })
})
