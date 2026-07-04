# 修复 MPV CI 构建失败计划

## 问题分析

CI 工作流 `.github/workflows/build-mpv-lib.yml` 存在两个问题：

### 问题 1：无效输入参数 `replace_existing_artifacts`
- **位置**：第 98 行
- **原因**：`replace_existing_artifacts` 是 `action-gh-release@v1` 的参数，在 **v2 中已移除**
- **影响**：Action 报 Warning 并忽略该参数，但核心功能受影响——无法替换已有 Release 的 artifacts
- **修复方案**：删除 `replace_existing_artifacts: true`，改用 v2 的等价参数 `overwrite_files: true`

### 问题 2：GitHub Token 权限不足（403）
- **错误信息**：`Resource not accessible by integration` — HTTP 403
- **原因**：工作流缺少 `permissions` 声明，默认的 `GITHUB_TOKEN` 没有 `contents: write` 权限，无法创建 Release
- **修复方案**：在工作流的 `jobs` 级别添加 `permissions: contents: write`

## 修改文件

**`.github/workflows/build-mpv-lib.yml`**

### 修改点 A：添加 permissions 声明（第 17 行之后）

```yaml
jobs:
  build:
    runs-on: ubuntu-latest
    permissions:
      contents: write    # ← 新增：允许创建/更新 GitHub Release
    steps:
```

### 修改点 B：替换废弃参数（第 98 行）

将：
```yaml
          replace_existing_artifacts: true
          files: ${{ runner.temp }}/mpv-jniLibs-arm64-v8a-v${{ env.MPV_LIB_VERSION }}.zip
```

改为：
```yaml
          overwrite_files: true
          files: ${{ runner.temp }}/mpv-jniLibs-arm64-v8a-v${{ env.MPV_LIB_VERSION }}.zip
```

## 验证方式

修复后手动触发 `build-mpv-lib` 工作流，确认：
1. 不再出现 `Unexpected input(s) 'replace_existing_artifacts'` 警告
2. Release 创建成功（HTTP 200/201）
3. zip 文件正确上传到 `mpv-native-libs` tag 对应的 Release

## 参考文档

- [softprops/action-gh-release v2 README](https://github.com/softprops/action-gh-release) — `overwrite_files` 替代了 v1 的 `replace_existing_artifacts`
- [GitHub Docs: Assigning permissions to jobs](https://docs.github.com/actions/security-guides/automatic-token-authentication#permissions-for-the-github_token) — `contents: write` 是创建 Release 的必要权限
