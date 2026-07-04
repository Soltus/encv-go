# 计划：按规范补全 video 插件和 alist_encrypt 加密任务配置项

## 一、现状分析

### 1.1 TaskOptions 声明机制

每个插件通过 `GetTaskOptions() -> TaskOptions` 向前端声明任务创建表单字段。前端 `NewTaskModal.vue` 根据 `ExtraFields[]` 动态渲染。

### 1.2 文件名加密：已实现未接入的两套系统

#### 系统 A: FNConfig（Feistel 密码）— 通用容器（text/image/pdf/video 等使用）

**已完成的代码链路**：

```
[encfn.go:27]   FNConfig.Encode(plaintext) → Feistel + SBox → encoded string  ✅ 已实现
[encfn.go:56]   FNConfig.Decode(encoded) → Feistel decrypt → plaintext       ✅ 已实现
[writer.go:95]  SetFilenameEncoding(name, password, cfg)                    ✅ 开关方法
[writer.go:99]  encryptFilename = originalName != "" && password != ""      ✅ 判断逻辑
[writer.go:192] Close() 时: if encryptFilename → fnCfg.Encode → 写入 Footer  ✅ 写入逻辑
```

**缺失环节**：
- 所有插件的 `PostEncryptProcessor` / `StandardPostEncrypt` 都**没有调用** `SetFilenameEncoding()`
- 唯一调用方是 [mobile_service.go:519](/workspace/internal/service/mobile_service.go#L519) 的 RenameFile API

#### 系统 B: EncodeName/DecodeName（MixBase64 + CRC6）— AlistEncrypt 专用

**已完成的代码链路**：

```
[filename.go:229]  EncodeName(plain, password, encType) → MixBase64 + CRC6    ✅ 已实现
[filename.go:242]  DecodeName(encoded, password, encType) → reverse            ✅ 已实现
[decryptor.go:44]  DecryptFile() 内: tryDecodeFilename → DecodeName()           ✅ 解密端已接入！
[mobile_api.go:1018] /api/alist-encrypt/decode-filename API                   ✅ API 已有
```

**缺失环节**：

```
[plugin.go:164]  PostEncryptProcessor():
  → RenameToFinalEncrypted(tempPath, originalFilename, outputDir, suffix)
  → 只做 strings.TrimSuffix + 拼接 suffix (secret.txt → secret.bin)
  → ❌ 没有调用 EncodeName()
```

解密端已经会尝试 `DecodeName()` 恢复文件名，但加密端不调用 `EncodeName()`，导致这个功能是死的。

### 1.3 当前 TaskOptions 缺失项

#### Video 插件 — `[plugin.go:431-438](/workspace/internal/v2/plugins/video/plugin.go#L431-L438)`

```go
// ExtraFields 为空
func (p *VideoPlugin) GetTaskOptions() pluginInterfaces.TaskOptions {
    return pluginInterfaces.TaskOptions{
        PasswordStrategy:     pluginInterfaces.PasswordGlobal,
        SupportVersionSelect: true,
        SupportedVersions:    p.SupportedContainerVersions(),
        DefaultVersion:       p.DefaultContainerVersion(),
    }
}
```

**缺失**：
1. `stream_preset`（select）— 编码预设 balanced/quality/high_quality
2. `encrypt_filename`（bool）— 是否启用 FNConfig 文件名加密（默认 false）

#### AlistEncrypt 插件 — `[plugin.go:235-250](/workspace/internal/v2/plugins/alistencrypt/plugin.go#L235-L250)`

```go
// 只有 plugin_password
func (p *AlistEncryptPlugin) GetTaskOptions() pluginInterfaces.TaskOptions {
    return pluginInterfaces.TaskOptions{
        PasswordStrategy:     pluginInterfaces.PasswordIndependent,
        SupportVersionSelect: false,
        ExtraFields: []pluginInterfaces.TaskField{
            { Key: "plugin_password", Label: "tasks.pluginPassword", Type: "password", ... },
        },
    }
}
```

**缺失**：
1. `encode_filename`（bool）— 是否调用 `EncodeName()` 编码文件名（默认 false）
2. `enc_type`（select）— 编码算法 aesctr/rc4md5/chacha20（仅在 encode_filename=true 时有意义）

---

## 二、修改清单

| # | 文件 | 改动 |
|---|------|------|
| B1 | `internal/v2/plugins/video/plugin.go` GetTaskOptions() L431-438 | 补充 `stream_preset`(select) + `encrypt_filename`(bool) |
| B2 | `internal/v2/plugins/alistencrypt/plugin.go` GetTaskOptions() L235-250 | 补充 `encode_filename`(bool) + `enc_type`(select) |
| B3 | `internal/v2/plugins/task_options_test.go` | 更新断言 |
| F1 | `src/composables/useI18n.ts` | 补充 i18n key |
| F2 | `src/components/NewTaskModal.vue` | 补充 `<ion-select>` + `<ion-toggle>` 渲染分支 |

---

## 三、实现步骤

### Step B1: Video 插件

```go
func (p *VideoPlugin) GetTaskOptions() pluginInterfaces.TaskOptions {
    return pluginInterfaces.TaskOptions{
        PasswordStrategy:     pluginInterfaces.PasswordGlobal,
        SupportVersionSelect: true,
        SupportedVersions:    p.SupportedContainerVersions(),
        DefaultVersion:       p.DefaultContainerVersion(),
        ExtraFields: []pluginInterfaces.TaskField{
            {
                Key:          "stream_preset",
                Label:        "tasks.streamPreset",
                Type:         "select",
                Required:     false,
                DefaultValue: "balanced",
                Help:         "tasks.streamPresetHelp",
                Options:      []string{"balanced", "quality", "high_quality"},
                Condition:     "encrypt",
            },
            {
                Key:          "encrypt_filename",
                Label:        "tasks.encryptFilename",
                Type:         "bool",
                Required:     false,
                DefaultValue: "false",
                Help:         "tasks.encryptFilenameHelp",
                Condition:     "encrypt",
            },
        },
    }
}
```

- `stream_preset`: condition=encrypt，仅加密时需选
- `encrypt_filename`: condition=encrypt，默认 false。勾选后执行侧调用 `SetFilenameEncoding()` 触发 FNConfig 写入 Manifest Footer

### Step B2: AlistEncrypt 插件

```go
func (p *AlistEncryptPlugin) GetTaskOptions() pluginInterfaces.TaskOptions {
    return pluginInterfaces.TaskOptions{
        PasswordStrategy:     pluginInterfaces.PasswordIndependent,
        SupportVersionSelect: false,
        ExtraFields: []pluginInterfaces.TaskField{
            {
                Key:       "plugin_password",
                Label:     "tasks.pluginPassword",
                Type:      "password",
                Required:  false,
                Help:      "tasks.pluginPasswordHelp",
                Condition: "",
            },
            {
                Key:          "encode_filename",
                Label:        "tasks.encodeFilename",
                Type:         "bool",
                Required:     false,
                DefaultValue: "false",
                Help:         "tasks.encodeFilenameHelp",
                Condition:     "encrypt",
            },
            {
                Key:          "enc_type",
                Label:        "tasks.encType",
                Type:         "select",
                Required:     false,
                DefaultValue: "aesctr",
                Help:         "tasks.encTypeHelp",
                Options:      []string{"aesctr", "rc4md5", "chacha20"},
                Condition:     "encrypt",
            },
        },
    }
}
```

- `encode_filename`: condition=encrypt，默认 false。勾选后执行侧在 `PostEncryptProcessor` 中调用 `EncodeName()` 替代当前的纯扩展名替换
- `enc_type`: condition=encrypt，EncodeName 的算法参数

### Step B3: 测试更新

Video: ExtraFields 0→2，断言 stream_preset(select) + encrypt_filename(bool, default=false)
AlistEncrypt: ExtraFields 1→3，断言 plugin_password + encode_filename(bool, default=false) + enc_type(select)

### Step F1: i18n

```typescript
'tasks.streamPreset': '编码预设',
'tasks.streamPresetHelp': '选择视频流式编码预设',
'tasks.encryptFilename': '加密文件名',
'tasks.encryptFilenameHelp': '将原始文件名加密后存入容器元数据',
'tasks.encodeFilename': '编码文件名',
'tasks.encodeFilenameHelp': '使用 MixBase64 算法编码文件名，解密时自动恢复',
'tasks.encType': '编码算法',
'tasks.encTypeHelp': '文件名编码使用的算法类型',
```

### Step F2: NewTaskModal.vue

import: `IonSelect`, `IonSelectOption`, `IonToggle`

模板 ExtraFields 区域按 type 分支渲染 bool/select/string+password。

---

## 四、验证

```bash
# 后端
cd /workspace && go test ./internal/v2/plugins/ -run "Test.*_GetTaskOptions" -v

# 前端
cd /workspace/app/encv-mobile && npx vitest run --reporter=verbose

# API
curl '.../predict-plugin?path=test.mp4&taskType=encrypt' | jq '.candidates[]|select(.id=="video")|.taskOptions.extraFields'
# → [{key:"stream_preset",type:"select",condition:"encrypt"},{key:"encrypt_filename",type:"bool",defaultValue:"false",condition:"encrypt"}]

curl '.../predict-plugin?path=test.bin&taskType=encrypt' | jq '.candidates[]|select(.id=="alist_encrypt")|.taskOptions.extraFields'
# → [{key:"plugin_password",type:"password"},{key:"encode_filename",type:"bool",defaultValue:"false",condition:"encrypt"},{key:"enc_type",type:"select",condition:"encrypt"}]
```

## 五、注意

本次补全是**声明侧**（`GetTaskOptions`）。执行侧（读取 extraFields 并实际调用 `EncodeName()` / `SetFilenameEncoding()`）为后续工作。
