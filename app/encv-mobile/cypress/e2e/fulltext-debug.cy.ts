describe('全文搜索逻辑符 E2E 测试 - 详细诊断', () => {
  const API_BASE = Cypress.env('apiBase') || 'http://localhost:2026'

  describe('后端 FTS 详细诊断', () => {
    it('打印所有布尔搜索的详细结果', () => {
      const queries = [
        '在线',
        '高清',
        '在线 高清',
        '在线 AND 高清',
        '在线 OR 高清',
        '在线 NOT 视频',
      ]

      for (const q of queries) {
        cy.request({
          url: `${API_BASE}/api/files/search-fulltext?q=${encodeURIComponent(q)}&limit=10`,
          failOnStatusCode: false,
        }).then((res) => {
          if (res.status === 200) {
            const results = res.body.results || []
            Cypress.log({
              name: 'FTS结果',
              message: `query="${q}" → ${results.length} results`,
            })
            console.log(`[FTS] "${q}" → ${results.length} results:`, results.map((r: any) => r.name || r.path))
          } else {
            Cypress.log({
              name: 'FTS失败',
              message: `query="${q}" → status ${res.status}`,
            })
          }
        })
      }
    })

    it('验证 AND 搜索结果是普通搜索的子集', () => {
      let normalCount = 0
      let andCount = 0

      cy.request({
        url: `${API_BASE}/api/files/search-fulltext?q=${encodeURIComponent('在线 高清')}&limit=10`,
        failOnStatusCode: false,
      }).then((res) => {
        normalCount = res.body?.results?.length || 0
        cy.log(`Normal "在线 高清": ${normalCount} results`)
      })

      cy.request({
        url: `${API_BASE}/api/files/search-fulltext?q=${encodeURIComponent('在线 AND 高清')}&limit=10`,
        failOnStatusCode: false,
      }).then((res) => {
        andCount = res.body?.results?.length || 0
        cy.log(`AND "在线 AND 高清": ${andCount} results`)

        // AND 结果应该 <= 普通结果
        expect(andCount, 'AND results should be <= normal results').to.be.lte(normalCount)
        // 如果普通搜索有结果，AND 也应该有结果（因为普通搜索就是隐式 AND）
        if (normalCount > 0) {
          expect(andCount, 'AND should return same results as implicit AND').to.equal(normalCount)
        }
      })
    })
  })

  describe('前端搜索详细诊断', () => {
    beforeEach(() => {
      cy.intercept('GET', '**/api/files/search-fulltext*').as('ftsSearch')
      cy.intercept('GET', '**/api/files/search*').as('normalSearch')
      cy.visit('/tabs/files')
      cy.wait(2000)
    })

    it('记录所有搜索 API 调用', () => {
      cy.get('[data-testid="search-input"]').click()
      cy.get('[data-testid="search-input"]').type('在线')
      cy.wait(1000)

      // 检查 AND 按钮是否可见
      cy.get('[data-testid="btn-and"]').should('be.visible')

      // 点击 AND
      cy.get('[data-testid="btn-and"]').click()
      cy.wait(500)

      // 检查搜索框 DOM 结构
      cy.get('[data-testid="search-input"]').then(($div) => {
        const div = $div[0]
        const text = div.textContent
        cy.log('搜索框 textContent:', `"${text}"`)
        cy.log('搜索框 innerHTML:', div.innerHTML.substring(0, 500))

        // 检查 span
        const spans = Array.from(div.querySelectorAll('span'))
        cy.log(`span 数量: ${spans.length}`)
        spans.forEach((s, i) => {
          cy.log(`  span[${i}]: kind=${s.dataset.kind}, op=${s.dataset.op}, text="${s.textContent}"`)
        })
      })

      cy.get('[data-testid="search-input"]').type('高清')
      cy.wait(2000)

      // 再次检查搜索框内容
      cy.get('[data-testid="search-input"]').then(($div) => {
        const text = $div.text()
        cy.log('最终搜索框内容:', `"${text}"`)
        cy.log('最终 innerHTML (前 500 字):', $div[0].innerHTML.substring(0, 500))
      })

      // 打印所有 FTS 搜索调用
      cy.get('@ftsSearch.all').then((interceptions: any) => {
        cy.log(`=== FTS 搜索调用次数: ${interceptions.length} ===`)
        interceptions.forEach((int: any, i: number) => {
          const url = int.request.url
          const status = int.response?.statusCode
          const body = int.response?.body
          cy.log(`FTS[${i}]: ${url}`)
          cy.log(`  status: ${status}, results: ${body?.results?.length || 0}`)
        })
      })

      // 打印所有普通搜索调用
      cy.get('@normalSearch.all').then((interceptions: any) => {
        cy.log(`=== 普通搜索调用次数: ${interceptions.length} ===`)
        interceptions.forEach((int: any, i: number) => {
          const url = int.request.url
          const status = int.response?.statusCode
          cy.log(`Normal[${i}]: ${url} → status ${status}`)
          // 解析 query 参数
          const urlObj = new URL(url)
          const keyword = urlObj.searchParams.get('keyword') || urlObj.searchParams.get('q')
          if (keyword) {
            cy.log(`  keyword/q: ${decodeURIComponent(keyword)}`)
          }
        })
        // 至少应该有普通搜索调用
        expect(interceptions.length, '至少有一次普通搜索调用').to.be.greaterThan(0)
      })
    })
  })
})
