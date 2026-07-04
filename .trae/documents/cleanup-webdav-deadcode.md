# 清理死代码 + 删除无效"控制台"开关

## 一、删除无效的 Console 配置项

### 问题
`LogConfig.Console` 字段（[types.go:204](file:///workspace/internal/v2/types/types.go#L204)）是个空壳——UI 有开关，但 logger 初始化逻辑完全不读取它，切换无任何效果。

### 执行步骤

1. **`internal/v2/types/types.go`** — 删除 `Console bool` 字段
2. **`internal/config/config.go`** — 删除默认值 `Console: true`
3. **前端 i18n** — 删除 `'settings.console': '控制台'` 和 `'settings.console': 'Console'`

## 二、清理死文件 WebDAV.vue

1. **删除** `app/encv-mobile/src/views/WebDAV.vue`
2. **全局搜索确认** 无残留引用
3. **验证构建** — `go vet ./... && vue-tsc --noEmit && vite build`
