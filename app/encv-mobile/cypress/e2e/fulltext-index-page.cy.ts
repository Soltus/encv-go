describe('全文索引页面 E2E 测试', () => {
  beforeEach(() => {
    cy.visit('/tabs/settings')
    cy.wait(2000)
  })

  it('设置页面 → 缓存页面 → 全文索引页面（三级导航）', () => {
    // 拦截 FTS stats API
    cy.intercept('GET', '**/api/files/search-fulltext/stats*').as('ftsStats')

    // 步骤1：设置页面点击缓存入口
    cy.get('ion-item').contains(/缓存|cache/i).click()
    cy.wait(2000)

    // 步骤2：缓存页面应该有全文索引入口
    cy.get('.fulltext-entry').should('exist')
    cy.get('.fulltext-entry').should('be.visible')

    // 截图：缓存页面
    cy.screenshot('cache-page-with-fulltext-entry')

    // 步骤3：点击全文索引入口
    cy.get('.fulltext-entry').click()
    cy.wait(3000)

    // 步骤4：检查 URL
    cy.url().should('include', 'fulltext-index')

    // 步骤5：检查页面渲染
    cy.get('ion-page').should('be.visible')

    // 截图：全文索引页面
    cy.screenshot('fulltext-index-page-from-nav')

    // 检查页面内容
    cy.get('ion-content').should('exist')
    cy.get('ion-content').invoke('text').then((text) => {
      cy.log('页面内容:', text.substring(0, 500))
      expect(text.length).to.be.greaterThan(0)
    })
  })

  it('直接访问全文索引页面 URL', () => {
    cy.visit('/tabs/settings/fulltext-index')
    cy.wait(3000)

    cy.url().should('include', 'fulltext-index')
    cy.get('ion-page').should('be.visible')

    // 截图
    cy.screenshot('fulltext-index-direct-access')

    // 检查页面内容
    cy.get('ion-content').should('exist')
    cy.get('ion-content').invoke('text').then((text) => {
      cy.log('Direct access 页面内容:', text.substring(0, 500))
    })
  })

  it('从缓存页面跳转后再返回 → 不崩溃', () => {
    // Settings → Cache → FullText Index
    cy.get('ion-item').contains(/缓存|cache/i).click()
    cy.wait(2000)
    cy.get('.fulltext-entry').click()
    cy.wait(3000)

    // 返回
    cy.get('ion-back-button').click()
    cy.wait(2000)

    // 应该回到缓存页面
    cy.url().should('include', 'cache')
    cy.get('ion-page').should('be.visible')
  })

  it('收集 console 错误', () => {
    const errors: string[] = []

    cy.on('window:before:load', (win) => {
      win.addEventListener('error', (e) => {
        errors.push(`window.error: ${e.message}`)
      })
      const origConsoleError = win.console.error
      win.console.error = (...args: any[]) => {
        const msg = args.map(a => typeof a === 'string' ? a : JSON.stringify(a)).join(' ')
        if (msg.includes('classList') || msg.includes('PROMISE') || msg.includes('undefined')) {
          errors.push(`console.error: ${msg}`)
        }
        origConsoleError.apply(win.console, args)
      }
    })

    // Settings → Cache → FullText Index
    cy.get('ion-item').contains(/缓存|cache/i).click()
    cy.wait(2000)
    cy.get('.fulltext-entry').click()
    cy.wait(3000)

    // 检查是否有 classList 相关错误
    cy.then(() => {
      if (errors.length > 0) {
        cy.log('=== 收集到的错误 ===')
        errors.forEach(err => cy.log(err))
      } else {
        cy.log('没有收集到错误')
      }
    })
  })
})
