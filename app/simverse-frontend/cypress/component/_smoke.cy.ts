// Smoke test - 验证 Cypress 组件测试基础设施是否正常
describe('SimVerse Smoke Test', () => {
  it('should mount component', () => {
    // 基础断言，验证测试框架正常工作
    expect(true).to.be.true
  })

  it('should have cypress running', () => {
    // 验证 Cypress 环境
    cy.window().then((win) => {
      expect(win).to.exist
    })
  })
})
