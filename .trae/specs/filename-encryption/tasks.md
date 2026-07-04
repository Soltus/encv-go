# Tasks

- [ ] Task 1: 对齐 alist-encrypt 插件文件名加密与 alist-encrypt-go
  - [ ] 1.1 克隆 alist-encrypt-go 到 /tmp/alist-encrypt-go，分析 EncryptedName 实现细节
  - [ ] 1.2 对比 `internal/alistencrypt/filename.go` 与参考实现的差异
  - [ ] 1.3 修复 filename.go 使其二进制兼容
  - [ ] 1.4 同步 cipher.go / aesctr.go 确保一致
  - [ ] 1.5 更新前端 `useAlistEncrypt.ts` 的 decodeAlistFilename

- [ ] Task 2: ENC-FN 核心算法实现 — 深度定制文件名编码器
  - [ ] 2.1 新建 `internal/v2/filename/charset.go`：
    - 定义 5 个 FNCharset 常量（alnum[必选隐含] / symbols_basic / symbols_extended / hanzi_rare / emoji）
    - 每个常量对应一个 rune 切片字符池
    - alnum 始终作为基础池自动包含，Charsets 数组仅包含扩展池
    - 实现 `BuildCharsetTable(charsets []FNCharset, deconfuse bool) ([]rune, error)` ：alnum(必选) ∪ 扩展池并集 → 去混淆过滤 → 最终字符表
    - 实现 `EncodeToCharset(bytes []byte, table []rune) string` 和 `DecodeFromCharset(s string, table []rune) ([]byte, error)`
  - [ ] 2.2 新建 `internal/v2/filename/kdf.go`：HKDF-SHA256 密钥派生，password → 主密钥 → S-box 种子(32B) + N 个轮密钥(每轮16B)
  - [ ] 2.3 新建 `internal/v2/filename/sbox.go`：从种子确定性生成 256 字节 S-box + 逆 S-box
  - [ ] 2.4 新建 `internal/v2/filename/feistel.go`：Feistel 网络（正向/逆向，4-12 轮可配置）
  - [ ] 2.5 新建 `internal/v2/filename/encfn.go`：组装 Encode/Decode 流程，支持 compact + structured 模式
  - [ ] 2.6 定义错误类型：ErrFNInvalidFormat, ErrFNChecksumMismatch, ErrFNCorrupt, ErrFNEmptyInput, ErrFNNoCharsets, ErrFNCharsetMismatch

- [ ] Task 3: ENCV v4 容器数据结构扩展
  - [ ] 3.1 在 Manifest_v4 中新增 OriginalName 和 FilenameAlgorithm 字段
  - [ ] 3.2 定义 FlagFilenameEncrypted 常量 (0x0010)
  - [ ] 3.3 确保 WriteHeaderV4/ReadHeaderV4 正确处理新 flag 位

- [ ] Task 4: ENCV v4 容器 — 文件名解析优先级与展示层抽象
  - [ ] 4.1 后端文件列表 API：v4 容器返回 original_name + filename_alg 元数据
  - [ ] 4.2 实现 ResolveDisplayName 函数（优先级：Manifest 解码 > 物理文件名）
  - [ ] 4.3 Files.vue 使用 display_name 渲染

- [ ] Task 5: ENCV v4 容器 — 文件名编码写入与读取集成
  - [ ] 5.1 v4 容器创建时 ENC-FN.Encode 写入 Manifest.original_name
  - [ ] 5.2 v4 容器打开时 ENC-FN.Decode 还原 original_name
  - [ ] 5.3 解码失败 fallback 策略

- [ ] Task 6: 原始文件名修改 API
  - [ ] 6.1 实现 `PATCH /api/v1/file/rename` { path, new_name, password? }
  - [ ] 6.2 更新 Manifest.original_name + 持久化
  - [ ] 6.3 前端调用后自动刷新列表

- [ ] Task 7: 边界情况处理
  - [ ] 7.1 超长文件名：Manifest 完整存储，物理文件名自动缩短
  - [ ] 7.2 空/空白文件名：ENC-FN 接受空输入，展示层回退
  - [ ] 7.3 Unicode 全量：emoji/CJK/生僻汉字/RTL/control char
  - [ ] 7.4 基于 UTF-8 字节操作

- [ ] Task 8: 验证与测试
  - [ ] 8.1 Go 单元测试：ENC-FN 往返一致性（覆盖多种 Mode × 多种 Charset 组合）
  - [ ] 8.2 Go 单元测试：ENC-FN 密码敏感性 + 确定性 + 雪崩效应
  - [ ] 8.3 Go 单元测试：多选字符集并集正确性（[hanzi_rare, emoji] 扩展池 + alnum 必选基础，去混淆后大小）
  - [ ] 8.4 Go 单元测试：去混淆开关（开启/关闭/纯扩展池无 alnum 字符可移除时效果）
  - [ ] 8.4b Go 单元测试：空扩展池（Charsets=nil）→ 仅 alnum ± 去混淆
  - [ ] 8.5 Go 单元测试：ENC-FN 错误处理（篡改、非法字符集、空输入、超长输入）
  - [ ] 8.6 Go 单元测试：alist-encrypt EncryptedName 往返一致性
  - [ ] 8.7 E2E 测试：创建→物理重命名乱码→列表显示原名→rename→立即反映
  - [ ] 8.8 vue-tsc + vitest + vite build 全量验证

# Task Dependencies
- [Task 1] || [Task 2] 可并行
- [Task 3] 无依赖可并行
- [Task 4] depends on [Task 2, Task 3]
- [Task 5] depends on [Task 2, Task 3, Task 4]
- [Task 6] depends on [Task 5]
- [Task 7] depends on [Task 2, Task 5]
- [Task 8] depends on [Task 1, Task 2, Task 5, Task 6, Task 7]
