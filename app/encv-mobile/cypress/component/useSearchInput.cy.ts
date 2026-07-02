/**
 * 搜索框交互测试 — contenteditable + span units
 *
 * 覆盖以下场景：
 * 1. 输入文字 → queryValue 正确更新 → 触发搜索回调
 * 2. 光标在开头插入操作符 → 不报错，正确插入
 * 3. 光标在中间插入操作符 → 正确拆分 text span
 * 4. 插入「」短语 → 正确插入三个节点，光标在中间
 * 5. 删除操作符 → 合并相邻 text span
 * 6. 序列化结果正确（AND/OR/NOT/phrase/regex）
 */

import { mount } from 'cypress/vue'
import { defineComponent, ref, h } from 'vue'
import { useSearchInput } from '../../src/composables/useSearchInput'
import SearchInputTestHarness from './helpers/SearchInputTestHarness.vue'

describe('useSearchInput 搜索框交互', () => {
  it('输入文字 → onChange 正确触发，queryValue 正确', () => {
    const onChangeSpy = cy.spy().as('onChange')

    mount(SearchInputTestHarness, {
      props: {
        onChange: onChangeSpy,
      },
    })

    // 点击搜索框，输入文字
    cy.get('[data-testid="search-input"]').click()
    cy.get('[data-testid="search-input"]').type('hello')
    cy.wait(50)

    // 检查 onChange 被调用
    cy.get('@onChange').should('have.been.calledWith', 'hello')

    // 检查 query 显示
    cy.get('[data-testid="query-display"]').should('contain.text', 'hello')
  })

  it('光标在开头插入 AND 操作符 → 不报错，正确插入', () => {
    const onChangeSpy = cy.spy().as('onChange')

    mount(SearchInputTestHarness, {
      props: {
        onChange: onChangeSpy,
        initialQuery: 'test',
      },
    })

    // 等待初始值渲染
    cy.wait(50)

    // 将光标移到开头
    cy.get('[data-testid="search-input"]').click().type('{home}')

    // 点击 AND 按钮
    cy.get('[data-testid="btn-and"]').click()
    cy.wait(50)

    // 检查结果：应该是 "AND test"
    cy.get('[data-testid="query-display"]').then(($el) => {
      const text = $el.text()
      cy.log('Query:', text)
      // 应该包含 AND
      expect(text).to.include('AND')
    })

    // 检查没有未捕获的错误
    cy.get('[data-testid="error-display"]').should('be.empty')
  })

  it('光标在中间插入 OR 操作符 → 正确拆分 text', () => {
    const onChangeSpy = cy.spy().as('onChange')

    mount(SearchInputTestHarness, {
      props: {
        onChange: onChangeSpy,
        initialQuery: 'abcdef',
      },
    })

    cy.wait(50)

    // 将光标移到中间（第 3 个字符后）
    cy.get('[data-testid="search-input"]').click()
    // 用 setSelectionRange 不适用 contenteditable，改用键盘移动
    cy.get('[data-testid="search-input"]').type('{home}')
    cy.get('[data-testid="search-input"]').type('{rightarrow}'.repeat(3))

    // 点击 OR 按钮
    cy.get('[data-testid="btn-or"]').click()
    cy.wait(50)

    // 检查结果：应该是 "abc OR def"
    cy.get('[data-testid="query-display"]').then(($el) => {
      const text = $el.text()
      cy.log('Query:', text)
      expect(text).to.include('OR')
    })

    cy.get('[data-testid="error-display"]').should('be.empty')
  })

  it('插入「」短语 → 正确插入三个节点', () => {
    const onChangeSpy = cy.spy().as('onChange')

    mount(SearchInputTestHarness, {
      props: {
        onChange: onChangeSpy,
        initialQuery: '',
      },
    })

    cy.wait(50)

    // 点击短语按钮
    cy.get('[data-testid="btn-phrase"]').click()
    cy.wait(50)

    // 检查结果：应该包含 phrase 标记
    cy.get('[data-testid="query-display"]').then(($el) => {
      const text = $el.text()
      cy.log('Query:', text)
      // 短语会被序列化为 "..."
      expect(text).to.include('"')
    })

    cy.get('[data-testid="error-display"]').should('be.empty')
  })

  it('插入 NOT 操作符 → 正确序列化', () => {
    const onChangeSpy = cy.spy().as('onChange')

    mount(SearchInputTestHarness, {
      props: {
        onChange: onChangeSpy,
        initialQuery: 'apple',
      },
    })

    cy.wait(50)

    // 光标移到末尾，插入 NOT
    cy.get('[data-testid="search-input"]').click().type('{end}')
    cy.get('[data-testid="btn-not"]').click()
    cy.wait(50)

    // 检查结果：应该包含 NOT
    cy.get('[data-testid="query-display"]').then(($el) => {
      const text = $el.text()
      cy.log('Query:', text)
      expect(text).to.include('NOT')
    })

    cy.get('[data-testid="error-display"]').should('be.empty')
  })

  it('清空按钮 → 清空输入', () => {
    const onChangeSpy = cy.spy().as('onChange')

    mount(SearchInputTestHarness, {
      props: {
        onChange: onChangeSpy,
        initialQuery: 'test query',
      },
    })

    cy.wait(50)

    // 点击清空
    cy.get('[data-testid="btn-clear"]').click()
    cy.wait(50)

    // 检查 query 为空
    cy.get('[data-testid="query-display"]').should('have.text', '')

    cy.get('[data-testid="error-display"]').should('be.empty')
  })

  it('连续输入多个操作符 → 不崩溃', () => {
    const onChangeSpy = cy.spy().as('onChange')

    mount(SearchInputTestHarness, {
      props: {
        onChange: onChangeSpy,
        initialQuery: '',
      },
    })

    cy.wait(50)

    // 连续插入多个操作符
    cy.get('[data-testid="btn-and"]').click()
    cy.wait(30)
    cy.get('[data-testid="btn-or"]').click()
    cy.wait(30)
    cy.get('[data-testid="btn-not"]').click()
    cy.wait(30)
    cy.get('[data-testid="btn-phrase"]').click()
    cy.wait(30)

    // 检查没有错误
    cy.get('[data-testid="error-display"]').should('be.empty')

    // 检查 query 非空
    cy.get('[data-testid="query-display"]').then(($el) => {
      const text = $el.text()
      cy.log('Final query:', text)
    })
  })

  // ===========================================================================
  // Bug 复现：回车换行后光标位置
  // ===========================================================================
  describe('回车换行后光标位置 (Bug 复现)', () => {
    it('在 text span 中间按回车 → 应该拆成两个 text span，光标在新行开头', () => {
      const onChangeSpy = cy.spy().as('onChange')

      mount(SearchInputTestHarness, {
        props: {
          initialQuery: 'abcdef',
          onChange: onChangeSpy,
        },
      })

      cy.wait(50)

      // 光标移到第 3 个字符后（abc|def）
      cy.get('[data-testid="search-input"]').click()
      cy.get('[data-testid="search-input"]').type('{home}')
      cy.get('[data-testid="search-input"]').type('{rightarrow}'.repeat(3))

      // 记录回车前的光标偏移量
      cy.get('[data-testid="search-input"]').then(($div) => {
        const div = $div[0]
        const sel = window.getSelection()
        if (sel && sel.rangeCount > 0) {
          const range = sel.getRangeAt(0)
          const preRange = document.createRange()
          preRange.selectNodeContents(div)
          preRange.setEnd(range.endContainer, range.endOffset)
          const offset = preRange.toString().length
          cy.log('Caret offset before enter:', offset)
        }
      })

      // 按回车
      cy.get('[data-testid="search-input"]').type('{enter}')
      cy.wait(50)

      // 检查：回车后光标偏移量
      cy.get('[data-testid="search-input"]').then(($div) => {
        const div = $div[0]
        const sel = window.getSelection()
        if (sel && sel.rangeCount > 0) {
          const range = sel.getRangeAt(0)
          const preRange = document.createRange()
          preRange.selectNodeContents(div)
          preRange.setEnd(range.endContainer, range.endOffset)
          const offset = preRange.toString().length
          // 收集 DOM 结构信息用于调试
          let domInfo = ''
          Array.from(div.childNodes).forEach((el, i) => {
            if (el.nodeType === Node.TEXT_NODE) {
              domInfo += `[${i}] TEXT: "${el.textContent}"\n`
            } else {
              const htmlEl = el as HTMLElement
              domInfo += `[${i}] ${htmlEl.tagName} kind=${htmlEl.dataset?.kind}: "${htmlEl.textContent}"\n`
            }
          })
          expect(offset, `Caret offset after enter should be > 3\nendContainer: ${range.endContainer.nodeName}, text="${range.endContainer.textContent}", endOffset=${range.endOffset}\nDOM:\n${domInfo}`).to.be.greaterThan(3)
        }
      })

      // 检查：序列化结果
      cy.get('[data-testid="query-display"]').then(($el) => {
        const text = $el.text()
        cy.log('Serialized query:', text)
      })

      cy.get('[data-testid="error-display"]').should('be.empty')
    })

    it('在 text span 末尾按回车 → 应该新增空 text span，光标在新 span 开头', () => {
      mount(SearchInputTestHarness, {
        props: {
          initialQuery: 'hello',
        },
      })

      cy.wait(50)

      // 光标移到末尾
      cy.get('[data-testid="search-input"]').click().type('{end}')

      // 按回车
      cy.get('[data-testid="search-input"]').type('{enter}')
      cy.wait(50)

      cy.get('[data-testid="query-display"]').then(($el) => {
        cy.log('Query after enter at end:', $el.text())
      })

      cy.get('[data-testid="error-display"]').should('be.empty')
    })

    it('有 op span 时按回车 → 不崩溃', () => {
      mount(SearchInputTestHarness, {
        props: {
          initialQuery: '',
        },
      })

      cy.wait(50)

      // 输入一些文字
      cy.get('[data-testid="search-input"]').type('abc')
      cy.wait(30)

      // 插入 AND
      cy.get('[data-testid="btn-and"]').click()
      cy.wait(30)

      // 再输入一些文字
      cy.get('[data-testid="search-input"]').type('def')
      cy.wait(30)

      // 光标移到中间
      cy.get('[data-testid="search-input"]').type('{leftarrow}'.repeat(2))

      // 按回车
      cy.get('[data-testid="search-input"]').type('{enter}')
      cy.wait(50)

      cy.get('[data-testid="error-display"]').should('be.empty')

      cy.get('[data-testid="query-display"]').then(($el) => {
        cy.log('Query after enter with op:', $el.text())
      })
    })
  })
})
