describe('全文索引页面 E2E 诊断', () => {
  it('三级导航后检查 DOM 状态', () => {
    const logs: string[] = []

    cy.visit('/tabs/settings')
    cy.wait(2000)

    // Settings → Cache
    cy.get('ion-item').contains(/缓存|cache/i).click()
    cy.wait(2000)

    // Cache → FullText Index
    cy.get('.fulltext-entry').click()
    cy.wait(3000)

    // 检查所有 ion-page 元素
    cy.get('ion-page').then(($pages) => {
      logs.push(`=== ion-page 数量: ${$pages.length} ===`)
      $pages.each((i, el) => {
        const $el = Cypress.$(el)
        const rect = el.getBoundingClientRect()
        logs.push(`ion-page[${i}]: class="${$el.attr('class')}", style="${$el.attr('style')}", visible=${$el.is(':visible')}, rect=${JSON.stringify({w:rect.width,h:rect.height,top:rect.top,left:rect.left})}, title="${$el.find('ion-title').text()}"`)
      })
    })

    // 检查 ion-router-outlet
    cy.get('ion-router-outlet').then(($outlet) => {
      logs.push(`=== ion-router-outlet children: ${$outlet.children().length} ===`)
      $outlet.children().each((i, el) => {
        const $el = Cypress.$(el)
        logs.push(`  child[${i}]: tag=${el.tagName}, class="${$el.attr('class')}", style="${$el.attr('style')}"`)
      })
    })

    cy.then(() => {
      cy.writeFile('/tmp/cypress-dom-log.txt', logs.join('\n'))
    })
  })

  it('对比 DatabaseDetail vs FullTextIndexDetail', () => {
    const logs: string[] = []

    // 先测试 DatabaseDetail（正常工作）
    cy.visit('/tabs/settings')
    cy.wait(2000)
    cy.get('ion-item').contains(/数据库|database/i).click()
    cy.wait(3000)

    cy.get('ion-router-outlet').then(($outlet) => {
      logs.push('=== DatabaseDetail 导航后 ===')
      logs.push(`ion-router-outlet children: ${$outlet.children().length}`)
      $outlet.children().each((i, el) => {
        const $el = Cypress.$(el)
        logs.push(`  child[${i}]: tag=${el.tagName}, class="${$el.attr('class')}", style="${$el.attr('style')}"`)
      })
    })

    // 回到 settings
    cy.visit('/tabs/settings')
    cy.wait(2000)

    // 测试 CacheDetail → FullTextIndexDetail
    cy.get('ion-item').contains(/缓存|cache/i).click()
    cy.wait(2000)
    cy.get('.fulltext-entry').click()
    cy.wait(3000)

    cy.get('ion-router-outlet').then(($outlet) => {
      logs.push('')
      logs.push('=== FullTextIndexDetail 导航后 ===')
      logs.push(`ion-router-outlet children: ${$outlet.children().length}`)
      $outlet.children().each((i, el) => {
        const $el = Cypress.$(el)
        logs.push(`  child[${i}]: tag=${el.tagName}, class="${$el.attr('class')}", style="${$el.attr('style')}"`)
      })
    })

    // 回到 settings
    cy.visit('/tabs/settings')
    cy.wait(2000)

    // 测试直接访问 DevToolsDetail
    cy.visit('/tabs/settings/devtools')
    cy.wait(3000)

    cy.get('ion-router-outlet').then(($outlet) => {
      logs.push('')
      logs.push('=== DevToolsDetail 直接访问 ===')
      logs.push(`ion-router-outlet children: ${$outlet.children().length}`)
      $outlet.children().each((i, el) => {
        const $el = Cypress.$(el)
        logs.push(`  child[${i}]: tag=${el.tagName}, class="${$el.attr('class')}", style="${$el.attr('style')}"`)
      })
    })

    cy.then(() => {
      cy.writeFile('/tmp/cypress-compare-log.txt', logs.join('\n'))
    })
  })
})
