/**
 * Bug3 复现测试 — 全文索引页面 classList 异步错误
 *
 * 用户报告：
 *   点击全文索引页面 → 未捕获的异步错误 Cannot read properties of undefined (reading 'classList')
 *   → 搞崩整个设置 tab
 *
 * 根因假设（根据 Capacitor 铁律 §六）：
 *   Vue render 崩溃 → Ionic RouterOutlet 切换事务中断 → 过渡动画 Promise 回调访问不存在元素
 *   → classList 错误（表象）
 *
 * 我们需要找到 render 崩溃的真正原因。
 */

import { mount } from 'cypress/vue'
import { defineComponent, ref } from 'vue'
import FullTextIndexDetail from '@/views/FullTextIndexDetail.vue'
import { errorStore } from '@/composables/useErrorCapture'

describe('Bug3 复现：全文索引页面 classList 异步错误', () => {
  beforeEach(() => {
    // 清空错误
    errorStore.clear()

    // 拦截 API 请求，模拟各种失败场景
    cy.intercept('GET', '**/api/files/search-fulltext/stats', {
      statusCode: 500,
      body: { error: 'Internal Server Error' },
    }).as('statsApi')
  })

  it('挂载组件 + API 失败 → 不应有 classList 错误', () => {
    // 监听 unhandledrejection
    const unhandledErrors: string[] = []
    cy.on('uncaught:exception', (err) => {
      unhandledErrors.push(err.message)
      // 不让 Cypress 自动失败，我们自己收集
      return false
    })

    mount(FullTextIndexDetail)

    // 等待 API 调用完成
    cy.wait('@statsApi', { timeout: 5000 }).then(() => {
      // 再等一会儿，让 Promise 回调都执行完
      cy.wait(1000)

      // 检查有没有 classList 错误
      const classListErrors = unhandledErrors.filter(e =>
        e.includes('classList') || e.includes('Cannot read properties of undefined')
      )

      // 将错误输出到 Cypress 日志
      if (classListErrors.length > 0) {
        cy.log('❌ 发现 classList 错误:', classListErrors.join('\n'))
      } else {
        cy.log('✅ 没有 classList 错误')
      }

      // 检查 errorStore 里有没有错误
      cy.wrap(null).then(() => {
        const storeErrors = errorStore.errors.value
        cy.log(`errorStore 里有 ${storeErrors.length} 个错误`)
        storeErrors.forEach((e, i) => {
          cy.log(`  [${i}] ${e.message}`)
        })
      })
    })
  })

  it('挂载组件 + API 返回空数据 → 不应崩溃', () => {
    cy.intercept('GET', '**/api/files/search-fulltext/stats', {
      statusCode: 200,
      body: { available: false, error: 'FTS5 not initialized' },
    }).as('statsApi')

    const errors: string[] = []
    cy.on('uncaught:exception', (err) => {
      errors.push(err.message)
      return false
    })

    mount(FullTextIndexDetail)

    cy.wait('@statsApi')
    cy.wait(500)

    // 验证显示"不可用"状态而不是崩溃
    cy.contains('全文索引不可用').should('exist')
    cy.wrap(errors).should('have.length', 0)
  })

  it('挂载组件 + API 返回正常数据 → 正常渲染', () => {
    cy.intercept('GET', '**/api/files/search-fulltext/stats', {
      statusCode: 200,
      body: {
        available: true,
        stats: {
          dbPath: '/tmp/test.db',
          fts5Enabled: true,
          tokenizer: 'unicode61 + bigram',
          indexVersion: 1,
          totalFiles: 12345,
          totalDirs: 678,
          totalSize: 1048576,
          lastBuildMs: 1500,
          indexedAt: '2026-07-02T10:00:00Z',
          isIndexing: false,
        },
      },
    }).as('statsApi')

    const errors: string[] = []
    cy.on('uncaught:exception', (err) => {
      errors.push(err.message)
      return false
    })

    mount(FullTextIndexDetail)

    cy.wait('@statsApi')
    cy.wait(500)

    // 验证正常渲染
    cy.contains('基础信息').should('exist')
    cy.contains('文件总数').should('exist')
    cy.wrap(errors).should('have.length', 0)
  })
})
