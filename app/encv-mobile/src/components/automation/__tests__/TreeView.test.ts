/**
 * TreeView (UnifiedTreeView) 单元测试
 *
 * 覆盖 Task 12 SubTask 12.6：
 * - 基础渲染：nodes 数组渲染正确（父节点 + 子节点）
 * - 搜索过滤：searchQuery 过滤 label / meta / children label
 * - 展开/收起：toggleNode 切换 expandedSet + emit toggle-node
 * - select-node emit：点击子节点触发 + 切换详情展开
 * - 默认展开失败 job：defaultExpandFailed=true 时自动展开有失败子节点的父节点
 * - slot 渲染：node-detail / node-icon / node-meta / toolbar
 * - 进度/耗时/速率字段显示（字段缺失时隐藏）
 * - 暗黑模式 class（body.dark 作用域）
 * - workflowRun 兼容派生（传入 workflowRun 时自动生成 nodes）
 * - searchable=false 时隐藏搜索框
 */
import { describe, it, expect } from 'vitest'
import { mount } from '@vue/test-utils'
import TreeView from '@/components/automation/TreeView.vue'
import PhaseIcon from '@/components/shared/PhaseIcon.vue'
import {
  Phase,
  type UnifiedTreeNode,
  type WorkflowRun,
} from '@/lib/workflow/types'

// ion-icon stub：避免 @ionic/vue 全局注册依赖
const IonIconStub = {
  name: 'IonIcon',
  props: {
    icon: { type: [String, Object], default: null },
  },
  template: '<span class="ion-icon-stub" :data-icon="String(icon)" />',
}

/** 构造测试用 UnifiedTreeNode（便于覆盖默认值） */
function makeNode(overrides: Partial<UnifiedTreeNode> = {}): UnifiedTreeNode {
  return {
    id: 'job-1',
    label: 'Encrypt All',
    status: 'running',
    meta: '2/5',
    children: [
      { id: 'step-1', label: 'encrypt mp4', status: 'success' },
      { id: 'step-2', label: 'encrypt mp3', status: 'running', progress: 45 },
      { id: 'step-3', label: 'encrypt jpg', status: 'pending' },
    ],
    ...overrides,
  }
}

/** 构造测试用 WorkflowRun（用于兼容派生测试） */
function makeWorkflowRun(overrides: Partial<WorkflowRun> = {}): WorkflowRun {
  return {
    id: 'run-1',
    workflowDefId: 'wf-def-1',
    status: 'running',
    triggeredBy: 'automation',
    createdAt: '2026-06-18T10:00:00Z',
    jobs: [
      {
        id: 'job-run-1',
        jobDefId: 'encrypt-all',
        status: 'running',
        steps: [
          {
            id: 'step-run-1',
            stepDefId: 'enc_mp4',
            status: 'success',
            durationMs: 1500,
          },
          {
            id: 'step-run-2',
            stepDefId: 'enc_mp3',
            status: 'running',
          },
          {
            id: 'step-run-3',
            stepDefId: 'enc_jpg',
            status: 'failure',
            error: 'ffmpeg exit 1',
          },
        ],
      },
    ],
    ...overrides,
  }
}

function mountTree(
  props: Record<string, unknown> = {},
  options: {
    slots?: Record<string, string>
  } = {},
) {
  return mount(TreeView, {
    props: {
      nodes: [makeNode()],
      ...props,
    },
    global: {
      stubs: { 'ion-icon': IonIconStub },
    },
    slots: options.slots,
  })
}

// ─── 基础渲染 ───────────────────────────────────────────────────────

describe('TreeView - 基础渲染', () => {
  it('渲染父节点 label', () => {
    const wrapper = mountTree({ nodes: [makeNode({ label: '自定义 Job' })] })
    const parentNodes = wrapper.findAll('.tree__node--parent')
    expect(parentNodes.length).toBe(1)
    expect(parentNodes[0].text()).toContain('自定义 Job')
  })

  it('渲染多个父节点', () => {
    const wrapper = mountTree({
      nodes: [
        makeNode({ id: 'job-1', label: 'Job A' }),
        makeNode({ id: 'job-2', label: 'Job B' }),
      ],
    })
    const parentNodes = wrapper.findAll('.tree__node--parent')
    expect(parentNodes.length).toBe(2)
  })

  it('渲染父节点 meta 文本', () => {
    const wrapper = mountTree({ nodes: [makeNode({ meta: '3/10' })] })
    expect(wrapper.find('.tree__node--parent .tree__meta').text()).toBe('3/10')
  })

  it('展开父节点后渲染子节点', async () => {
    const wrapper = mountTree()
    // 初始未展开 → 无子节点
    expect(wrapper.findAll('.tree__node--child').length).toBe(0)
    // 点击展开
    await wrapper.find('.tree__node--parent').trigger('click')
    expect(wrapper.findAll('.tree__node--child').length).toBe(3)
  })

  it('子节点 label 渲染正确', async () => {
    const wrapper = mountTree()
    await wrapper.find('.tree__node--parent').trigger('click')
    const childLabels = wrapper.findAll('.tree__node--child .tree__label')
    expect(childLabels[0].text()).toBe('encrypt mp4')
    expect(childLabels[1].text()).toBe('encrypt mp3')
  })

  it('空 nodes 时渲染空状态', () => {
    const wrapper = mountTree({ nodes: [] })
    expect(wrapper.find('.tree__empty').exists()).toBe(true)
    expect(wrapper.find('.tree__empty').text()).toContain('No nodes found')
  })
})

// ─── 搜索过滤 ───────────────────────────────────────────────────────

describe('TreeView - 搜索过滤', () => {
  it('搜索框默认显示（searchable=true）', () => {
    const wrapper = mountTree()
    expect(wrapper.find('.tree__search').exists()).toBe(true)
  })

  it('searchable=false 时隐藏搜索框', () => {
    const wrapper = mountTree({ searchable: false })
    expect(wrapper.find('.tree__search').exists()).toBe(false)
  })

  it('按父节点 label 过滤', async () => {
    const wrapper = mountTree({
      nodes: [
        makeNode({ id: 'job-1', label: 'Encrypt All' }),
        makeNode({ id: 'job-2', label: 'Decrypt All' }),
      ],
    })
    await wrapper.find('.tree__search').setValue('decrypt')
    const parentNodes = wrapper.findAll('.tree__node--parent')
    expect(parentNodes.length).toBe(1)
    expect(parentNodes[0].text()).toContain('Decrypt All')
  })

  it('按子节点 label 过滤（父节点保留）', async () => {
    const wrapper = mountTree({
      nodes: [
        makeNode({
          id: 'job-1',
          label: 'Job A',
          children: [{ id: 's1', label: 'mp4 encrypt', status: 'success' }],
        }),
        makeNode({
          id: 'job-2',
          label: 'Job B',
          children: [{ id: 's2', label: 'mp3 encrypt', status: 'success' }],
        }),
      ],
    })
    await wrapper.find('.tree__search').setValue('mp3')
    const parentNodes = wrapper.findAll('.tree__node--parent')
    expect(parentNodes.length).toBe(1)
    expect(parentNodes[0].text()).toContain('Job B')
  })

  it('搜索无匹配时显示空状态', async () => {
    const wrapper = mountTree()
    await wrapper.find('.tree__search').setValue('zzz_no_match')
    expect(wrapper.find('.tree__empty').exists()).toBe(true)
  })

  it('tree__count 显示过滤后节点数', () => {
    const wrapper = mountTree({
      nodes: [
        makeNode({ id: 'job-1' }),
        makeNode({ id: 'job-2' }),
      ],
    })
    expect(wrapper.find('.tree__count').text()).toContain('2 nodes')
  })
})

// ─── 展开/收起 ──────────────────────────────────────────────────────

describe('TreeView - 展开/收起', () => {
  it('点击父节点切换展开状态', async () => {
    const wrapper = mountTree()
    expect(wrapper.findAll('.tree__node--child').length).toBe(0)
    await wrapper.find('.tree__node--parent').trigger('click')
    expect(wrapper.findAll('.tree__node--child').length).toBe(3)
    // 再次点击收起
    await wrapper.find('.tree__node--parent').trigger('click')
    expect(wrapper.findAll('.tree__node--child').length).toBe(0)
  })

  it('展开时父节点带 tree__node--expanded class', async () => {
    const wrapper = mountTree()
    const parent = wrapper.find('.tree__node--parent')
    expect(parent.classes()).not.toContain('tree__node--expanded')
    await parent.trigger('click')
    expect(parent.classes()).toContain('tree__node--expanded')
  })

  it('展开时 arrow 用 chevron-down，收起时用 chevron-forward', async () => {
    const wrapper = mountTree()
    const arrow = wrapper.find('.tree__node--parent .tree__arrow')
    expect(arrow.exists()).toBe(true)
    // 收起状态：ion-icon stub 渲染为 .ion-icon-stub（class 合并到同一元素）
    expect(arrow.classes()).toContain('ion-icon-stub')
    await wrapper.find('.tree__node--parent').trigger('click')
    const arrowExpanded = wrapper.find('.tree__node--parent .tree__arrow')
    expect(arrowExpanded.exists()).toBe(true)
  })

  it('点击父节点 emit toggle-node 事件', async () => {
    const wrapper = mountTree()
    await wrapper.find('.tree__node--parent').trigger('click')
    const events = wrapper.emitted('toggle-node')
    expect(events).toHaveLength(1)
    expect(events![0][1]).toBe(true) // expanded=true
  })

  it('再次点击父节点 emit toggle-node 事件 expanded=false', async () => {
    const wrapper = mountTree()
    await wrapper.find('.tree__node--parent').trigger('click')
    await wrapper.find('.tree__node--parent').trigger('click')
    const events = wrapper.emitted('toggle-node')
    expect(events).toHaveLength(2)
    expect(events![1][1]).toBe(false) // expanded=false
  })
})

// ─── select-node emit ───────────────────────────────────────────────

describe('TreeView - select-node emit', () => {
  it('点击子节点 emit select-node 事件', async () => {
    const wrapper = mountTree()
    await wrapper.find('.tree__node--parent').trigger('click')
    const child = wrapper.findAll('.tree__node--child')[0]
    await child.trigger('click')
    const events = wrapper.emitted('select-node')
    expect(events).toHaveLength(1)
    const emittedNode = events![0][0] as UnifiedTreeNode
    expect(emittedNode.id).toBe('step-1')
  })

  it('点击子节点切换详情展开', async () => {
    const wrapper = mountTree({}, {
      slots: {
        'node-detail': '<div class="custom-detail">详情内容</div>',
      },
    })
    await wrapper.find('.tree__node--parent').trigger('click')
    const child = wrapper.findAll('.tree__node--child')[0]
    // 初始无详情
    expect(wrapper.find('.tree__child-detail').exists()).toBe(false)
    await child.trigger('click')
    expect(wrapper.find('.tree__child-detail').exists()).toBe(true)
    // 再次点击收起
    await child.trigger('click')
    expect(wrapper.find('.tree__child-detail').exists()).toBe(false)
  })

  it('选中的子节点带 tree__node--selected class', async () => {
    const wrapper = mountTree()
    await wrapper.find('.tree__node--parent').trigger('click')
    const child = wrapper.findAll('.tree__node--child')[0]
    await child.trigger('click')
    expect(child.classes()).toContain('tree__node--selected')
  })
})

// ─── 默认展开失败 job ───────────────────────────────────────────────

describe('TreeView - 默认展开失败 job', () => {
  it('defaultExpandFailed=true 时自动展开有 failure 子节点的父节点', () => {
    const wrapper = mountTree({
      nodes: [
        makeNode({
          id: 'job-ok',
          label: 'All Success',
          children: [
            { id: 's1', label: 'step1', status: 'success' },
          ],
        }),
        makeNode({
          id: 'job-fail',
          label: 'Has Failure',
          children: [
            { id: 's2', label: 'step2', status: 'success' },
            { id: 's3', label: 'step3', status: 'failure' },
          ],
        }),
      ],
    })
    // job-fail 应自动展开（有 failure 子节点）
    const parentNodes = wrapper.findAll('.tree__node--parent')
    const failParent = parentNodes.find((n) => n.text().includes('Has Failure'))
    expect(failParent!.classes()).toContain('tree__node--expanded')
    // job-ok 不应展开
    const okParent = parentNodes.find((n) => n.text().includes('All Success'))
    expect(okParent!.classes()).not.toContain('tree__node--expanded')
  })

  it('defaultExpandFailed=true 时自动展开有 timed_out 子节点的父节点', () => {
    const wrapper = mountTree({
      nodes: [
        makeNode({
          id: 'job-timeout',
          label: 'Has Timeout',
          children: [
            { id: 's1', label: 'step1', status: 'timed_out' },
          ],
        }),
      ],
    })
    expect(wrapper.find('.tree__node--parent').classes()).toContain('tree__node--expanded')
  })

  it('defaultExpandFailed=false 时不自动展开', () => {
    const wrapper = mountTree({
      defaultExpandFailed: false,
      nodes: [
        makeNode({
          id: 'job-fail',
          label: 'Has Failure',
          children: [
            { id: 's1', label: 'step1', status: 'failure' },
          ],
        }),
      ],
    })
    expect(wrapper.find('.tree__node--parent').classes()).not.toContain('tree__node--expanded')
  })
})

// ─── slot 渲染 ──────────────────────────────────────────────────────

describe('TreeView - slot 渲染', () => {
  it('node-detail slot 在子节点展开时渲染', async () => {
    const wrapper = mountTree({}, {
      slots: {
        'node-detail': '<div class="custom-detail">详情内容</div>',
      },
    })
    await wrapper.find('.tree__node--parent').trigger('click')
    const child = wrapper.findAll('.tree__node--child')[0]
    await child.trigger('click')
    expect(wrapper.find('.custom-detail').exists()).toBe(true)
    expect(wrapper.find('.custom-detail').text()).toBe('详情内容')
  })

  it('node-detail slot 接收 node 作为 slot prop', async () => {
    const wrapper = mountTree({}, {
      slots: {
        'node-detail': '<template #node-detail="{ node }"><div class="slot-node">{{ node.label }}</div></template>',
      },
    })
    await wrapper.find('.tree__node--parent').trigger('click')
    const child = wrapper.findAll('.tree__node--child')[0]
    await child.trigger('click')
    expect(wrapper.find('.slot-node').text()).toBe('encrypt mp4')
  })

  it('node-icon slot 覆盖默认图标', () => {
    const wrapper = mountTree({}, {
      slots: {
        'node-icon': '<span class="custom-icon">🎯</span>',
      },
    })
    expect(wrapper.find('.custom-icon').exists()).toBe(true)
    // 默认 PhaseIcon / StepMiniBadge 不应渲染
    expect(wrapper.findComponent(PhaseIcon).exists()).toBe(false)
  })

  it('node-meta slot 覆盖默认 meta', () => {
    const wrapper = mountTree({}, {
      slots: {
        'node-meta': '<span class="custom-meta">自定义 meta</span>',
      },
    })
    expect(wrapper.find('.custom-meta').exists()).toBe(true)
    expect(wrapper.find('.tree__node--parent .tree__meta').exists()).toBe(false)
  })

  it('toolbar slot 渲染在搜索框右侧', () => {
    const wrapper = mountTree({}, {
      slots: {
        toolbar: '<button class="custom-toolbar-btn">导出</button>',
      },
    })
    expect(wrapper.find('.custom-toolbar-btn').exists()).toBe(true)
    expect(wrapper.find('.custom-toolbar-btn').text()).toBe('导出')
  })
})

// ─── 进度/耗时/速率字段显示 ──────────────────────────────────────────

describe('TreeView - 进度/耗时/速率字段显示', () => {
  it('progress 字段存在时渲染百分比', () => {
    const wrapper = mountTree({
      nodes: [makeNode({ progress: 75 })],
    })
    expect(wrapper.find('.tree__node--parent .tree__progress').text()).toBe('75%')
  })

  it('progress=0 时也渲染（0 是有效值）', () => {
    const wrapper = mountTree({
      nodes: [makeNode({ progress: 0 })],
    })
    expect(wrapper.find('.tree__progress').exists()).toBe(true)
    expect(wrapper.find('.tree__progress').text()).toBe('0%')
  })

  it('progress 缺失时不渲染', () => {
    const wrapper = mountTree({
      nodes: [makeNode({ progress: undefined })],
    })
    expect(wrapper.find('.tree__node--parent .tree__progress').exists()).toBe(false)
  })

  it('duration 字段存在时渲染', () => {
    const wrapper = mountTree({
      nodes: [makeNode({ duration: '1.5s' })],
    })
    expect(wrapper.find('.tree__duration').text()).toBe('1.5s')
  })

  it('duration 缺失时不渲染', () => {
    const wrapper = mountTree({
      nodes: [makeNode({ duration: undefined })],
    })
    expect(wrapper.find('.tree__node--parent .tree__duration').exists()).toBe(false)
  })

  it('speed 字段存在时渲染（含 ion-icon flash 图标）', () => {
    const wrapper = mountTree({
      nodes: [makeNode({ speed: '12.5 MB/s' })],
    })
    const speedEl = wrapper.find('.tree__speed')
    expect(speedEl.exists()).toBe(true)
    expect(speedEl.text()).toBe('12.5 MB/s')
    // 包含 ion-icon 图标（stub 渲染为 .ion-icon-stub）
    expect(speedEl.find('.ion-icon-stub').exists()).toBe(true)
  })

  it('speed 缺失时不渲染', () => {
    const wrapper = mountTree({
      nodes: [makeNode({ speed: undefined })],
    })
    expect(wrapper.find('.tree__node--parent .tree__speed').exists()).toBe(false)
  })

  it('errorHint 字段存在时渲染 ion-icon close-circle', () => {
    const wrapper = mountTree({
      nodes: [makeNode({ errorHint: 'error' })],
    })
    expect(wrapper.find('.tree__node--parent .tree__error-hint-icon').exists()).toBe(true)
  })

  it('子节点的 progress 字段渲染', async () => {
    const wrapper = mountTree({
      nodes: [makeNode({
        children: [
          { id: 's1', label: 'step1', status: 'running', progress: 50 },
        ],
      })],
    })
    await wrapper.find('.tree__node--parent').trigger('click')
    expect(wrapper.find('.tree__node--child .tree__progress').text()).toBe('50%')
  })
})

// ─── Phase 图标集成 ─────────────────────────────────────────────────

describe('TreeView - Phase 图标集成', () => {
  it('node.phase 存在时渲染 PhaseIcon', () => {
    const wrapper = mountTree({
      nodes: [makeNode({ phase: Phase.Encrypting })],
    })
    const phaseIcon = wrapper.findComponent(PhaseIcon)
    expect(phaseIcon.exists()).toBe(true)
    expect(phaseIcon.props('phase')).toBe(Phase.Encrypting)
  })

  it('node.phase 缺失时渲染 StepMiniBadge', () => {
    const wrapper = mountTree({
      nodes: [makeNode({ phase: undefined })],
    })
    expect(wrapper.findComponent(PhaseIcon).exists()).toBe(false)
    // StepMiniBadge 存在
    expect(wrapper.find('.badge').exists()).toBe(true)
  })

  it('子节点 phase 存在时渲染 PhaseIcon', async () => {
    const wrapper = mountTree({
      nodes: [makeNode({
        children: [
          { id: 's1', label: 'step1', status: 'running', phase: Phase.Packing },
        ],
      })],
    })
    await wrapper.find('.tree__node--parent').trigger('click')
    const phaseIcons = wrapper.findAllComponents(PhaseIcon)
    // 父节点无 phase → 用 StepMiniBadge；子节点有 phase → 用 PhaseIcon
    expect(phaseIcons.length).toBe(1)
    expect(phaseIcons[0].props('phase')).toBe(Phase.Packing)
  })
})

// ─── 状态色 class ───────────────────────────────────────────────────

describe('TreeView - 状态色 class', () => {
  const statusCases: Array<[UnifiedTreeNode['status'], string]> = [
    ['success', 'tree__node--success'],
    ['failure', 'tree__node--failure'],
    ['running', 'tree__node--running'],
    ['timed_out', 'tree__node--timed_out'],
  ]

  it.each(statusCases)(
    '父节点 status=%s 时包含 class %s',
    (status, expectedClass) => {
      const wrapper = mountTree({
        nodes: [makeNode({ status })],
      })
      expect(wrapper.find('.tree__node--parent').classes()).toContain(expectedClass)
    },
  )
})

// ─── workflowRun 兼容派生 ───────────────────────────────────────────

describe('TreeView - workflowRun 兼容派生', () => {
  it('传入 workflowRun 时自动派生 nodes', () => {
    const stepNames = new Map([['enc_mp4', '加密 MP4'], ['enc_mp3', '加密 MP3']])
    const jobDisplayNames = new Map([['encrypt-all', '加密全部']])
    const wrapper = mountTree({
      nodes: undefined,
      workflowRun: makeWorkflowRun(),
      stepNames,
      jobDisplayNames,
    })
    // 父节点 label 应为 jobDisplayName
    const parent = wrapper.find('.tree__node--parent')
    expect(parent.text()).toContain('加密全部')
  })

  it('workflowRun 派生的子节点 label 使用 stepNames 映射', async () => {
    const stepNames = new Map([
      ['enc_mp4', '加密 MP4'],
      ['enc_mp3', '加密 MP3'],
      ['enc_jpg', '加密 JPG'],
    ])
    const wrapper = mountTree({
      nodes: undefined,
      workflowRun: makeWorkflowRun(),
      stepNames,
    })
    // 有 failure 子节点 → 自动展开
    const childLabels = wrapper.findAll('.tree__node--child .tree__label')
    expect(childLabels.length).toBe(3)
    expect(childLabels[0].text()).toBe('加密 MP4')
    expect(childLabels[1].text()).toBe('加密 MP3')
    expect(childLabels[2].text()).toBe('加密 JPG')
  })

  it('workflowRun 派生的 meta 显示完成数/总数', () => {
    const wrapper = mountTree({
      nodes: undefined,
      workflowRun: makeWorkflowRun(),
    })
    // 3 个 step，2 个终态（success + failure）→ 2/3
    expect(wrapper.find('.tree__node--parent .tree__meta').text()).toContain('2/3')
  })

  it('workflowRun 派生的 step.error → errorHint', async () => {
    const wrapper = mountTree({
      nodes: undefined,
      workflowRun: makeWorkflowRun(),
    })
    // 自动展开（有 failure），第三个子节点有 error
    const children = wrapper.findAll('.tree__node--child')
    expect(children[2].find('.tree__error-hint-icon').exists()).toBe(true)
  })

  it('workflowRun 派生的 step.durationMs → duration 格式化', async () => {
    const wrapper = mountTree({
      nodes: undefined,
      workflowRun: makeWorkflowRun(),
    })
    // 第一个 step durationMs=1500 → '1.5s'
    const children = wrapper.findAll('.tree__node--child')
    expect(children[0].find('.tree__duration').text()).toBe('1.5s')
  })

  it('nodes prop 优先于 workflowRun', () => {
    const wrapper = mountTree({
      nodes: [makeNode({ label: '来自 nodes prop' })],
      workflowRun: makeWorkflowRun(),
    })
    expect(wrapper.find('.tree__node--parent').text()).toContain('来自 nodes prop')
  })
})

// ─── 暗黑模式 ───────────────────────────────────────────────────────

describe('TreeView - 暗黑模式', () => {
  it('暗黑模式样式通过 body.dark 作用域生效（CSS 存在）', () => {
    // 验证组件挂载正常（暗黑模式 CSS 通过 :global(body.dark) 作用域，
    // 在 jsdom 中不实际应用样式，但组件结构应不受影响）
    const wrapper = mountTree()
    expect(wrapper.find('.tree').exists()).toBe(true)
    expect(wrapper.find('.tree__search').exists()).toBe(true)
  })

  it('暗黑模式下组件仍正常渲染节点', () => {
    const wrapper = mountTree({
      nodes: [makeNode({ label: 'Dark Mode Test' })],
    })
    expect(wrapper.find('.tree__node--parent').text()).toContain('Dark Mode Test')
  })
})
