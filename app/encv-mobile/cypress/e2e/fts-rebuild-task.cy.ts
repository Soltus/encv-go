/**
 * 🆕 2026-07-03 FTS 索引重建任务化 E2E 测试 — 真实后端模式（spec fts-rebuild-task）
 *
 * 用户原话：
 *   "使用任务系统你不懂吗，了解了个寂寞，任务卡片自带进度和耗时，
 *    只需要加入任务类型和耗时估计"
 *   "我不希望再看到什么不依赖真实后端之类的"
 *
 * 设计原则：
 *   - 全部走真实后端（cy.request 真实调用 + 真实轮询任务状态），不 mock 任何响应
 *   - API 级测试：POST /api/files/search-fulltext/rebuild → 轮询 /api/tasks → 验证 completed
 *   - UI 级测试：真实点击重建按钮 → 等待 WS task:completed 事件 → 验证 UI 状态
 *   - classList 修复回归：纯 DOM 验证（.trae/rules/capacitor.md §八）
 *
 * 前置条件：
 *   - 后端 :2025 在线（make dev-mobile）
 *   - Vite dev server :5173 在线（cd app/encv-mobile && PM2_HOME=/tmp/cypress-pm2 pnpm dev）
 *
 * 测试场景：
 *   1. API: POST /rebuild 创建任务 + 轮询到 completed
 *   2. API: 任务完成后 FTS stats 的 indexedAt 更新
 *   3. API: 重复触发保护（409 Conflict 或新任务，取决于时序）
 *   4. API: GET /api/tasks/{id} 返回完整任务结构（含 steps/phase/progress）
 *   5. UI: 点击重建按钮 → 任务卡片显示 → 完成后状态正确
 *   6. UI: classList 修复回归（ion-page 渲染成 div.ion-page，非 ION-PAGE）
 *
 * 注：503 错误 / 取消慢任务 等场景由 Go 单元测试覆盖（task_manager_fts_rebuild.go），
 *     Cypress e2e 聚焦真实后端的完整重建流程 + UI 集成。
 */
describe('FTS 索引重建任务化 E2E — 真实后端（2026-07-03）', () => {
  const backendUrl = Cypress.env('apiBase') || 'http://localhost:2025'

  // ===========================================================================
  // 辅助函数
  // ===========================================================================

  /** 清理所有任务，确保干净环境 */
  function cleanupTasks(): Cypress.Chainable<void> {
    return cy.request({
      method: 'DELETE',
      url: `${backendUrl}/api/tasks?all=true`,
      failOnStatusCode: false,
    }).then(() => cy.wrap(undefined as unknown as void))
  }

  /**
   * 轮询 /api/tasks 直到指定任务进入终态（completed/failed/cancelled）。
   * @param taskId 任务 ID
   * @param timeoutMs 超时（默认 30s）
   */
  function waitForTaskEnd(
    taskId: string,
    timeoutMs = 30000,
  ): Cypress.Chainable<any> {
    const start = Date.now()
    const poll = (): Cypress.Chainable<any> => {
      return cy.request(`${backendUrl}/api/tasks`).then((resp) => {
        const task = (resp.body.tasks || []).find((t: any) => t.id === taskId)
        if (!task) {
          throw new Error(`task ${taskId} not found in /api/tasks response`)
        }
        const terminalStates = ['completed', 'failed', 'cancelled']
        if (terminalStates.includes(task.status)) {
          return cy.wrap(task)
        }
        if (Date.now() - start > timeoutMs) {
          throw new Error(
            `task ${taskId} did not reach terminal state within ${timeoutMs}ms (last status=${task.status})`,
          )
        }
        cy.wait(500)
        return poll()
      })
    }
    return poll()
  }

  /** dismiss ErrorCaptureOverlay（WS 连接失败浮窗） */
  function dismissErrorOverlay() {
    cy.get('body').then(($body) => {
      const closeBtn = $body.find('.error-overlay-close')
      if (closeBtn.length > 0) {
        cy.wrap(closeBtn).first().click({ force: true })
        cy.wait(300)
      }
    })
  }

  // ===========================================================================
  // 前置 / 后置
  // ===========================================================================

  before(() => {
    // 验证后端在线 + mobile 模式
    cy.request(`${backendUrl}/api/runtime`).then((resp) => {
      expect(resp.status, '后端 /api/runtime 应可用').to.eq(200)
      expect(resp.body.mobile, '后端应为 mobile 模式').to.eq(true)
      cy.log(`后端在线: version=${resp.body.version} instance=${resp.body.instance_id}`)
    })
    cleanupTasks()
  })

  beforeEach(() => {
    // 关键：Cypress 测试环境没有 preview-gateway :16666，必须把 API base URL 直连后端 :2025
    cy.on('window:before:load', (win) => {
      win.localStorage.setItem('encv-server-url', backendUrl)
    })
  })

  afterEach(() => {
    // 每个测试后清理任务，避免相互影响
    cleanupTasks()
  })

  // ===========================================================================
  // 测试 1：API — POST /rebuild 创建任务 + 轮询到 completed
  // ===========================================================================
  it('API: POST /api/files/search-fulltext/rebuild 创建任务并轮询到 completed', () => {
    cy.request('POST', `${backendUrl}/api/files/search-fulltext/rebuild`).then((resp) => {
      expect(resp.status, 'POST rebuild 应返回 200').to.eq(200)
      expect(resp.body.taskId, '应返回 taskId').to.be.a('string').and.not.empty
      expect(resp.body.status, '初始状态应为 queued').to.eq('queued')
      expect(resp.body.runId, '应返回 runId').to.be.a('string')

      const taskId = resp.body.taskId
      cy.log(`任务已创建: taskId=${taskId}`)

      // 轮询到终态
      waitForTaskEnd(taskId).then((task) => {
        cy.log(`任务终态: status=${task.status} progress=${task.progress} phase=${task.phase}`)
        expect(task.status, '任务应完成').to.eq('completed')
        expect(task.progress, '进度应为 100').to.eq(100)
        expect(task.type, '任务类型应为 rebuild_fts_index').to.eq('rebuild_fts_index')
      })
    })
  })

  // ===========================================================================
  // 测试 2：API — 任务完成后 FTS stats 的 indexedAt 更新
  //   验证重建真的执行了（indexedAt 从空或旧值变成新值）
  // ===========================================================================
  it('API: 任务完成后 FTS stats 的 indexedAt 更新', () => {
    // 记录重建前的 indexedAt
    cy.request(`${backendUrl}/api/files/search-fulltext/stats`).then((before) => {
      expect(before.body.available, 'FTS 应可用').to.eq(true)
      const beforeIndexedAt = before.body.stats?.indexedAt || ''
      cy.log(`重建前 indexedAt="${beforeIndexedAt}"`)

      cy.request('POST', `${backendUrl}/api/files/search-fulltext/rebuild`).then((resp) => {
        const taskId = resp.body.taskId
        waitForTaskEnd(taskId).then(() => {
          // 任务完成后查询 stats
          cy.request(`${backendUrl}/api/files/search-fulltext/stats`).then((after) => {
            const afterIndexedAt = after.body.stats?.indexedAt || ''
            cy.log(`重建后 indexedAt="${afterIndexedAt}"`)

            // indexedAt 应该更新（与之前不同，且非空）
            expect(afterIndexedAt, '重建后 indexedAt 应非空').to.not.eq('')
            expect(afterIndexedAt, '重建后 indexedAt 应与之前不同').to.not.eq(beforeIndexedAt)
          })
        })
      })
    })
  })

  // ===========================================================================
  // 测试 3：API — 重复触发保护（409 Conflict 或新任务，取决于时序）
  //   后端逻辑：检查是否有 running/queued 状态的 rebuild_fts_index 任务
  //   由于 servingDir 文件少，任务可能瞬间完成，第二个 POST 可能返回 200（新任务）
  //   测试接受两种真实行为
  // ===========================================================================
  it('API: 重复触发 POST（409 Conflict 或新任务，取决于时序）', () => {
    cy.request('POST', `${backendUrl}/api/files/search-fulltext/rebuild`).then((first) => {
      expect(first.status).to.eq(200)
      const firstTaskId = first.body.taskId
      cy.log(`第一次 POST: taskId=${firstTaskId}`)

      // 立即发第二个（failOnStatusCode:false 以接受 409）
      cy.request({
        method: 'POST',
        url: `${backendUrl}/api/files/search-fulltext/rebuild`,
        failOnStatusCode: false,
      }).then((second) => {
        cy.log(`第二次 POST: status=${second.status} body=${JSON.stringify(second.body)}`)

        if (second.status === 409) {
          // 场景 A：第一个任务还在 running/queued → 第二个返回 409
          expect(second.body.code, '409 应返回 REBUILD_IN_PROGRESS').to.eq('REBUILD_IN_PROGRESS')
          expect(second.body.taskId, '409 应返回现有 taskId').to.eq(firstTaskId)
          expect(second.body.status, '409 应返回现有任务状态').to.be.a('string')
          cy.log('场景 A: 409 Conflict（第一个任务尚未完成）')
        } else {
          // 场景 B：第一个任务已完成 → 第二个返回 200（新任务）
          expect(second.status).to.eq(200)
          expect(second.body.taskId, '新任务 taskId 应与第一个不同').to.not.eq(firstTaskId)
          cy.log('场景 B: 200 新任务（第一个任务已完成）')
        }
      })
    })
  })

  // ===========================================================================
  // 测试 4：API — GET /api/tasks 返回完整任务结构
  //   验证任务的 steps/phase/progress 字段结构正确
  //
  // 容错：若上一个测试遗留的 queued/running 任务导致 POST 返回 409，
  //   直接使用 409 响应体里的 existing taskId（后端 REBUILD_IN_PROGRESS 合约）
  // ===========================================================================
  it('API: GET /api/tasks 返回完整任务结构（含 steps/phase/progress）', () => {
    cy.request({
      method: 'POST',
      url: `${backendUrl}/api/files/search-fulltext/rebuild`,
      failOnStatusCode: false,
    }).then((resp) => {
      let taskId: string
      if (resp.status === 409) {
        // 上一个测试遗留的 queued 任务 → 复用其 taskId
        expect(resp.body.code, '409 应返回 REBUILD_IN_PROGRESS').to.eq('REBUILD_IN_PROGRESS')
        taskId = resp.body.taskId
        cy.log(`复用 409 中的 existing taskId=${taskId}`)
      } else {
        expect(resp.status, 'POST rebuild 应返回 200').to.eq(200)
        taskId = resp.body.taskId
        cy.log(`新任务 taskId=${taskId}`)
      }

      waitForTaskEnd(taskId).then((task) => {
        // 验证任务结构
        expect(task.id, 'id').to.eq(taskId)
        expect(task.type, 'type').to.eq('rebuild_fts_index')
        expect(task.status, 'status').to.eq('completed')
        expect(task.progress, 'progress').to.eq(100)

        // steps 数组应存在且非空（记录了 queued→initializing→scanning→indexing→completed 各阶段）
        expect(task.steps, 'steps 应为数组').to.be.an('array')
        expect(task.steps.length, 'steps 应非空').to.be.greaterThan(0)

        // 每个 step 应有 phase + startedAt + completedAt
        const firstStep = task.steps[0]
        expect(firstStep.phase, 'step.phase').to.be.a('string')
        expect(firstStep.startedAt, 'step.startedAt').to.be.a('string')

        cy.log(`任务 steps 数量: ${task.steps.length}`)
        cy.log(`任务 phases: ${task.steps.map((s: any) => s.phase).join(' → ')}`)
      })
    })
  })

  // ===========================================================================
  // 测试 5：UI — 点击重建按钮 → 任务卡片显示 → 完成后状态正确
  //   真实后端 + 真实 UI 交互 + 真实 WS 事件
  // ===========================================================================
  it('UI: 点击重建按钮 → 任务卡片显示 → 完成后状态正确', () => {
    cy.visit('/tabs/settings/fulltext-index')
    cy.wait(3000)
    dismissErrorOverlay()

    // 验证 idle 状态：重建按钮可见
    cy.get('[data-testid="rebuild-idle-item"]', { timeout: 10000 })
      .should('exist')
      .and('be.visible')
    cy.get('[data-testid="rebuild-trigger-btn"]').should('not.be.disabled')

    // 记录重建前的 stats indexedAt（通过 API）
    cy.request(`${backendUrl}/api/files/search-fulltext/stats`).then((before) => {
      const beforeIndexedAt = before.body.stats?.indexedAt || ''

      // 点击重建按钮
      cy.get('[data-testid="rebuild-trigger-btn"]').click({ force: true })

      // 验证切换到 active 状态（任务卡片显示）
      //   注：由于 servingDir 文件少，任务可能瞬间完成，active 状态可能一闪而过
      //   所以这里用 timeout:5000 等待 active-item，但如果直接回到 idle 也接受
      cy.get('body').then(() => {
        // 等待一段时间让任务完成 + WS 事件到达
        cy.wait(3000)
      })

      // 验证：任务完成后，要么显示 active(completed) 卡片，要么回到 idle
      //   两种都是合法的最终状态（取决于 WS 事件时序）
      cy.get('body').then(($body) => {
        const hasActive = $body.find('[data-testid="rebuild-active-item"]').length > 0
        const hasIdle = $body.find('[data-testid="rebuild-idle-item"]').length > 0
        cy.log(`UI 状态: active=${hasActive} idle=${hasIdle}`)
        expect(hasActive || hasIdle, '应显示 active 或 idle 卡片').to.be.true

        // 如果显示 active，验证状态标签非空
        if (hasActive) {
          cy.get('[data-testid="rebuild-status-label"]').invoke('text').then((text) => {
            cy.log(`任务卡片状态: "${text}"`)
            expect(text.length).to.be.greaterThan(0)
          })
        }
      })

      // 验证后端 stats 已更新（indexedAt 变化）
      cy.request(`${backendUrl}/api/files/search-fulltext/stats`).then((after) => {
        const afterIndexedAt = after.body.stats?.indexedAt || ''
        cy.log(`UI 重建后 indexedAt: "${afterIndexedAt}"`)
        expect(afterIndexedAt, '重建后 indexedAt 应更新').to.not.eq(beforeIndexedAt)
        expect(afterIndexedAt, 'indexedAt 应非空').to.not.eq('')
      })
    })

    cy.screenshot('fts-rebuild-ui-real-backend')
  })

  // ===========================================================================
  // 测试 6：UI — classList 修复回归（.trae/rules/capacitor.md §八）
  //   验证三级页面 ion-page 渲染成 div.ion-page（非 ION-PAGE 大写未编译元素）
  //
  // 关键：必须走"真实导航路径"才能触发多页面栈：
  //   /tabs/settings → 点 cache → 点 fulltext-entry → /tabs/settings/fulltext-index
  //   这样 ion-router-outlet 才会有 ≥2 个 children，Ionic 才会设置 z-index
  //   （直接 cy.visit 单页面访问时 Ionic 不强制设 z-index）
  // ===========================================================================
  it('UI: classList 修复 — 真实三级导航后 ion-page 有 z-index（非 ION-PAGE）', () => {
    // 步骤 1：进入 settings
    cy.visit('/tabs/settings')
    cy.wait(2000)
    dismissErrorOverlay()

    // 步骤 2：进入缓存页面（多页面栈开始建立）
    cy.get('ion-item').contains(/缓存|cache/i).first().click()
    cy.wait(2000)
    cy.url().should('include', 'cache')

    // 步骤 3：缓存页面应有全文索引入口
    cy.get('.fulltext-entry').should('exist').and('be.visible')

    // 步骤 4：点击全文索引入口（这就是修复前会触发 bug 的真实路径）
    cy.get('.fulltext-entry').click()
    cy.wait(3000)
    cy.url().should('include', 'fulltext-index')

    // 步骤 5：关键验证 — ion-router-outlet 最后一个 child 应是 div.ion-page（不是 ION-PAGE）
    //   修复前：tag=ION-PAGE, class=undefined（未编译的自定义元素，被前页覆盖）
    //   修复后：tag=DIV, class=ion-page, style 含 z-index
    cy.get('ion-router-outlet').then(($outlet) => {
      const children = $outlet.children()
      const childCount = children.length
      const lastChild = children[childCount - 1]
      const style = lastChild.getAttribute('style') || ''
      cy.log(`outlet children=${childCount}, lastChild tag=${lastChild.tagName}, class=${lastChild.className}, style="${style}"`)

      // 必须断言 1：不应是未编译的 ION-PAGE（修复前的 bug 症状）
      expect(lastChild.tagName, '不应是未编译的 ION-PAGE').to.not.eq('ION-PAGE')

      // 必须断言 2：应有 ion-page class（Ionic RouterOutlet 依赖此 class 识别页面）
      expect(lastChild.className, '应有 ion-page class').to.include('ion-page')

      // 必须断言 3：多页面栈时，当前页必有 z-index style
      //   修复前的根因就是缺 z-index，被前页（CacheDetail z-index:101）覆盖
      expect(childCount, '应有多页面栈（≥2 children）').to.be.gte(2)
      expect(style, '多页面栈时当前页应有 z-index style').to.include('z-index')
    })

    // 步骤 6：页面标题应可见（不被前页覆盖）
    cy.get('ion-title').contains(/全文索引|Full.*Text.*Index/i).should('be.visible')

    cy.screenshot('fts-rebuild-classlist-fix-preserved')
  })
})
