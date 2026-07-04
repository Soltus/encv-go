# 修复 PlayMode 硬编码 → 常量化重构

## 问题

`'artplayer'` / `'mpv-plugin'` / `'external'` 等 PlayMode 字符串字面量硬编码散落在 **Files.vue（6处）** 和 **Settings.vue（4处）**，违反 DRY 原则，且类型定义与使用值不一致导致 TS 编译失败。

## 修复方案：提取常量 + 联合类型推导

### Step 1: 新建常量文件

**新建**: `app/encv-mobile/src/constants/player.ts`

```typescript
export const PLAY_MODE = {
  ARTPLAYER: 'artplayer',
  MPV_PLUGIN: 'mpv-plugin',
  EXTERNAL: 'external',
} as const

export type PlayMode = (typeof PLAY_MODE)[keyof typeof PLAY_MODE]

export const VIDEO_DEFAULT: PlayMode = PLAY_MODE.ARTPLAYER
export const AUDIO_DEFAULT: PlayMode = PLAY_MODE.MPV_PLUGIN
```

### Step 2: 重构 Files.vue

- 删除 L273 的 `type PlayMode = ...` 本地定义
- import `{ PLAY_MODE, type PlayMode, VIDEO_DEFAULT, AUDIO_DEFAULT }` from `@/constants/player`
- L278: `stored === PLAY_MODE.ARTPLAYER || stored === PLAY_MODE.MPV_PLUGIN || stored === PLAY_MODE.EXTERNAL`
- L279: `return mediaType === 'video' ? VIDEO_DEFAULT : AUDIO_DEFAULT`
- L289: `case PLAY_MODE.ARTPLAYER:`
- L292: `case PLAY_MODE.MPV_PLUGIN:`
- L299: `case PLAY_MODE.EXTERNAL:`

### Step 3: 重构 Settings.vue

- import `{ PLAY_MODE, type PlayMode }` from `@/constants/player`
- L417: `ref(localStorage.getItem('encv_player_video') || PLAY_MODE.ARTPLAYER)`
- L419-420: 删除 `'mpv'` → `'mpv-plugin'` 迁移代码（常量已统一，不再需要）
- L422: `ref(localStorage.getItem('encv_player_audio') || PLAY_MODE.MPV_PLUGIN)`

### Step 4: 验证

```bash
cd app/encv-mobile && npx vue-tsc --noEmit && npm run build
```

## 变更文件

| 文件 | 操作 |
|------|------|
| `src/constants/player.ts` | **新建** — PlayMode 常量 + 类型导出 |
| `src/views/Files.vue` | **修改** — 删除本地类型，import 常量，6 处替换 |
| `src/views/Settings.vue` | **修改** — import 常量，4 处替换，删除迁移代码 |
