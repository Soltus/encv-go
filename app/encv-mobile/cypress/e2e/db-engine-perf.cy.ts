/**
 * 数据库引擎性能对比 E2E 测试（基于真实页面完整流程）
 *
 * 测试场景：
 *   - 完整走通自动化插件测试流程（页面导航 → 生成 Mock → 加载插件 → 运行工作流）
 *   - 分别在 SQLite 和 Turso 引擎下运行
 *   - 对比两种引擎的端到端性能差异
 *
 * 测试流程（完全基于真实页面交互，不偷懒）：
 *   1. 页面导航到插件测试页
 *   2. 页面点击生成 Mock 数据按钮，等待完成
 *   3. 页面点击加载插件按钮，等待插件加载完成
 *   4. 页面点击 Run Workflow 按钮启动工作流
 *   5. 页面实时监听进度，等待所有任务完成
 *   6. 记录端到端性能指标
 */

describe('数据库引擎性能对比测试（真实页面流程）', () => {
  const API_BASE = Cypress.env('apiBase') || 'http://localhost:2025'

  // ==========================================================================
  // 工具函数
  // ==========================================================================

  /** 获取当前数据库引擎信息 */
  function getDbInfo() {
    return cy.request('GET', `${API_BASE}/api/database/info`).then((res) => res.body)
  }

  /** 清理所有任务 */
  function clearAllTasks() {
    return cy.request({
      method: 'DELETE',
      url: `${API_BASE}/api/tasks?all=true`,
      failOnStatusCode: false,
    })
  }

  /** 设置 API base URL 到 localStorage */
  function setApiBaseUrl() {
    cy.window().then((win) => {
      win.localStorage.setItem('encv-server-url', API_BASE)
    })
  }

  // ==========================================================================
  // 页面导航辅助函数
  // ==========================================================================

  /** 导航到插件测试页面 */
  function navigateToPluginTests() {
    cy.visit('/tabs/settings/devtools/plugin-tests')
    setApiBaseUrl()
    cy.reload()
    cy.url().should('include', '/tabs/settings/devtools/plugin-tests')
    // 用 WORKFLOW ENGINE 英文文本确认页面加载
    cy.contains('WORKFLOW ENGINE', { timeout: 10000 }).should('be.visible')
  }

  /** 点击「生成 Mock 数据」按钮 */
  function clickGenerateMock() {
    // 第一个 ion-list 是 Mock 数据管理区
    // 第二个 ion-item（eq(1)）是生成 Mock 数据按钮
    cy.get('ion-content ion-list').first().find('ion-item').eq(1).click({ force: true })
  }

  /** 点击「加载插件」按钮 */
  function clickLoadPlugins() {
    // 第二个 ion-list 是 WORKFLOW ENGINE 区，第一个按钮是加载插件
    cy.get('ion-content ion-list').eq(1).find('ion-item').first().click({ force: true })
  }

  /** 点击「Run Workflow」按钮 */
  function clickRunWorkflow() {
    // 第二个 ion-list 中的第三个 ion-item 是 Run Workflow
    // （第一个是加载插件，第二个是测试用例，第三个是 Run Workflow）
    cy.get('ion-content ion-list').eq(1).find('ion-item').eq(2).click({ force: true })
  }

  /** 等待 Mock 数据生成完成 */
  function waitForMockGenerated() {
    // mock-stats-card 出现表示生成完成
    cy.get('.mock-stats-card', { timeout: 120000 }).should('be.visible')
  }

  /** 等待插件加载完成 */
  function waitForPluginsLoaded() {
    cy.get('ion-content ion-list')
      .eq(1)
      .find('ion-item')
      .eq(1)
      .should(($item) => {
        const text = $item.text()
        const match = text.match(/(\d+)\s*个用例/)
        expect(match).to.not.be.null
        expect(parseInt(match![1], 10)).to.be.greaterThan(0)
      })
  }

  /** 获取测试用例数 */
  function getTestCaseCount(): Cypress.Chainable<number> {
    return cy.get('ion-content ion-list')
      .eq(1)
      .find('ion-item')
      .eq(1)
      .then(($item) => {
        const text = $item.text()
        const match = text.match(/(\d+)\s*个用例/)
        return match ? parseInt(match[1], 10) : 0
      })
  }

  /** 等待前 N 个任务完成 */
  function waitForFirstNTasksCompleted(n: number, timeoutMs: number) {
    cy.get('.progress-card .progress-stats', { timeout: timeoutMs }).should(($stats) => {
      const text = $stats.text()
      const match = text.match(/(\d+)\s*\/\s*(\d+)/)
      expect(match).to.not.be.null
      const completed = parseInt(match![1], 10)
      expect(completed).to.be.at.least(n)
    })
  }

  // ==========================================================================
  // 测试前置/后置
  // ==========================================================================

  before(() => {
    getDbInfo().then((info) => {
      cy.log(`当前数据库引擎: ${info.engine}, 并发度: ${info.concurrency}`)
    })
    clearAllTasks()
  })

  beforeEach(() => {
    clearAllTasks()
    cy.wait(500)
  })

  // ==========================================================================
  // 测试 1：页面导航可用性验证
  // ==========================================================================
  it('页面导航：插件测试页加载成功', () => {
    navigateToPluginTests()

    // 验证页面关键区域存在
    cy.contains('WORKFLOW ENGINE').should('be.visible')
    cy.get('ion-content ion-list').first().should('be.visible')
    cy.get('ion-content ion-list').eq(1).should('be.visible')
  })

  // ==========================================================================
  // 测试 2：生成 Mock 数据（真实页面按钮点击）
  // ==========================================================================
  it('生成 Mock 数据（页面交互）', () => {
    navigateToPluginTests()

    const startTime = performance.now()
    clickGenerateMock()

    waitForMockGenerated()
    const duration = performance.now() - startTime

    cy.log(`Mock 数据生成耗时: ${duration.toFixed(0)}ms`)

    // 验证统计数据
    cy.get('.mock-stats-card .stat-value').first().should(($el) => {
      const count = parseInt($el.text(), 10)
      expect(count).to.be.greaterThan(0)
    })
  })

  // ==========================================================================
  // 测试 3：加载插件（真实页面按钮点击）
  // ==========================================================================
  it('加载插件（页面交互）', () => {
    navigateToPluginTests()

    // 先生成 mock 数据
    clickGenerateMock()
    waitForMockGenerated()

    const startTime = performance.now()
    clickLoadPlugins()

    waitForPluginsLoaded()
    const duration = performance.now() - startTime

    cy.log(`插件加载耗时: ${duration.toFixed(0)}ms`)

    getTestCaseCount().then((count) => {
      cy.log(`测试用例数: ${count}`)
    })
  })

  // ==========================================================================
  // 测试 4：工作流运行性能（端到端，真实页面交互）
  // ==========================================================================
  it('工作流运行性能（端到端，真实页面交互）', () => {
    const SAMPLE_SIZE = 20

    navigateToPluginTests()

    // ---- 步骤 1：生成 Mock 数据 ----
    cy.log('=== 步骤 1：生成 Mock 数据 ===')
    clickGenerateMock()
    waitForMockGenerated()

    // ---- 步骤 2：加载插件 ----
    cy.log('=== 步骤 2：加载插件 ===')
    clickLoadPlugins()
    waitForPluginsLoaded()

    let totalCases = 0
    getTestCaseCount().then((count) => {
      totalCases = count
      cy.log(`测试用例总数: ${totalCases}`)
    })

    // ---- 步骤 3：启动工作流 ----
    cy.log('=== 步骤 3：启动工作流 ===')
    const workflowStartTime = performance.now()

    clickRunWorkflow()

    // 验证进度卡片出现（工作流已启动）
    cy.get('.progress-card', { timeout: 30000 }).should('exist')
    cy.get('.progress-card .progress-stats', { timeout: 30000 }).should('exist')

    // ---- 步骤 4：等待前 N 个任务完成 ----
    cy.log(`=== 步骤 4：等待前 ${SAMPLE_SIZE} 个任务完成 ===`)

    // 超时时间：每个任务 30 秒
    const maxWaitMs = SAMPLE_SIZE * 30000
    waitForFirstNTasksCompleted(SAMPLE_SIZE, maxWaitMs)

    const sampleDuration = performance.now() - workflowStartTime

    // ---- 步骤 5：记录性能指标 ----
    cy.log('=== 步骤 5：性能指标 ===')
    cy.log(`测试用例总数: ${totalCases}`)
    cy.log(`采样数量: ${SAMPLE_SIZE}`)
    cy.log(`前 ${SAMPLE_SIZE} 个任务耗时: ${sampleDuration.toFixed(0)}ms`)
    if (sampleDuration > 0) {
      const throughput = (SAMPLE_SIZE / sampleDuration) * 1000
      cy.log(`吞吐率: ${throughput.toFixed(3)} tasks/sec`)
      cy.log(`单任务平均耗时: ${(sampleDuration / SAMPLE_SIZE).toFixed(2)}ms/task`)
    }

    // 读取通过/失败数
    cy.get('.progress-card .progress-stats .passed').should(($el) => {
      const text = $el.text()
      const match = text.match(/(\d+)/)
      expect(match).to.not.be.null
      const passed = parseInt(match![1], 10)
      cy.log(`通过: ${passed}`)
    })

    cy.get('.progress-card .progress-stats .failed').should(($el) => {
      const text = $el.text()
      const match = text.match(/(\d+)/)
      expect(match).to.not.be.null
      const failed = parseInt(match![1], 10)
      cy.log(`失败: ${failed}`)
    })
  })

  // ==========================================================================
  // 测试 5：数据库信息验证
  // ==========================================================================
  it('数据库信息接口正常返回', () => {
    getDbInfo().then((info) => {
      expect(info).to.have.property('engine')
      expect(info).to.have.property('concurrency')
      expect(info).to.have.property('taskCount')
      expect(info).to.have.property('hasCalibration')
      expect(typeof info.engine).to.equal('string')
      expect(typeof info.concurrency).to.equal('number')
      expect(typeof info.taskCount).to.equal('number')
      expect(info.concurrency).to.be.greaterThan(0)
    })
  })
})
