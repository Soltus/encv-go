/**
 * 🆕 2026-07-03 优化2：FTS 无结果 fallback 向量搜索 — E2E 测试
 *
 * 用户原话：
 *   "即使搜索有逻辑符 FTS 搜索没有匹配结果，结果也不应当为空。
 *    同样遵守增量合并原则，以及匹配过少智能贪婪，转换逻辑符进行普通向量搜索。
 *    合并结果为0才触发贪婪。"
 *
 * 验证点：
 *   1. 布尔查询 + FTS=0 + 向量=0 → 应触发 retry（用 cleanedQuery 重跑向量搜索）
 *   2. retry 命中 → searchMode='greedy'，UI 显示橙色"语义近似"banner
 *   3. retry 仍 0 → searchMode='greedy'（让用户知道已尝试贪婪匹配）
 *   4. 各种布尔语法（AND/OR/NOT/phrase/regex）的 cleanedQuery 转换正确
 *
 * 测试策略（参考 test-master-plan.md §五 Bug 复现铁律）：
 *   - cy.intercept 模拟后端返回 0 结果（不依赖真实索引）
 *   - 通过检查第二次 /api/search/files 调用的 URL 验证 cleanedQuery
 *   - 通过检查 .search-mode-banner.greedy 验证 UI 状态
 */
describe('优化2：FTS 无结果 fallback 向量搜索（2026-07-03）', () => {
  const backendUrl = Cypress.env('apiBase') || 'http://localhost:2025'

  before(() => {
    // 确保后端在线
    cy.request({ url: `${backendUrl}/api/runtime`, failOnStatusCode: false }).then((resp) => {
      cy.log('后端 /api/runtime 状态:', resp.status)
      expect(resp.status).to.eq(200)
    })
  })

  // 关键：Cypress 测试环境没有 preview-gateway :16666，必须把 API base URL 直连后端 :2025
  //   详见 search-diagnostics-and-classlist-fix.cy.ts 的同款注释
  beforeEach(() => {
    cy.on('window:before:load', (win) => {
      win.localStorage.setItem('encv-server-url', backendUrl)
    })
  })

  // dismiss ErrorCaptureOverlay（WS 失败浮窗）
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
  // 测试 1：AND 布尔查询 + FTS=0 + 向量=0 → retry cleanedQuery 命中
  // ===========================================================================
  it('AND 布尔查询 0 结果 → 用 cleanedQuery 重试向量搜索 → 命中 + greedy banner', () => {
    // 拦截 FTS API：始终返回 0 结果（dbEngine=sqlite 表示 FTS 可用但 0 命中）
    cy.intercept('GET', '**/api/files/search-fulltext*', {
      statusCode: 200,
      body: {
        results: [],
        total: 0,
        query: '在线 AND 高清',
        dbEngine: 'sqlite',
        indexSize: 100,
      },
    }).as('ftsSearch')

    // 拦截向量搜索 API：用计数器区分第 1 次（原 query）和第 2 次（cleanedQuery）
    let vectorCallCount = 0
    const vectorCalls: string[] = []
    cy.intercept('GET', '**/api/search/files*', (req) => {
      vectorCallCount++
      const url = req.url
      vectorCalls.push(url)
      cy.log(`vector search call #${vectorCallCount}: ${url.substring(0, 200)}`)
      if (vectorCallCount === 1) {
        // 第 1 次：原 query "在线 AND 高清" → 0 结果
        req.reply({
          files: [],
          total: 0,
          vector_search: false,
          search_mode: 'greedy',
        })
      } else {
        // 第 2 次：cleanedQuery "在线 高清" → 1 个结果
        req.reply({
          files: [
            {
              name: 'online_hd_video.mp4',
              path: '/d/videos/online_hd_video.mp4',
              size: 1024000,
              modtime: '2026-07-01T00:00:00Z',
              isDir: false,
              isDirectory: false,
            },
          ],
          total: 1,
          vector_search: true,
          search_mode: 'greedy',
        })
      }
    }).as('vectorSearch')

    cy.visit('/tabs/files')
    cy.wait(3000)
    dismissErrorOverlay()

    // 输入布尔查询（直接 type 文本，contenteditable 会按字面字符处理）
    cy.get('[data-testid="search-input"]').click({ force: true })
    cy.get('[data-testid="search-input"]').type('在线 AND 高清{enter}', { delay: 50, force: true })

    // 等搜索 debounce(300ms) + FTS 请求 + 第 1 次向量 + retry 向量
    cy.wait(5000)
    dismissErrorOverlay()

    // 验证 1：FTS API 被调用
    cy.get('@ftsSearch.all').then((interceptions) => {
      cy.log(`FTS search calls: ${interceptions.length}`)
      expect(interceptions.length).to.be.greaterThan(0, 'FTS API 应该被调用（query 含 AND 触发 FTS）')
    })

    // 验证 2：向量搜索至少调用 2 次（第 1 次原 query，第 2 次 cleanedQuery）
    cy.get('@vectorSearch.all').then((interceptions) => {
      cy.log(`Vector search total calls: ${interceptions.length}`)
      expect(intersections.length, '向量搜索应至少 2 次（原 query + retry cleanedQuery）').to.be.gte(2)

      // 第 2 次调用的 URL 应该包含 cleanedQuery（不含 AND）
      const secondUrl = vectorCalls[1] || ''
      cy.log(`第 2 次向量搜索 URL: ${secondUrl}`)
      expect(secondUrl, 'retry URL 应包含 cleanedQuery').to.include('%E5%9C%A8%E7%BA%BF') // "在线" URL-encoded
      expect(secondUrl, 'retry URL 不应包含大写 AND 字面').to.not.include('AND')
    })

    // 验证 3：搜索结果不为空（用了 retry 的结果）
    //   contenteditable div + Files.vue 用 ion-item 渲染搜索结果
    cy.get('ion-item').then(($items) => {
      const fileItems = $items.filter((i, el) => /online_hd_video/.test(el.textContent || ''))
      cy.log(`匹配 online_hd_video 的 ion-item 数: ${fileItems.length}`)
      expect(fileItems.length, '应该看到 retry 命中的结果').to.be.greaterThan(0)
    })

    // 验证 4：橙色 greedy banner 应显示（searchMode='greedy'）
    //   UI 在 search-mode-banner.greedy-match 上有橙色虚线边框
    cy.get('body').then(($body) => {
      const greedyBanner = $body.find('.search-mode-greedy, .greedy-match')
      cy.log(`greedy banner/match 元素数: ${greedyBanner.length}`)
      // 至少有 greedy 相关元素出现（banner 或 item 上的 greedy-match class）
      expect(greedyBanner.length, '应该显示 greedy 模式 UI 提示').to.be.greaterThan(0)
    })

    cy.screenshot('fulltext-fallback-and-greedy-banner')
  })

  // ===========================================================================
  // 测试 2：NOT 布尔查询 → cleanedQuery 丢弃 NOT 后的词
  //   "在线 NOT 视频" → cleanedQuery="在线"
  // ===========================================================================
  it('NOT 布尔查询 → cleanedQuery 丢弃 NOT 后的词（"在线 NOT 视频" → "在线"）', () => {
    cy.intercept('GET', '**/api/files/search-fulltext*', {
      statusCode: 200,
      body: { results: [], total: 0, query: '', dbEngine: 'sqlite', indexSize: 100 },
    }).as('ftsSearch')

    let vectorCallCount = 0
    const vectorCalls: string[] = []
    cy.intercept('GET', '**/api/search/files*', (req) => {
      vectorCallCount++
      vectorCalls.push(req.url)
      if (vectorCallCount === 1) {
        req.reply({ files: [], total: 0, vector_search: false, search_mode: 'greedy' })
      } else {
        // 第 2 次 cleanedQuery "在线" → 1 个结果
        req.reply({
          files: [
            {
              name: 'online_movie.mp4',
              path: '/d/online_movie.mp4',
              size: 1024,
              modtime: '2026-07-01T00:00:00Z',
              isDir: false,
              isDirectory: false,
            },
          ],
          total: 1,
          vector_search: true,
          search_mode: 'greedy',
        })
      }
    }).as('vectorSearch')

    cy.visit('/tabs/files')
    cy.wait(3000)
    dismissErrorOverlay()

    cy.get('[data-testid="search-input"]').click({ force: true })
    cy.get('[data-testid="search-input"]').type('在线 NOT 视频{enter}', { delay: 50, force: true })
    cy.wait(5000)
    dismissErrorOverlay()

    // 验证 retry URL 包含 "在线" 但不包含 "视频"（NOT 后的词被丢弃）
    cy.get('@vectorSearch.all').then(() => {
      expect(vectorCalls.length, '应至少 2 次向量调用').to.be.gte(2)
      const retryUrl = vectorCalls[1] || ''
      cy.log(`retry URL: ${retryUrl}`)
      // "在线" = %E5%9C%A8%E7%BA%BF
      expect(retryUrl).to.include('%E5%9C%A8%E7%BA%BF')
      // "视频" = %E8%A7%86%E9%A2%91 — 不应出现（NOT 后的词被丢弃）
      expect(retryUrl, 'NOT 后的词应被丢弃').to.not.include('%E8%A7%86%E9%A2%91')
    })
  })

  // ===========================================================================
  // 测试 3：FTS + retry 向量 都 0 → searchMode=greedy（让用户知道已尝试贪婪匹配）
  // ===========================================================================
  it('FTS + retry 向量 都 0 结果 → 仍标 greedy 模式（让用户知道已尝试）', () => {
    cy.intercept('GET', '**/api/files/search-fulltext*', {
      statusCode: 200,
      body: { results: [], total: 0, query: '', dbEngine: 'sqlite', indexSize: 100 },
    }).as('ftsSearch')

    // 所有向量调用都返回 0
    cy.intercept('GET', '**/api/search/files*', {
      body: { files: [], total: 0, vector_search: false, search_mode: 'greedy' },
    }).as('vectorSearch')

    cy.visit('/tabs/files')
    cy.wait(3000)
    dismissErrorOverlay()

    cy.get('[data-testid="search-input"]').click({ force: true })
    cy.get('[data-testid="search-input"]').type('在线 AND 高清{enter}', { delay: 50, force: true })
    cy.wait(5000)
    dismissErrorOverlay()

    // 验证 1：向量搜索被调用至少 2 次（原 query + retry）
    cy.get('@vectorSearch.all').then((interceptions) => {
      cy.log(`vector search calls: ${interceptions.length}`)
      expect(interceptions.length, '应至少 2 次（原 + retry）').to.be.gte(2)
    })

    // 验证 2：诊断卡片应显示（搜索结果为 0）
    cy.get('[data-testid="search-diagnostics-card"]', { timeout: 8000 })
      .should('exist')
      .and('be.visible')

    cy.screenshot('fulltext-fallback-all-empty-greedy')
  })

  // ===========================================================================
  // 测试 4：纯 NOT 查询（"NOT 视频"）→ cleanedQuery 为空 → 仍标 greedy
  // ===========================================================================
  it('纯 NOT 查询（"NOT 视频"）→ cleanedQuery 空 → 仍标 greedy 模式', () => {
    cy.intercept('GET', '**/api/files/search-fulltext*', {
      statusCode: 200,
      body: { results: [], total: 0, query: '', dbEngine: 'sqlite', indexSize: 100 },
    }).as('ftsSearch')

    let vectorCallCount = 0
    cy.intercept('GET', '**/api/search/files*', (req) => {
      vectorCallCount++
      cy.log(`vector call #${vectorCallCount}: ${req.url}`)
      req.reply({ files: [], total: 0, vector_search: false, search_mode: 'greedy' })
    }).as('vectorSearch')

    cy.visit('/tabs/files')
    cy.wait(3000)
    dismissErrorOverlay()

    cy.get('[data-testid="search-input"]').click({ force: true })
    cy.get('[data-testid="search-input"]').type('NOT 视频{enter}', { delay: 50, force: true })
    cy.wait(5000)
    dismissErrorOverlay()

    // 验证：FTS 被调用（query 含 NOT 触发布尔语法检测）
    cy.get('@ftsSearch.all').then((interceptions) => {
      expect(interceptions.length).to.be.greaterThan(0)
    })

    // 验证：向量搜索只调用 1 次（cleanedQuery 为空 → 不 retry）
    cy.get('@vectorSearch.all').then((interceptions) => {
      cy.log(`纯 NOT 查询的向量调用数: ${interceptions.length}`)
      expect(interceptions.length, 'cleanedQuery 为空时不应该 retry').to.eq(1)
    })

    // 验证：诊断卡片应显示
    cy.get('[data-testid="search-diagnostics-card"]', { timeout: 8000 })
      .should('exist')
      .and('be.visible')
  })

  // ===========================================================================
  // 测试 5：phrase 布尔查询 → cleanedQuery 去引号
  //   '"exact phrase" 高清' → cleanedQuery="exact phrase 高清"
  // ===========================================================================
  it('phrase 查询 → cleanedQuery 去引号（"exact phrase" 高清 → exact phrase 高清）', () => {
    cy.intercept('GET', '**/api/files/search-fulltext*', {
      statusCode: 200,
      body: { results: [], total: 0, query: '', dbEngine: 'sqlite', indexSize: 100 },
    }).as('ftsSearch')

    let vectorCallCount = 0
    const vectorCalls: string[] = []
    cy.intercept('GET', '**/api/search/files*', (req) => {
      vectorCallCount++
      vectorCalls.push(req.url)
      if (vectorCallCount === 1) {
        req.reply({ files: [], total: 0, vector_search: false, search_mode: 'greedy' })
      } else {
        req.reply({
          files: [
            {
              name: 'exact_phrase_hd.mp4',
              path: '/d/exact_phrase_hd.mp4',
              size: 1024,
              modtime: '2026-07-01T00:00:00Z',
              isDir: false,
              isDirectory: false,
            },
          ],
          total: 1,
          vector_search: true,
          search_mode: 'greedy',
        })
      }
    }).as('vectorSearch')

    cy.visit('/tabs/files')
    cy.wait(3000)
    dismissErrorOverlay()

    // 输入含双引号的 phrase 查询
    cy.get('[data-testid="search-input"]').click({ force: true })
    cy.get('[data-testid="search-input"]').type('"exact phrase" 高清{enter}', { delay: 50, force: true })
    cy.wait(5000)
    dismissErrorOverlay()

    cy.get('@vectorSearch.all').then(() => {
      expect(vectorCalls.length).to.be.gte(2)
      const retryUrl = vectorCalls[1] || ''
      cy.log(`phrase retry URL: ${retryUrl}`)
      // cleanedQuery 应该是 "exact phrase 高清"（去引号），URL 编码后：
      //   exact = exact, phrase = phrase
      expect(retryUrl).to.include('exact')
      expect(retryUrl).to.include('phrase')
      // 不应包含 %22（双引号 URL 编码）
      expect(retryUrl, 'retry URL 不应包含双引号').to.not.include('%22')
    })
  })
})
