// 自定义 Cypress 命令
//   - cy.dataCy(selector): 通过 data-cy 属性定位
//   - cy.resetTasksStore(): 重置 task store（防止测试间状态污染）
//   - cy.compareSnapshot(name, threshold?): 视觉回归截图对比（ESM: pixelmatch@7 + pngjs@7）

Cypress.Commands.add('dataCy', (selector: string) => {
  return cy.get(`[data-cy=${selector}]`)
})

// 视觉回归：先 cy.screenshot('<name>') 写入 cypress/screenshots/<name>.png，
// 再 cy.compareSnapshot('<name>', 0.1) 与 baseline 比对（node task 处理比对逻辑）。
Cypress.Commands.add('compareSnapshot', (name: string, threshold?: number) => {
  return cy.task('compareSnapshot', { name, threshold })
})
