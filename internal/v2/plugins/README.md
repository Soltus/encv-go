TODO list

* [X] 恢复源后缀名中置倒转处理
* [ ] 加密文件名
* [ ] Openlist上传时加密
* [X] 解决文件名中包含 ".." 的解析错误 -> 统一使用 utils.ResolveToAbsPath 而不是粗暴拒绝 ".."
* [ ] CRC32校验
* [ ] `encv://$url`

可选优化：

* [ ] 递归处理应当通过参数指定而不是默认行为
