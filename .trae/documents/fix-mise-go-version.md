# 修正 mise.toml Go 版本配置

## 问题

- `go.mod` 要求 `go 1.25.1`
- 项目根目录 `/workspace/mise.toml` 错误配置为 `go = "1.24.3"`
- 导致每次 `mise exec -- go build` 时触发不必要的版本解析/下载

## 修复

将 `/workspace/mise.toml` 中：
```
go = "1.24.3"
```
改为：
```
go = "1.25.1"
```

mise 已安装 1.25.1（上一轮误操作时已下载完成），修改后直接可用，无需额外下载。

## 验证

```bash
mise exec -- go build ./cmd/encv/
```

预期：使用已安装的 1.25.1 直接编译，无下载。
