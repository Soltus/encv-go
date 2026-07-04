// 临时占位 stub：仅用于 OpenPreview 工具注册一个 web_server 类型 command
// 让 OpenPreview(command_id=..., preview_url="http://localhost:16666/") 能注册成功
// 真实预览在 :16666 preview-gateway，本 stub 仅返回 200 保持端口在线
// 用 pm2 start 而不是 blocking 方式，参考 .trae/rules/preview-management.md
const http = require('node:http')
const port = Number(process.env.PORT || 15003)
http.createServer((_q, r) => {
  r.writeHead(200, { 'Content-Type': 'text/plain; charset=utf-8' })
  r.end('encv preview stub on :' + port + ', real preview at http://localhost:16666/\n')
}).listen(port, '0.0.0.0', () => {
  console.log('[openpreview-stub] listening on 0.0.0.0:' + port)
})
