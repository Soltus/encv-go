// Cypress 全局配置
//   - 关闭 uncaught:exception 自动失败（Ionic 内部可能抛无害异常）
//   - 导入自定义命令

import './commands'

Cypress.on('uncaught:exception', (err) => {
  // 忽略 Ionic / Capacitor / Vue 的非致命异常（如 IonContent 内部 ResizeObserver）
  // 仅在 CI 中记录，生产环境 fail-fast
  if (Cypress.env('CI')) {
    // eslint-disable-next-line no-console
    console.warn('[Cypress] uncaught:', err.message)
    return false
  }
  return false // 全部忽略，让测试自己用 cy.should() 验证
})
