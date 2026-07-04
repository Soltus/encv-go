// Cypress Component Testing store helper（2026-06-23）
//   - 解决 cypress component 模式下 cy.window() 拿不到 win 的问题
//   - 通过 module-level closure 共享 test pinia
//   - spec 通过 import { _getTaskStore } 直接访问 store
//   - 不依赖 cy.window() / win.__stores

import { createPinia, setActivePinia, type Pinia } from 'pinia'
import { createMemoryHistory, createRouter, type Router } from 'vue-router'
import { useTaskStore } from '@/stores/taskStore'
import { useRunTasksStoreSingleton } from '@/stores/runTasksStore'
import { useRunSummariesSingleton } from '@/composables/useRunSummaries'

let _testPinia: Pinia | null = null

/**
 * 共享 mock router（component.ts 也会用）
 *   - 这里 create instance，component.ts 共用同一实例
 *   - spec 想测某个 route params 时用 _pushTo 切换
 */
export const sharedTestRouter: Router = createRouter({
  history: createMemoryHistory(),
  routes: [
    { path: '/', component: { template: '<div />' } },
    { path: '/tabs/tasks', component: { template: '<div />' } },
    { path: '/group/:runId', component: { template: '<div />' } },
    { path: '/:pathMatch(.*)*', component: { template: '<div />' } },
  ],
})

/**
 * 重置测试 pinia（在 component.ts 的 beforeEach 调）
 * - 创建新 pinia
 * - setActivePinia → 让 useTaskStore() 等用这个 pinia
 */
export function _resetTestPinia() {
  _testPinia = createPinia()
  setActivePinia(_testPinia)
}

/**
 * 拿 task store（用当前 active pinia，即 _testPinia）
 * - spec 里直接调：const store = _getTaskStore(); store.bulkSetTasks(...)
 */
export function _getTaskStore() {
  if (!_testPinia) {
    throw new Error('[_getTaskStore] _testPinia is null — _resetTestPinia() not called yet (beforeEach not run?)')
  }
  // setActivePinia 是 noop（已经在 beforeEach 调过）但保证幂等
  setActivePinia(_testPinia)
  return useTaskStore()
}

export function _getRunTasksStore() {
  if (!_testPinia) throw new Error('[_getRunTasksStore] _testPinia is null')
  setActivePinia(_testPinia)
  return useRunTasksStoreSingleton()
}

export function _getRunSummaries() {
  if (!_testPinia) throw new Error('[_getRunSummaries] _testPinia is null')
  setActivePinia(_testPinia)
  return useRunSummariesSingleton()
}

/** 拿当前 pinia（spec 想直接操作 pinia state 时用） */
export function _getCurrentPinia(): Pinia {
  if (!_testPinia) throw new Error('[_getCurrentPinia] _testPinia is null')
  return _testPinia
}

/** 切到指定 path（spec 想测某个 route params 时用） */
export async function _pushTo(path: string) {
  await sharedTestRouter.push(path)
}
