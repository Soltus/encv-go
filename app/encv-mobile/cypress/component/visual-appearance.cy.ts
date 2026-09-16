/**
 * 视觉回归示例（Phase 4 主题迁移截图比对基）
 *
 * 用法：
 *   - 首次跑：写 baseline 到 cypress/visual/baseline/，test 判 matched（firstRun）
 *   - 之后跑：pixelmatch 对比 candidate 与 baseline，mismatch > threshold 即 diff
 *     图落到 cypress/visual/diff/，并让 test fail（视觉回归）
 *
 * 阈值为 0.1（10% 像素容差），平衡抗锯齿抖动与回归灵敏度。
 *
 * 注意：AppearanceDetail 的 composable 在挂载后即把主题写到
 *   document.documentElement[data-theme]，故切主题只需改该属性再截图。
 */
import AppearanceDetail from '@/views/AppearanceDetail.vue'

const THEME_SHOT: Array<{ id: string; dataTheme: string | null }> = [
  { id: 'appearance-default', dataTheme: null },
  { id: 'appearance-sunset', dataTheme: 'sunset' },
  { id: 'appearance-mint', dataTheme: 'mint' },
]

describe('Appearance 视觉回归（主题切换）', () => {
  THEME_SHOT.forEach(({ id, dataTheme }) => {
    it(`截图对比: ${id}`, () => {
      cy.mount(AppearanceDetail)
      cy.get('ion-content').should('exist')
      cy.get('body').then(($body) => {
        if (dataTheme) {
          $body[0].ownerDocument.documentElement.setAttribute('data-theme', dataTheme)
        } else {
          $body[0].ownerDocument.documentElement.removeAttribute('data-theme')
        }
      })
      // 等 IonContent 渲染 + 主题令牌生效
      cy.wait(300)
      cy.screenshot(id, { capture: 'fullPage' }).then(() => {
        cy.compareSnapshot(id, 0.1).then((res: any) => {
          cy.log(
            res.firstRun
              ? `[baseline] 已写入 ${id}`
              : `matched=${res.matched} mismatch=${res.mismatchPercent}%`,
          )
          if (!res.firstRun) {
            expect(res.matched, `视觉回归: ${id} 不匹配，diff=${res.diffPath}`).to.equal(true)
          }
        })
      })
    })
  })
})
