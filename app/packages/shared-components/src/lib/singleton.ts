/**
 * 模块级单例样板（单一真源）。
 *
 * 收敛自多处 `let _instance = null; function useX() { if (!_instance) _instance = factory(); return _instance; }`
 * + `__resetXForTests()` 的重复。返回 `{ get, reset }` 句柄，调用方保留既有导出名
 * （`useXSingleton()` / `__resetXForTests()`）以不动消费方。
 *
 * 注：带额外模块态（options 缓存 / 双实例 / 实例内 reset）的站点属「设计内差异」，
 * 不强行套用（呼应 K17 / K22 / K43 的「避免错误抽象」纪律）。
 */

export interface SingletonHandle<T> {
  /** 取得单例（首次调用时创建并缓存）。 */
  get(): T;
  /** 重置单例（仅供单测 setup / teardown 使用）。 */
  reset(): void;
}

export function defineSingleton<T>(factory: () => T): SingletonHandle<T> {
  let _instance: T | null = null;
  return {
    get() {
      if (!_instance) _instance = factory();
      return _instance;
    },
    reset() {
      _instance = null;
    },
  };
}
