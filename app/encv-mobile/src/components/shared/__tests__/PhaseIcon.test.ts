/**
 * PhaseIcon 单元测试
 *
 * 覆盖 Task 10 SubTask 10.4：
 * - 9 个 Phase 值渲染对应 ion-icon
 * - 未知 phase 用 fallback（flashOutline）
 * - class 包含 phase 标识
 */
import { describe, it, expect } from 'vitest'
import { mount } from '@vue/test-utils'
import {
  searchOutline,
  settingsOutline,
  colorFilterOutline,
  lockClosedOutline,
  lockOpenOutline,
  cubeOutline,
  shieldCheckmarkOutline,
  checkmarkCircleOutline,
  flashOutline,
} from 'ionicons/icons'
import PhaseIcon from '@/components/shared/PhaseIcon.vue'
import { Phase, ALL_PHASES } from '@/lib/workflow/types'

// ion-icon stub：捕获 icon prop 供断言
const IonIconStub = {
  name: 'IonIcon',
  props: {
    icon: { type: [String, Object], default: null },
  },
  template: '<span class="ion-icon-stub" :data-icon="String(icon)" />',
}

function mountPhaseIcon(phase: Phase) {
  return mount(PhaseIcon, {
    props: { phase },
    global: {
      stubs: { 'ion-icon': IonIconStub },
    },
  })
}

// Phase → 期望的 ionicon 图标常量
const EXPECTED_ICON_MAP: Record<Phase, string> = {
  [Phase.Created]: flashOutline,
  [Phase.Analyzing]: searchOutline,
  [Phase.Initializing]: settingsOutline,
  [Phase.Preprocessing]: colorFilterOutline,
  [Phase.Encrypting]: lockClosedOutline,
  [Phase.Decrypting]: lockOpenOutline,
  [Phase.Packing]: cubeOutline,
  [Phase.Verifying]: shieldCheckmarkOutline,
  [Phase.Completed]: checkmarkCircleOutline,
}

describe('PhaseIcon - 9 个 Phase 值渲染对应 ion-icon', () => {
  it.each(ALL_PHASES.map((p) => [p, p] as const))(
    'Phase.%s 渲染对应的 ion-icon',
    (phase) => {
      const wrapper = mountPhaseIcon(phase)
      const iconEl = wrapper.find('.ion-icon-stub')
      expect(iconEl.exists()).toBe(true)
      expect(iconEl.attributes('data-icon')).toBe(String(EXPECTED_ICON_MAP[phase]))
    },
  )
})

describe('PhaseIcon - class 包含 phase 标识', () => {
  it.each(ALL_PHASES.map((p) => [p, p] as const))(
    'Phase.%s 渲染的 ion-icon 包含 phase-icon--%s class',
    (phase) => {
      const wrapper = mountPhaseIcon(phase)
      const iconEl = wrapper.find('.ion-icon-stub')
      expect(iconEl.classes()).toContain('phase-icon')
      expect(iconEl.classes()).toContain(`phase-icon--${phase}`)
    },
  )
})

describe('PhaseIcon - fallback 处理', () => {
  it('未知 phase 值（强转）使用 flashOutline 作为 fallback', () => {
    // 模拟后端推送了未识别的 phase 字符串（强转为 Phase 类型）
    const wrapper = mountPhaseIcon('unknown_phase' as unknown as Phase)
    const iconEl = wrapper.find('.ion-icon-stub')
    expect(iconEl.attributes('data-icon')).toBe(String(flashOutline))
  })

  it('空字符串 phase 使用 fallback', () => {
    const wrapper = mountPhaseIcon('' as unknown as Phase)
    const iconEl = wrapper.find('.ion-icon-stub')
    expect(iconEl.attributes('data-icon')).toBe(String(flashOutline))
  })
})

describe('PhaseIcon - 基础渲染', () => {
  it('渲染单个 ion-icon 元素', () => {
    const wrapper = mountPhaseIcon(Phase.Created)
    expect(wrapper.findAll('.ion-icon-stub')).toHaveLength(1)
  })

  it('phase prop 变化时 icon 跟随更新', async () => {
    const wrapper = mountPhaseIcon(Phase.Created)
    expect(wrapper.find('.ion-icon-stub').attributes('data-icon')).toBe(
      String(flashOutline),
    )
    await wrapper.setProps({ phase: Phase.Encrypting })
    expect(wrapper.find('.ion-icon-stub').attributes('data-icon')).toBe(
      String(lockClosedOutline),
    )
  })
})
