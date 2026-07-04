# Tasks

- [x] Task 1: Replace OpenListBridge placeholder stubs with real openlistlib calls
  - [x] 1.1 Import `openlistlib.Openlistlib`, `openlistlib.Event`, `openlistlib.LogCallback` from the gomobile-bind AAR
  - [x] 1.2 Add `private val openlistlib = Openlistlib()` field
  - [x] 1.3 Wire `init()` → `Openlistlib.setConfigData(dataDir)`, `Openlistlib.setPort(port)`
  - [x] 1.4 Wire `start()` → `Openlistlib.start()` on a background thread, set `running=true`, call `broadcastStatus`
  - [x] 1.5 Wire `shutdown()` → `Openlistlib.shutdown()` + 500ms grace timer + force-kill fallback
  - [x] 1.6 Wire `setAdminPassword()` → `Openlistlib.setAdminPassword()`
  - [x] 1.7 Wire `forceDbSync()` → `Openlistlib.forceDBSync()` with try-catch + SAT-DBG log
  - [x] 1.8 Remove local `OpenListEvent` / `OpenListLogCallback` stubs (interfaces now from AAR)
  - [x] 1.9 Implement `Event` interface methods: `onShutdown`, `onStartError`, `onProcessExit` with real imports
  - [x] 1.10 Implement `LogCallback` interface: `onLog(level, time, log)` forwards to LocalBroadcast BROADCAST_LOG
  - **Validation**: `./gradlew :plugin-openlist:compileDebugKotlin` passes with no unresolved references

- [x] Task 2: Add saturation debug logging (SAT-DBG) across entire OpenList Android stack
  - [x] 2.1 `OpenListService`: SAT-DBG for onCreate / onStartCommand / onDestroy / startupSequence steps / shutdownSequence
  - [x] 2.2 `OpenListBridge`: SAT-DBG for each method call + result + elapsed ms
  - [x] 2.3 `OpenListConfig`: SAT-DBG for load/save + dataDir resolution
  - [x] 2.4 `OpenListPluginEntry`: SAT-DBG for onLoad / onUnload + BroadcastReceiver registration
  - **Validation**: Manually inspect `Log.e(TAG, "[SAT-DBG][OpenList] ...")` calls in each file, verify they cover every significant code path

- [x] Task 3: Register OpenListBridge as Koin singleton in OpenListPluginEntry
  - [x] 3.1 Change `pluginModule` from `emptyList()` to `listOf(module { single { OpenListBridge } })`
  - [x] 3.2 Validate Koin can resolve the singleton (log SAT-DBG on resolution)
  - **Validation**: `./gradlew :plugin-openlist:compileDebugKotlin` passes

- [x] Task 4: Extend eventBus TypeScript types for OpenList events
  - [x] 4.1 Add `openlist:status` event type to `EncvEvents` interface in `useEventBus.ts`
  - [x] 4.2 Add `openlist:log` event type
  - [x] 4.3 Add `openlist:error` event type
  - **Validation**: `npx vue-tsc --noEmit` passes with no type errors

- [x] Task 5: Enhance LocalOpenListStatusCard.vue with developer-friendly error UI
  - [x] 5.1 Add crash-loop detection: track consecutive `running→stopped` transitions, if ≥3 in 10s → show red card
  - [x] 5.2 Subscribe to `eventBus.on('openlist:status', ...)` and `eventBus.on('openlist:error', ...)` instead of HTTP polling
  - [x] 5.3 Port conflict card: orange background, port number, "修改端口" action button
  - [x] 5.4 Crash loop card: red background, cycle count, "查看诊断日志" action button → router.push to /tabs/devlogs
  - [x] 5.5 Not installed card: gray background, "前往扩展管理" action button → router.push to /tabs/extensions
  - [x] 5.6 Add SAT-DBG console.error calls on every state transition and action handler
  - **Validation**: `npx vue-tsc --noEmit` passes, visual review of card variants

- [x] Task 6: Wire Capacitor GoProcess plugin to forward OpenList broadcasts to eventBus
  - [x] 6.1 In `GoProcess.ts`, add `getOpenListFullState()` typed method
  - [x] 6.2 Add Capacitor `addListener('openlist:status', callback)` registration in `useOpenListBridge.ts` composable (new file)
  - [x] 6.3 In the Capacitor plugin's Android implementation, register a BroadcastReceiver for OpenList intents and call `notifyListeners('openlist:status', payload)`
  - **Validation**: Build and verify TypeScript types compile; verify Android BroadcastReceiver is registered in plugin's `load()` method

- [x] Task 7: Add OpenList settings UI to PluginSettings or ExtensionsPage
  - [x] 7.1 Add i18n keys for OpenList settings fields (port, dataDir, adminPassword) in `settings.ts`
  - [x] 7.2 Add OpenList settings schema definition to `schema.json` under `plugin_settings.openlist`
  - [x] 7.3 In `ExtensionsPage.vue`, add a "设置" button on OpenList card that navigates to PluginSettings with openlist filters
  - [x] 7.4 In `PluginSettings.vue`, ensure OpenList settings fields render with correct field types (integer port, string path with browse, password)
  - **Validation**: `npx vue-tsc --noEmit` passes, verify schema.json validates

- [x] Task 8: Add SAT-DBG logging on the frontend side
  - [x] 8.1 In `useOpenListBridge.ts` composable: SAT-DBG for Capacitor listener registration, status changes, errors
  - [x] 8.2 In `LocalOpenListStatusCard.vue`: SAT-DBG for state transitions (not_installed → running, running → conflict, etc.)
  - [x] 8.3 In `ExtensionsPage.vue`: SAT-DBG for OpenList plugin state check results
  - **Validation**: Search `console.error('[SAT-DBG][OpenList]'` across all frontend files, confirm coverage

# Task Dependencies
- Task 2 depends on Task 1 (need real bridge to log real events)
- Task 3 is independent (Koin registration doesn't need real AAR)
- Task 4 is independent (pure TypeScript type definitions)
- Task 5 depends on Task 4 (needs typed events)
- Task 6 depends on Task 3 (needs bridge accessible) and Task 4 (needs event types)
- Task 7 is independent (pure frontend + schema changes)
- Task 8 can run in parallel with Tasks 4-7 (adds logging to existing code)