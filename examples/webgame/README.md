# Ginx Arena — Vue 3 + TypeScript + Phaser

独立、可运行的浏览器联机示例。两个标签页可以进同一房间，移动探索者、争抢水晶，
观察排行榜、房间事件和 Ginx 运行指标。所有玩法都在本目录，`gnet/gface/gcore` 不包含此业务。

## 一键启动

需要 Docker Desktop（Linux 容器）和 PowerShell 7，不需要本机 Go、Node 或 Python。
首次启动需联网下载构建依赖。双击本目录 `start.cmd`，或在仓库根运行：

```powershell
pwsh -File examples/webgame/start.ps1
```

访问 **http://127.0.0.1:8090**。用不同昵称打开两个标签页，进入同一房间。
WASD/方向键（先点画布获取焦点）、点击地图、页面方向按钮都可移动。
靠近水晶自动得到 1 分，水晶 5 秒后刷新。切房或断线后本局分数归零。

```powershell
pwsh -File examples/webgame/start.ps1 -Port 8091
pwsh -File examples/webgame/start.ps1 -Stop
```

跨平台可直接运行（从仓库根）：

```sh
docker compose -p ginx-webgame -f examples/webgame/compose.yaml up -d --build --wait
docker compose -p ginx-webgame -f examples/webgame/compose.yaml logs --tail 100
docker compose -p ginx-webgame -f examples/webgame/compose.yaml down
```

只发布本机 HTTP 端口；TCP 上游监听容器内 loopback 随机端口，不暴露到宿主机。
镜像内含前端静态文件与 Go 服务，非 root、只读运行，无运行时外部资源请求。
它使用独立 Compose 项目 `ginx-webgame`，不影响旧 `start-server` 示例。

## 目录

```text
examples/webgame/
  frontend/              Vue 3 + TS 页面、Phaser 场景、二进制协议客户端
    src/App.vue          登录、房间、排行榜、诊断面板
    src/game.ts          场景绘制、键盘/触控、服务端坐标插值
    src/client.ts        WebSocket 生命周期、心跳、状态
    src/protocol.ts      示例消息 ID、DataPack 编解码
  server/                访客、房间世界、服务端模拟、HTTP 入口
  cmd/                   应用生命周期、TCP/HTTP/桥接组装
  Dockerfile             前端与 Go 多阶段构建
  compose.yaml           本地隔离运行
  start.ps1 / start.cmd  一键构建、健康检查、停止
wsbridge/                可复用的固定上游 WS→TCP 桥接，不定义玩法
test/webgame_test.go     桥接和游戏后端集成测试
test/webgame/            Playwright 双浏览器与移动端验收
```

## 真实链路与验证范围

```text
HTTP /api/guest -> session.Issue
浏览器二进制 WebSocket
  -> wsbridge (每个浏览器独立 TCP 连接)
  -> gnet DataPack -> Worker -> 消息 ID 路由
  -> session.Bind -> gcore.Room + AOI -> 示例权威世界
  -> Room.Broadcast -> Connection.SendBuffMsg
  -> TCP -> WebSocket -> Phaser 渲染
```

- 两个隔离房间，每房间最多 8 人；Go 4 个 Worker，连接上限 64。
- 20 Hz 模拟 / 10 Hz 快照；客户端 20 Hz 输入，不允许提交坐标、分数或玩家身份。
- 单调输入序列号、防重复输入、方向归一化、180 单位/秒固定步进移速。
- 最后输入超 250ms 自动停止；失焦/页面隐藏发送零输入；2 秒应用心跳。
- 角色坐标提交 AOI，页面展示九宫格邻居数。**本示例仍全房间广播，不声称做 AOI 裁剪。**
- 断线/切房清理成员和 AOI，绝对 30 分钟会话有效期，10 秒未鉴权清理。
- 服务端限流、消息长度验证、发送队列背压、异常包关闭、停止时关闭升级连接。
- 诊断计数来自 `gnet.Server.GetMetrics()`；不是前端模拟数据。

没有数据库存档、账号密码、反作弊平台、战斗或集群服务。访客名不是账号，允许重名。
内存分数、世界和身份均非持久化；需要 SQLite 成长示例请使用 `examples/gameapp`。
未认证的访客签发与指标接口仅供本地演示，不应直接发布公网。

## 开发模式

前端 Node 20.19+ 或 22.12+。依赖通过 `package-lock.json` 锁定；TypeScript 5.9.3
与 Vue 类型检查器已验证，不能仅因 npm 最新版本变化就直接升级主版本。

```powershell
# 仓库根：有本机 Go 时
go run ./examples/webgame/cmd -origins http://127.0.0.1:5173

# 或用 Docker 跑开发后端，不发布内部 TCP
docker run --rm -p 127.0.0.1:8090:8090 --mount "type=bind,source=${PWD},target=/src" -w /src golang:1.26-bookworm go run ./examples/webgame/cmd -http 0.0.0.0:8090 -origins http://127.0.0.1:5173

# 另一个终端：前端目录
cd examples/webgame/frontend
npm ci
npm run dev
```

访问 http://127.0.0.1:5173；Vite 代理 `/api`、`/healthz` 和 `/ws`。
如果使用 localhost，需要将 `http://localhost:5173` 加入 `-origins` 精确白名单。
普通生产构建同源，不需要额外 Origin。Token 只在内存，通过鉴权帧发送，不放 URL/日志/本地存储。

## 验证

```powershell
# 仓库根
go test -count=1 ./...
go test -race -count=1 ./...
go vet ./...
go build ./...

# frontend/，先确保 Demo 在 8090 运行，验收时不要保留其他已连接玩家
npm ci
npm run build
npx playwright install chromium
npm run test:e2e
# 自定义地址：$env:GINX_WEB_URL='http://127.0.0.1:8091'
```

端到端验收包括：两个独立浏览器上下文 → HTTP 访客 → WS/TCP 鉴权 → 入房 →
键盘移动 → 服务端采集计分 → 另一浏览器榜单同步 → 切房隔离 → 断线清理 → 重连，
以及移动端布局与无效 WebSocket 帧拒绝。另用浏览器故障注入验证切房失败后保持原房间、
首次入房被拒绝后立即释放连接，以及指标请求失败时清除旧在线状态。
截图与报告在被 Git 忽略的 `frontend/test-results/`。

协议及桥接契约见 [WebSocket 与 Web 游戏指南](../../docs/webgame-guide.md)。
