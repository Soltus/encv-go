package com.encvgo.combolite.proxy

import com.combo.core.component.service.BaseHostService

/**
 * Phase 25 A2：Host 端 Service 代理池。
 *
 * 16 个空壳类（HostService1..16），每个对应一个 Android Service 实例。
 * Manifest 必须预注册这 16 个 <service>（同 demo 模式但缩小到 N=16）。
 *
 * 池大小选择理由（combolite.md §10.9）：
 *   - 期望 plugin 数 ≤ 15 + 1 buffer = 16
 *   - openlist 1 个 main instance + 1 buffer = 2 (绝对最小)
 *   - 留 14 个余量给未来 plugin 扩展
 *   - 不写死 10（demo 演示用），不写死 2（过保守）
 *
 * ComboLite 框架 [com.combo.core.proxy.ProxyManager.acquireServiceProxy] 用
 * 池 size 决定能同时启多少个 plugin service。setServicePool(listOf(HostService1..16))
 * 后，框架可同时支持 15 个 plugin service 并发（+ 1 buffer 防止 race）。
 *
 * ⚠️ 受约束（combolite.md §10.6 决策）：setServicePool 一次性 set
 * （重设会清空 activeServiceProxies，导致运行中 plugin service 引用丢失）。
 * 如需扩容到 N' > 16：
 *   1. 在本文件加 HostService17..N' class
 *   2. 同步改 EncvApplication.onFrameworkSetup 传新 listOf
 *   3. 同步改 host AndroidManifest.xml 加 <service> 声明
 */
class HostService1 : BaseHostService()
class HostService2 : BaseHostService()
class HostService3 : BaseHostService()
class HostService4 : BaseHostService()
class HostService5 : BaseHostService()
class HostService6 : BaseHostService()
class HostService7 : BaseHostService()
class HostService8 : BaseHostService()
class HostService9 : BaseHostService()
class HostService10 : BaseHostService()
class HostService11 : BaseHostService()
class HostService12 : BaseHostService()
class HostService13 : BaseHostService()
class HostService14 : BaseHostService()
class HostService15 : BaseHostService()
class HostService16 : BaseHostService()
