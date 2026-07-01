/**
 * 数据库引擎性能对比 E2E 测试
 *
 * 测试场景：
 *   - 批量创建任务（CreateBatch）的 API 响应时间
 *   - 分别在 SQLite 和 Turso 引擎下运行
 *   - 对比两种引擎的性能差异
 *
 * 运行方式：
 *   - 启动后端服务，配置对应的数据库引擎
 *   - CYPRESS_API_BASE=http://localhost:2025 pnpm exec cypress run --e2e --spec "cypress/e2e/db-engine-perf.cy.ts"
 *
 * 预期：
 *   - Turso 引擎（MVCC 并发写）性能显著优于 SQLite（单写者锁）
 *   - 批量任务越大，Turso 的优势越明显
 */

describe('数据库引擎性能对比测试', () => {
  const API_BASE = Cypress.env('apiBase') || 'http://localhost:2025'

  function genBatchSpecs(count: number) {
    const specs = []
    for (let i = 0; i < count; i++) {
      specs.push({
        type: 'encrypt',
        sourcePath: `/perf-test/source-${i}.mp4`,
        targetPath: `/perf-test/target-${i}.encv`,
        password: 'perf-test-password',
        pluginName: 'ffmpeg-encryption',
        extraFields: {
          cipherMode: 'aes-256-gcm',
          compressionMode: 'zstd',
        },
        cipherMode: 0,
        compressionMode: 'zstd',
        version: 4,
      })
    }
    return specs
  }

  function measureBatchCreate(count: number, label: string) {
    const specs = genBatchSpecs(count)
    const startTime = performance.now()

    return cy.request({
      method: 'POST',
      url: `${API_BASE}/api/tasks/batch`,
      body: {
        specs,
        runId: `perf-${label}-${count}-${Date.now()}`,
        triggeredBy: 'automation',
      },
      timeout: 60000,
    }).then((res) => {
      const duration = performance.now() - startTime
      const perTask = duration / count
      cy.log(`**${label} - ${count} tasks:** ${duration.toFixed(0)}ms total, ${perTask.toFixed(2)}ms/task`)
      return { total: duration, perTask, count, tasks: res.body.length }
    })
  }

  function getDbInfo() {
    return cy.request('GET', `${API_BASE}/api/database/info`).then((res) => res.body)
  }

  function clearAllTasks() {
    return cy.request('DELETE', `${API_BASE}/api/tasks?all=true`).catch(() => {})
  }

  before(() => {
    // 先获取当前数据库引擎信息
    getDbInfo().then((info) => {
      cy.log(`当前数据库引擎: ${info.engine}, 并发度: ${info.concurrency}`)
    })
  })

  beforeEach(() => {
    clearAllTasks()
    cy.wait(500)
  })

  // ==========================================================================
  // 测试 1：小批量（100 任务）
  // ==========================================================================
  it('批量创建 100 个任务的性能', () => {
    measureBatchCreate(100, 'small-batch').then((result) => {
      expect(result.tasks).to.equal(100)
      // 性能断言：单任务平均耗时不应过长
      // 注意：具体阈值取决于运行环境，这里只做合理性检查
      expect(result.perTask).to.be.lessThan(500) // 单任务 < 500ms
    })
  })

  // ==========================================================================
  // 测试 2：中批量（500 任务）
  // ==========================================================================
  it('批量创建 500 个任务的性能', () => {
    measureBatchCreate(500, 'medium-batch').then((result) => {
      expect(result.tasks).to.equal(500)
      expect(result.perTask).to.be.lessThan(300) // 单任务 < 300ms（批量越大摊得越薄）
    })
  })

  // ==========================================================================
  // 测试 3：大批量（1000 任务）
  // ==========================================================================
  it('批量创建 1000 个任务的性能', () => {
    measureBatchCreate(1000, 'large-batch').then((result) => {
      expect(result.tasks).to.equal(1000)
      expect(result.perTask).to.be.lessThan(200) // 单任务 < 200ms
    })
  })

  // ==========================================================================
  // 测试 4：连续 3 次大批量（验证稳定性）
  // ==========================================================================
  it('连续 3 次批量创建 500 任务的稳定性', () => {
    const results: number[] = []

    cy.wrap(null).then(async function run() {
      for (let i = 0; i < 3; i++) {
        const specs = genBatchSpecs(500)
        const start = performance.now()
        await new Cypress.Promise((resolve) => {
          cy.request({
            method: 'POST',
            url: `${API_BASE}/api/tasks/batch`,
            body: {
              specs,
              runId: `perf-stability-${i}-${Date.now()}`,
              triggeredBy: 'automation',
            },
            timeout: 60000,
          }).then(() => {
            const dur = performance.now() - start
            results.push(dur)
            cy.log(`第 ${i + 1} 次: ${dur.toFixed(0)}ms`)
            resolve(null)
          })
        })
        // 清理
        await new Cypress.Promise((resolve) => {
          clearAllTasks().then(() => resolve(null))
        })
      }
    })

    cy.then(() => {
      expect(results.length).to.equal(3)
      // 3 次运行的变异系数不应过大（稳定性）
      const avg = results.reduce((a, b) => a + b, 0) / 3
      const variance = results.reduce((sum, v) => sum + Math.pow(v - avg, 2), 0) / 3
      const stdDev = Math.sqrt(variance)
      const cv = stdDev / avg
      cy.log(`平均耗时: ${avg.toFixed(0)}ms, 标准差: ${stdDev.toFixed(0)}ms, 变异系数: ${(cv * 100).toFixed(1)}%`)
      expect(cv).to.be.lessThan(0.5) // 变异系数 < 50% 就算稳定
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
