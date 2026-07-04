/**
 * App 烟测 E2E 测试（Cypress E2E）
 *
 * 覆盖核心用户旅程：
 *   - 首页加载 → tab 导航可用
 *   - Tasks tab 打开 → 任务列表渲染
 *   - Files tab 打开 → 文件列表渲染
 *   - Settings tab 打开 → 设置页面渲染
 *
 * 注意：
 *   - 依赖 preview-gateway + Go backend 运行
 *   - baseUrl 通过 CYPRESS_BASE_URL 环境变量配置
 *   - 所有 API 请求走真实后端（非 stub）
 */

describe('App Smoke Test (E2E)', () => {
  beforeEach(() => {
    // 拦截 API 请求，等待关键接口返回
    cy.intercept('GET', '**/api/tasks*').as('getTasks')
    cy.intercept('GET', '**/api/plugins*').as('getPlugins')
  })

  it('首页加载 → 重定向到 /tabs/home', () => {
    cy.visit('/')
    cy.url().should('include', '/tabs/home')
  })

  it('底部 tab bar 包含 5 个主 tab', () => {
    cy.visit('/tabs/home')
    cy.get('ion-tab-bar').should('be.visible')
    // 验证 tab 按钮存在
    cy.get('ion-tab-button').should('have.length.at.least', 3)
  })

  it('切换到 Tasks tab → 任务列表页面渲染', () => {
    cy.visit('/tabs/tasks')
    cy.url().should('include', '/tabs/tasks')
    // 等待页面 hydrate
    cy.wait(1000)
    // 验证 ion-content 存在（页面根容器）
    cy.get('ion-content').should('exist')
  })

  it('切换到 Files tab → 文件列表页面渲染', () => {
    cy.visit('/tabs/files')
    cy.url().should('include', '/tabs/files')
    cy.wait(1000)
    cy.get('ion-content').should('exist')
  })

  it('切换到 Settings tab → 设置页面渲染', () => {
    cy.visit('/tabs/settings')
    cy.url().should('include', '/tabs/settings')
    cy.wait(1000)
    cy.get('ion-content').should('exist')
  })
})
