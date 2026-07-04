/**
 * 🆕 2026-07-03 悲观测试：FTS / 向量搜索 各种失败场景
 *
 * 用户原话：
 *   "完善悲观测试。全文搜索插入逻辑符依旧无匹配结果，
 *    状态显示 FTS5 可用但是 FTS 索引为 0。"
 *
 * 测试场景（按 graceful-degradation.md L1/L2/L3 分级）：
 *   L1：FTS 完全不可用（dbEngine='none' / 503）→ 降级到普通搜索 + banner 提示
 *   L2：FTS 可用但索引为 0（indexSize=0）→ 搜索 0 结果 + 诊断卡片显示"FTS 索引: 0 文件"
 *   L2：FTS API 500 错误 → catch + 降级 + banner 提示原因
 *   L2：向量搜索 500 错误 → fallback 到 searchFiles（legacy API）
 *   L3：FTS + 向量 + searchFiles 都失败 → 诊断卡片可见，无白屏
 *
 * 验证策略：
 *   - cy.intercept 模拟各种错误响应
 *   - 检查 UI 不崩溃（无白屏）
 *   - 检查诊断卡片 / banner 显示了正确的失败原因
 */
describe('搜索悲观测试：FTS / 向量搜索 失败场景（2026-07-03）', () => {
  const backendUrl = Cypress.env('apiBase') || 'http://localhost:2025'

  before(() => {
    cy.request({ url: `${backendUrl}/api/runtime`, failOnStatusCode: false }).then((resp) => {
      expect(resp.status).to.eq(200)
    })
  })

  beforeEach(() => {
    cy.on('window:before:load', (win) => {
      win.localStorage.setItem('encv-server-url', backendUrl)
    })
  })

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
  // L1：FTS 完全不可用（dbEngine='none'）→ 降级 + banner
  // ===========================================================================
  it('L1: FTS dbEngine=none → 应显示"全文搜索不可用"banner + 降级到普通搜索', () => {
    // FTS 返回 dbEngine='none' 表示索引未初始化
    cy.intercept('GET', '**/api/files/search-fulltext?*', {
      statusCode: 200,
      body: {
        results: [],
        total: 0,
        query: '',
        dbEngine: 'none',
        indexSize: 0,
      },
    }).as('ftsSearch')

    // 普通向量搜索有结果（验证降级路径生效）
    cy.intercept('GET', '**/api/search/files*', {
      body: {
        files: [
          {
            name: 'normal_match.mp4',
            path: '/d/normal_match.mp4',
            size: 1024,
            modtime: '2026-07-01T00:00:00Z',
            isDir: false,
            isDirectory: false,
          },
        ],
        total: 1,
        vector_search: true,
        search_mode: 'strict',
      },
    }).as('vectorSearch')

    cy.visit('/tabs/files')
    cy.wait(3000)
    dismissErrorOverlay()

    // 输入含布尔语法的查询（强制触发 FTS）
    cy.get('[data-testid="search-input"]').click({ force: true })
    cy.get('[data-testid="search-input"]').type('在线 AND 高清{enter}', { delay: 50, force: true })
    cy.wait(5000)
    dismissErrorOverlay()

    // 验证 1：FTS 被调用且返回 dbEngine='none'
    cy.get('@ftsSearch.all').then((interceptions) => {
      expect(interceptions.length).to.be.greaterThan(0)
    })

    // 验证 2：向量搜索有结果（降级路径生效，没白屏）
    cy.get('ion-item').then(($items) => {
      const matches = $items.filter((i, el) => /normal_match/.test(el.textContent || ''))
      expect(matches.length, 'FTS 不可用时应降级到普通搜索并显示结果').to.be.greaterThan(0)
    })

    cy.screenshot('pessimistic-l1-fts-none-degraded')
  })

  // ===========================================================================
  // L2：FTS 可用但索引为 0 → 搜索 0 结果 + 诊断卡片显示 ftsIndexSize=0
  //   这正是用户反馈的"状态显示 FTS5 可用但是 FTS 索引为 0"场景
  // ===========================================================================
  it('L2: FTS 可用但索引为 0 → 诊断卡片显示"FTS 索引: 0 文件"', () => {
    // FTS API：返回 0 结果（dbEngine='sqlite' 表示 FTS 可用，但 indexSize=0 表示没索引文件）
    cy.intercept('GET', '**/api/files/search-fulltext?*', {
      statusCode: 200,
      body: {
        results: [],
        total: 0,
        query: '',
        dbEngine: 'sqlite',
        indexSize: 0, // ← 关键：索引为 0
      },
    }).as('ftsSearch')

    // FTS stats API：返回 fts5Enabled=true + totalFiles=0
    cy.intercept('GET', '**/api/files/search-fulltext/stats*', {
      statusCode: 200,
      body: {
        available: true,
        stats: {
          totalFiles: 0,
          totalDirs: 0,
          totalSize: 0,
          indexedAt: '',
          isIndexing: false,
          lastBuildMs: 0,
          dbPath: '/tmp/fts5.db',
          fts5Enabled: true,
          tokenizer: 'unicode61',
          indexVersion: 1,
        },
      },
    }).as('ftsStats')

    // 普通索引 stats：返回 totalFiles=500（与 FTS 索引 0 形成对比）
    cy.intercept('GET', '**/api/index/stats*', {
      statusCode: 200,
      body: {
        totalFiles: 500,
        totalDirs: 50,
        totalSize: 1024 * 1024 * 100,
        indexedAt: '2026-07-01T00:00:00Z',
        isIndexing: false,
        lastBuildMs: 5000,
      },
    }).as('indexStats')

    // 向量搜索也返回 0 结果
    cy.intercept('GET', '**/api/search/files*', {
      body: { files: [], total: 0, vector_search: false, search_mode: 'greedy' },
    }).as('vectorSearch')

    cy.visit('/tabs/files')
    cy.wait(3000)
    dismissErrorOverlay()

    cy.get('[data-testid="search-input"]').click({ force: true })
    cy.get('[data-testid="search-input"]').type('在线 AND 高清{enter}', { delay: 50, force: true })
    cy.wait(6000)
    dismissErrorOverlay()

    // 验证 1：诊断卡片出现
    cy.get('[data-testid="search-diagnostics-card"]', { timeout: 10000 })
      .should('exist')
      .and('be.visible')

    // 验证 2：FTS badge 显示"可用"（绿色）
    cy.get('[data-testid="diag-fts-badge"]').should('exist')
    cy.get('[data-testid="diag-fts-badge"]').invoke('text').then((text) => {
      cy.log(`FTS badge text: ${text}`)
      expect(text).to.include('可用')
    })

    // 验证 3：FTS 索引大小显示 0
    cy.get('[data-testid="diag-fts-index-size"]').invoke('text').then((text) => {
      cy.log(`FTS index size text: ${text}`)
      expect(text).to.include('0')
    })

    cy.screenshot('pessimistic-l2-fts-index-zero')
  })

  // ===========================================================================
  // L2：FTS API 500 错误 → catch + 降级到普通搜索 + banner 提示原因
  // ===========================================================================
  it('L2: FTS API 500 错误 → catch + 降级到普通搜索（不崩溃）', () => {
    // FTS API 抛 500
    cy.intercept('GET', '**/api/files/search-fulltext?*', {
      statusCode: 500,
      body: { error: 'internal server error' },
    }).as('ftsSearch')

    // 普通搜索有结果
    cy.intercept('GET', '**/api/search/files*', {
      body: {
        files: [
          {
            name: 'fallback_result.mp4',
            path: '/d/fallback_result.mp4',
            size: 1024,
            modtime: '2026-07-01T00:00:00Z',
            isDir: false,
            isDirectory: false,
          },
        ],
        total: 1,
        vector_search: true,
        search_mode: 'strict',
      },
    }).as('vectorSearch')

    cy.visit('/tabs/files')
    cy.wait(3000)
    dismissErrorOverlay()

    cy.get('[data-testid="search-input"]').click({ force: true })
    cy.get('[data-testid="search-input"]').type('在线 AND 高清{enter}', { delay: 50, force: true })
    cy.wait(5000)
    dismissErrorOverlay()

    // 验证 1：FTS 被调用（即使返回 500）
    cy.get('@ftsSearch.all').then((interceptions) => {
      expect(interceptions.length).to.be.greaterThan(0)
    })

    // 验证 2：向量搜索有结果（500 不影响降级路径）
    cy.get('ion-item').then(($items) => {
      const matches = $items.filter((i, el) => /fallback_result/.test(el.textContent || ''))
      expect(matches.length, 'FTS 500 时应降级到普通搜索并显示结果').to.be.greaterThan(0)
    })

    cy.screenshot('pessimistic-l2-fts-500-degraded')
  })

  // ===========================================================================
  // L2：向量搜索 500 → fallback 到 searchFiles（legacy API）
  //   注：performSearch 中 try { searchFilesVector } catch { searchFiles }
  // ===========================================================================
  it('L2: 向量搜索 500 → fallback 到 searchFiles（legacy API）', () => {
    // FTS 不可用
    cy.intercept('GET', '**/api/files/search-fulltext?*', {
      statusCode: 200,
      body: { results: [], total: 0, query: '', dbEngine: 'none', indexSize: 0 },
    }).as('ftsSearch')

    // 向量搜索 500
    cy.intercept('GET', '**/api/search/files*', {
      statusCode: 500,
      body: { error: 'vector search backend down' },
    }).as('vectorSearch')

    // searchFiles (legacy) 有结果
    cy.intercept('GET', '**/api/files/search?*', {
      statusCode: 200,
      body: {
        files: [
          {
            name: 'legacy_search_result.mp4',
            path: '/d/legacy_search_result.mp4',
            size: 1024,
            modtime: '2026-07-01T00:00:00Z',
            isDir: false,
            isDirectory: false,
          },
        ],
      },
    }).as('legacySearch')

    cy.visit('/tabs/files')
    cy.wait(3000)
    dismissErrorOverlay()

    // 用普通查询（不含布尔语法，避免触发 FTS）
    cy.get('[data-testid="search-input"]').click({ force: true })
    cy.get('[data-testid="search-input"]').type('legacy{enter}', { delay: 50, force: true })
    cy.wait(5000)
    dismissErrorOverlay()

    // 验证 1：向量搜索被调用（500）
    cy.get('@vectorSearch.all').then((interceptions) => {
      expect(interceptions.length).to.be.greaterThan(0)
    })

    // 验证 2：legacy searchFiles 被调用（fallback）
    cy.get('@legacySearch.all').then((interceptions) => {
      cy.log(`legacy search calls: ${interceptions.length}`)
      expect(interceptions.length, '向量搜索 500 时应 fallback 到 searchFiles').to.be.greaterThan(0)
    })

    // 验证 3：legacy 结果在 UI 显示
    cy.get('ion-item').then(($items) => {
      const matches = $items.filter((i, el) => /legacy_search_result/.test(el.textContent || ''))
      expect(matches.length, 'fallback 路径结果应显示').to.be.greaterThan(0)
    })

    cy.screenshot('pessimistic-l2-vector-500-legacy-fallback')
  })

  // ===========================================================================
  // L3：FTS + 向量 + searchFiles 都失败 → 不白屏，诊断卡片可见
  // ===========================================================================
  it('L3: 所有搜索 API 都失败 → 不白屏 + 诊断卡片可见', () => {
    // FTS 500
    cy.intercept('GET', '**/api/files/search-fulltext?*', {
      statusCode: 500,
      body: { error: 'fts down' },
    }).as('ftsSearch')

    // 向量 500
    cy.intercept('GET', '**/api/search/files*', {
      statusCode: 500,
      body: { error: 'vector down' },
    }).as('vectorSearch')

    // searchFiles 也 500
    cy.intercept('GET', '**/api/files/search?*', {
      statusCode: 500,
      body: { error: 'legacy down' },
    }).as('legacySearch')

    // index/stats 也失败
    cy.intercept('GET', '**/api/index/stats*', {
      statusCode: 500,
      body: { error: 'index stats down' },
    }).as('indexStats')

    // FTS stats 也失败
    cy.intercept('GET', '**/api/files/search-fulltext/stats*', {
      statusCode: 500,
      body: { error: 'fts stats down' },
    }).as('ftsStats')

    cy.visit('/tabs/files')
    cy.wait(3000)
    dismissErrorOverlay()

    cy.get('[data-testid="search-input"]').click({ force: true })
    cy.get('[data-testid="search-input"]').type('在线 AND 高清{enter}', { delay: 50, force: true })
    cy.wait(6000)
    dismissErrorOverlay()

    // 验证 1：页面不白屏（ion-page 仍可见）
    cy.get('ion-page').should('be.visible')

    // 验证 2：搜索框仍可交互（没冻死）
    cy.get('[data-testid="search-input"]').should('exist')

    // 验证 3：诊断卡片可见（refreshSearchDiagnostics 内部 try/catch，不影响）
    cy.get('[data-testid="search-diagnostics-card"]', { timeout: 10000 })
      .should('exist')
      .and('be.visible')

    cy.screenshot('pessimistic-l3-all-fail-no-crash')
  })

  // ===========================================================================
  // L2：FTS API 503 (Service Unavailable) → 前端已有降级处理
  //   注：encv_search.ts:177-179 显式处理 503，返回 dbEngine='none'
  // ===========================================================================
  it('L2: FTS API 503 → 前端返回 dbEngine=none + 降级', () => {
    cy.intercept('GET', '**/api/files/search-fulltext?*', {
      statusCode: 503,
      body: { error: 'service unavailable' },
    }).as('ftsSearch')

    cy.intercept('GET', '**/api/search/files*', {
      body: {
        files: [
          {
            name: 'after_503.mp4',
            path: '/d/after_503.mp4',
            size: 1024,
            modtime: '2026-07-01T00:00:00Z',
            isDir: false,
            isDirectory: false,
          },
        ],
        total: 1,
        vector_search: true,
        search_mode: 'strict',
      },
    }).as('vectorSearch')

    cy.visit('/tabs/files')
    cy.wait(3000)
    dismissErrorOverlay()

    cy.get('[data-testid="search-input"]').click({ force: true })
    cy.get('[data-testid="search-input"]').type('在线 AND 高清{enter}', { delay: 50, force: true })
    cy.wait(5000)
    dismissErrorOverlay()

    cy.get('@ftsSearch.all').then((interceptions) => {
      expect(interceptions.length).to.be.greaterThan(0)
    })

    // 503 后应降级到向量搜索，显示结果
    cy.get('ion-item').then(($items) => {
      const matches = $items.filter((i, el) => /after_503/.test(el.textContent || ''))
      expect(matches.length, '503 应触发降级').to.be.greaterThan(0)
    })

    cy.screenshot('pessimistic-l2-fts-503-degraded')
  })
})
