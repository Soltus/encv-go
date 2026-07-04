/**
 * 详细调试测试：搞清楚测试用例数量和进度
 */
describe('详细调试：测试用例数量验证', () => {
  const API_BASE = Cypress.env('apiBase') || 'http://localhost:2025'

  function setApiBaseUrl() {
    cy.window().then((win) => {
      win.localStorage.setItem('encv-server-url', API_BASE)
    })
  }

  function navigateToPluginTests() {
    cy.visit('/tabs/settings/devtools/plugin-tests')
    setApiBaseUrl()
    cy.reload()
    cy.url().should('include', '/tabs/settings/devtools/plugin-tests')
    cy.contains('WORKFLOW ENGINE', { timeout: 10000 }).should('be.visible')
  }

  function clickGenerateMock() {
    cy.get('ion-content ion-list').first().find('ion-item').eq(1).click({ force: true })
  }

  function waitForMockGenerated() {
    cy.get('.mock-stats-card', { timeout: 120000 }).should('be.visible')
  }

  function clickLoadPlugins() {
    cy.get('ion-content ion-list').eq(1).find('ion-item').first().click({ force: true })
  }

  function clickRunWorkflow() {
    cy.get('ion-content ion-list').eq(1).find('ion-item').eq(2).click({ force: true })
  }

  it('步骤1：检查测试用例数量', () => {
    navigateToPluginTests()
    clickGenerateMock()
    waitForMockGenerated()
    clickLoadPlugins()

    cy.wait(2000)

    // 获取第二个 ion-list 中所有 ion-item 的完整 HTML
    cy.get('ion-content ion-list').eq(1).find('ion-item').each(($item, index) => {
      const text = $item.text()
      const html = $item.html()
      cy.log(`ion-item[${index}] text: ${text.substring(0, 100)}`)
      cy.log(`ion-item[${index}] html length: ${html.length}`)
    })

    // 检查测试用例 ion-item 中的数字
    cy.get('ion-content ion-list').eq(1).find('ion-item').eq(1).then(($item) => {
      const text = $item.text()
      cy.log('=== 测试用例项完整文本 ===')
      cy.log(text)
      cy.log('========================')
      
      // 提取所有数字
      const numbers = text.match(/\d+/g)
      if (numbers) {
        cy.log(`找到的数字: ${numbers.join(', ')}`)
      }
    })

    cy.screenshot('debug-step1-test-cases')
  })

  it('步骤2：启动工作流后检查进度卡片', () => {
    navigateToPluginTests()
    clickGenerateMock()
    waitForMockGenerated()
    clickLoadPlugins()

    cy.wait(2000)

    // 滚动到页面底部，确保进度卡片在视口中
    cy.get('ion-content').scrollTo('bottom', { duration: 500 })
    cy.wait(500)

    // 打印 WORKFLOW ENGINE 区域的内容
    cy.contains('WORKFLOW ENGINE').then(($el) => {
      cy.log(`WORKFLOW ENGINE 可见: ${$el.is(':visible')}`)
    })

    // 统计有多少个 .progress-card 元素
    cy.get('.progress-card').then(($cards) => {
      cy.log(`.progress-card 元素数量: ${$cards.length}`)
    })

    // 统计有多少个 .progress-stats 元素
    cy.get('.progress-stats').then(($stats) => {
      cy.log(`.progress-stats 元素数量: ${$stats.length}`)
    })

    // 启动工作流
    clickRunWorkflow()

    // 等待进度卡片出现
    cy.get('.progress-card', { timeout: 30000 }).should('exist')

    // 再次滚动到底部
    cy.get('ion-content').scrollTo('bottom', { duration: 500 })
    cy.wait(500)

    // 打印所有 .progress-card 的文本
    cy.get('.progress-card').each(($card, index) => {
      const text = $card.text()
      cy.log(`progress-card[${index}] text: ${text.substring(0, 200)}`)
    })

    // 打印所有 .progress-stats 的文本
    cy.get('.progress-stats').each(($stats, index) => {
      const text = $stats.text()
      cy.log(`progress-stats[${index}] text: ${text.substring(0, 200)}`)
      
      // 提取所有数字
      const numbers = text.match(/\d+/g)
      if (numbers) {
        cy.log(`  数字: ${numbers.join(', ')}`)
      }
      
      // 提取所有 X/Y 模式
      const xyPatterns = text.match(/\d+\s*\/\s*\d+/g)
      if (xyPatterns) {
        cy.log(`  X/Y 模式: ${xyPatterns.join(', ')}`)
      }
    })

    cy.screenshot('debug-step2-progress-card')
  })
})
