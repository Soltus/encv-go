// 自定义 Cypress 命令
//   - cy.dataCy(selector): 通过 data-cy 属性定位
//   - cy.resetTasksStore(): 重置 task store（防止测试间状态污染）

Cypress.Commands.add('dataCy', (selector: string) => {
  return cy.get(`[data-cy=${selector}]`)
})
