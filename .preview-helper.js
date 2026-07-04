// 最小占位 HTTP server：仅用于 OpenPreview 工具注册一个长跑的 web_server
// 进程（不会阻塞 sandbox 会话，因为它是 pm2 管理的 daemon）。
// 真实预览通过 preview-gateway :16666 提供，本服务仅返回 200 保持 :15001 在线。
// 持久化：pm2 save 写入 /root/.pm2/dump.pm2，sandbox 会话重置后
//         `pm2 resurrect` 自动恢复。
const http = require('node:http');
const PORT = Number(process.env.PORT || 15002);
const server = http.createServer((req, res) => {
  res.writeHead(200, { 'Content-Type': 'text/plain; charset=utf-8' });
  res.end('preview-helper: open http://localhost:16666/ for the real preview\n');
});
server.listen(PORT, '0.0.0.0', () => {
  console.log(`[preview-helper] listening on 0.0.0.0:${PORT}`);
});
