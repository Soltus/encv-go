/**
 * 简单调试测试：验证测试用例数量
 */
describe('调试：测试用例数量验证', () => {
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

  it('显示测试用例数量', () => {
    navigateToPluginTests()
    clickGenerateMock()
    waitForMockGenerated()
    clickLoadPlugins()

    // 等待测试用例数显示
    cy.wait(2000)

    // 打印第二个 ion-list 中的所有 ion-item 文本
    cy.get('ion-content ion-list').eq(1).find('ion-item').each(($item, index) => {
      cy.log(`ion-item[${index}]: ${$item.text()}`)
    })

    // 从 Vue 应用中获取 dynamicTestCases 长度（通过 window 对象）
    // 由于我们无法直接访问 Vue 组件的内部状态，我们通过 DOM 文本提取
    cy.get('ion-content ion-list').eq(1).find('ion-item').eq(1).then(($item) => {
      const text = $item.text()
      cy.log(`测试用例项文本: ${text}`)
      const match = text.match(/(\d+)\s*个用例/)
      if (match) {
        cy.log(`提取到的测试用例数: ${match[1]}`)
      }
    })

    // 截图
    cy.screenshot('debug-test-cases')
  })
})
