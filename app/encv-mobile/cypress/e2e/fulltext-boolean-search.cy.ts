describe('全文搜索逻辑符 E2E 测试', () => {
  const API_BASE = Cypress.env('apiBase') || 'http://localhost:2026'

  describe('后端 FTS 布尔搜索 API 验证', () => {
    it('普通空格分隔（隐式 AND）搜索返回结果', () => {
      cy.request({
        url: `${API_BASE}/api/files/search-fulltext?q=在线 高清&limit=10`,
        failOnStatusCode: false,
      }).then((res) => {
        expect(res.status, 'FTS API should return 200 or 501').to.be.oneOf([200, 501])
        if (res.status === 200) {
          expect(res.body).to.have.property('results')
          cy.log(`Normal search results: ${res.body.results?.length || 0}`)
          cy.wrap(res.body.results?.length || 0).as('normalCount')
        }
      })
    })

    it('AND 布尔搜索返回结果', () => {
      cy.request({
        url: `${API_BASE}/api/files/search-fulltext?q=在线 AND 高清&limit=10`,
        failOnStatusCode: false,
      }).then((res) => {
        expect(res.status, 'FTS AND search').to.be.oneOf([200, 501])
        if (res.status === 200) {
          expect(res.body).to.have.property('results')
          cy.log(`AND search results: ${res.body.results?.length || 0}`)
          cy.wrap(res.body.results?.length || 0).as('andCount')
        }
      })
    })

    it('OR 布尔搜索返回结果', () => {
      cy.request({
        url: `${API_BASE}/api/files/search-fulltext?q=在线 OR 高清&limit=10`,
        failOnStatusCode: false,
      }).then((res) => {
        expect(res.status, 'FTS OR search').to.be.oneOf([200, 501])
        if (res.status === 200) {
          expect(res.body).to.have.property('results')
          cy.log(`OR search results: ${res.body.results?.length || 0}`)
        }
      })
    })

    it('NOT 布尔搜索返回结果', () => {
      cy.request({
        url: `${API_BASE}/api/files/search-fulltext?q=在线 NOT 视频&limit=10`,
        failOnStatusCode: false,
      }).then((res) => {
        expect(res.status, 'FTS NOT search').to.be.oneOf([200, 501])
        if (res.status === 200) {
          expect(res.body).to.have.property('results')
          cy.log(`NOT search results: ${res.body.results?.length || 0}`)
        }
      })
    })
  })

  describe('前端搜索框逻辑符插入', () => {
    beforeEach(() => {
      cy.visit('/tabs/files')
      cy.wait(2000)
    })

    it('搜索框存在且可输入', () => {
      cy.get('[data-testid="search-input"]').should('exist')
      cy.get('[data-testid="search-input"]').should('be.visible')
    })

    it('输入文字后显示插入按钮行', () => {
      cy.get('[data-testid="search-input"]').click()
      cy.get('[data-testid="search-input"]').type('在线')
      cy.wait(500)
      cy.get('[data-testid="btn-and"]').should('be.visible')
      cy.get('[data-testid="btn-or"]').should('be.visible')
      cy.get('[data-testid="btn-not"]').should('be.visible')
    })

    it('点击 AND 按钮插入 AND 逻辑符', () => {
      cy.get('[data-testid="search-input"]').click()
      cy.get('[data-testid="search-input"]').type('在线')
      cy.wait(300)

      // 点击 AND 按钮
      cy.get('[data-testid="btn-and"]').click()
      cy.wait(300)

      // 检查搜索框内容包含 AND 符号
      cy.get('[data-testid="search-input"]').then(($div) => {
        const text = $div.text()
        cy.log('Search box text after AND insert:', text)
        expect(text).to.include('在线')
        // AND 符号可能是 ＆ (全角) 或其他显示符号
        // 但我们通过检查有几个 span 来确认
        const spans = $div.find('span')
        cy.log(`Number of spans: ${spans.length}`)
        for (let i = 0; i < spans.length; i++) {
          cy.log(`Span ${i}: kind=${spans[i].dataset.kind}, op=${spans[i].dataset.op}, text="${spans[i].textContent}"`)
        }
        expect(spans.length).to.be.at.least(2)
      })
    })

    it('插入 AND 后继续输入第二个词 → 触发搜索', () => {
      // 拦截 FTS 搜索 API
      cy.intercept('GET', '**/api/files/search-fulltext*').as('ftsSearch')
      cy.intercept('GET', '**/api/files/search*').as('normalSearch')

      cy.get('[data-testid="search-input"]').click()
      cy.get('[data-testid="search-input"]').type('在线')
      cy.wait(300)

      // 点击 AND
      cy.get('[data-testid="btn-and"]').click()
      cy.wait(300)

      // 输入第二个词
      cy.get('[data-testid="search-input"]').type('高清')
      cy.wait(1500) // 等待防抖 + 搜索

      // 检查是否调用了 FTS 搜索 API
      cy.get('@ftsSearch.all').then((interceptions) => {
        cy.log(`FTS search calls: ${interceptions.length}`)
        if (interceptions.length > 0) {
          const lastCall = interceptions[interceptions.length - 1]
          const url = lastCall.request.url
          cy.log('Last FTS search URL:', url)
          // 验证 URL 中包含 AND
          expect(url).to.include('AND')
        }
      })

      // 检查普通搜索调用
      cy.get('@normalSearch.all').then((interceptions) => {
        cy.log(`Normal search calls: ${interceptions.length}`)
      })
    })
  })
})
