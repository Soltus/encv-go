# 修复 CI APK 选择逻辑错误导致签名验证失败

## Why

CI 构建成功生成了签名的 APK（`app-release-signed.apk`），但 `find -name "*signed*.apk"` 模式同时匹配了 `app-release-unsigned.apk`（因为 `unsigned` 包含子串 `signed`），导致验证了错误的 APK 文件。

## 根因分析

### 洞察：`*signed*` 通配符的语义陷阱

`find -name "*signed*.apk"` 的设计意图是匹配 Capacitor CLI 生成的签名 APK（`app-release-signed.apk`），但 `unsigned` = `un` + `signed`，所以 `*signed*` 也匹配了 `app-release-unsigned.apk`。

这是典型的"通配符过于宽松"问题。正确的做法是精确匹配签名 APK 的文件名，或排除 unsigned 文件。

### CI 日志证据

```
[success] Successfully generated app-release-signed.apk
...
-rw-r--r-- 1 runner runner 34698898 May 27 18:57 app-release-signed.apk
-rw-r--r-- 1 runner runner 34665116 May 27 18:57 app-release-unsigned.apk
Selected APK: android/app/build/outputs/apk/release/app-release-unsigned.apk  ← 选错了！
Verifying signature...
DOES NOT VERIFY
ERROR: Missing META-INF/MANIFEST.MF
```

### 防回归范式

1. **精确匹配签名 APK**：使用 `-name "app-release-signed.apk"` 而非 `-name "*signed*.apk"`
2. **或排除 unsigned**：使用 `! -name "*unsigned*"` 过滤
3. **验证签名后才能继续**：`apksigner verify` 失败必须 `exit 1`（已在上次修复中实现）

## What Changes

- 修复 CI 工作流中 APK 选择逻辑，精确匹配签名 APK 文件名
- 同时修复 `Verify APK contents` 步骤中的相同问题

## Impact

- Affected code: `.github/workflows/android.yml`
- Affected specs: `fix-release-apk-signing`（追加修复）

## ADDED Requirements

### Requirement: CI APK 选择精确匹配

CI 工作流 SHALL 精确选择签名 APK，不误选 unsigned APK。

#### Scenario: 签名 APK 存在时优先选择
- **WHEN** `app-release-signed.apk` 存在于输出目录
- **THEN** CI SHALL 选择该文件进行签名验证

#### Scenario: 签名 APK 不存在时报错
- **WHEN** `app-release-signed.apk` 不存在
- **THEN** CI SHALL 报错退出，而非 fallback 到 unsigned APK

## MODIFIED Requirements

无

## REMOVED Requirements

无
