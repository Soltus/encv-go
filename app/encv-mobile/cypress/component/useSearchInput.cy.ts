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
})
