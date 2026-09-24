# WebSocket 桥接与 Web 游戏示例

## 边界

`wsbridge` 是可选传输适配器，依赖 Gorilla WebSocket。现有 `gnet`、`gface` API
和 TCP 线协议保持不变，核心无需导入 WebSocket、Vue 或 Phaser。
它不是原生 WS Connection 实现：一个 WS 隧道消耗一个上游 TCP 连接和对应转发协程。
Ginx 的 RemoteAddr 是桥接地址，不是浏览器真实 IP；不要将其用作用户身份或 IP 限流依据。
需要互联网部署时应另外增加 TLS、认证、IP/用户限流和代理信任配置。

## 桥接契约

```go
bridge, err := wsbridge.New(wsbridge.Options{
    Upstream: "127.0.0.1:7777", // 由应用固定配置，绝不由请求控制
    MaxPacketSize: 64 << 10,
    MaxConnections: 64,
})
// 处理 err，注册到 http.ServeMux，并在应用停止时显式 bridge.Close()。
```

- 浏览器每个**二进制 WS 消息**恰好携带一条完整 Ginx DataPack：
  `uint32 LE body_length + uint32 LE message_id + body_length bytes`。
- WS 分片由 WebSocket 库重组；不接受文本、缺头/缺体、超长或一个 WS 消息内多个 DataPack。
- 上游是 TCP 流，使用已有 `DataPack.ReadMessage` 处理半包/粘包，每包独立写回 WS。
- 默认包体 64 KiB，连接上限 64（含拨号中/升级中），空闲 20 秒，写入 3 秒；构造函数没有拨号副作用。
- 默认要求精确同源 Origin，包括 scheme/host/port；缺失 Origin 也拒绝。
  `AllowedOrigins` 仅支持完整 `http(s)://host[:port]` 白名单，不支持通配符、路径或转发头推断。
- 不在 URL 中携带会话；桥接不解释业务，应用仍需在 Ginx 路由内鉴权。
- 每方向只有一个 reader/writer；无无限队列。慢客户端通过写超时断开，Ginx 队列满也关闭。
- 上游拨号失败返回 HTTP 502；容量满或已关闭返回 503；非法 Origin 返回 403。
- `Close()` 禁止新连接、取消拨号/升级请求、关闭双方 socket，并等待已有隧道退出。
  `http.Server.Shutdown` 不负责升级后的 socket，应用必须显式关闭 bridge。

代理 TLS 终止时，应用看到的是 HTTP；请明确设置允许的 HTTPS Origin，勿无条件信任
`X-Forwarded-*`。WebSocket ping 本身不代替游戏层心跳；Demo 每 2 秒发送心跳业务包。

## Demo 消息

包体均为 UTF-8 JSON。数值 ID 是**本示例业务**，不加入框架常量；Go/TS 一致性有自动测试。

| ID | 方向 | 结构/作用 |
| --- | --- | --- |
| 4101 | 请求/响应 | `{token}` → `{id, name}`，绑定 HTTP 临时身份到连接 |
| 4102 | 请求/响应 | `{room_id}` → `{room_id}`，进入或切换房间 |
| 4103 | 请求 | `{dx, dy, seq}`，有限方向向量、递增 uint32 序列号 |
| 4104 | 请求/响应 | `{}` → `{ok:true}`，离房但保留连接 |
| 4105 | 请求/响应 | `{sent_at}` 原样应答，计算 RTT |
| 4201 | 服务端推送 | `{tick,room_id,players,crystals}`，房间快照 |
| 4202 | 服务端推送 | `{request_id,code}`，拒绝无效/未鉴权/满房请求 |
| 4203 | 服务端推送 | `{kind,message}`，加入/离开/采集事件 |

玩家字段：`id,name,x,y,score,seq,nearby`，其中 id 为当前连接 ID，不是可持久化 PlayerID。
水晶字段：`id,x,y`。`nearby` 为 AOI 九宫格邻居数；快照没有 AOI 可见性裁剪。
`seq` 是最后接受的输入序列，不用于客户端预测回滚。前端插值显示服务端坐标，无预测。

## 测试与限制

`test/webgame_test.go` 经由真实 HTTP/WS/TCP 验证会话、路由、房间、AOI、移动计分、隔离、
断线指标、异常包、Origin、容量和关闭；`test/webgame/arena.spec.ts` 用双浏览器验证可玩链路。
这是可观察的单实例框架演示，不是“整个框架所有场景”或生产承载能力认证：未覆盖
持久化、跨节点、丢包模拟、客户端预测、负载压测。既有框架回归测试仍必须运行。

运行方式与完整目录参阅 [Web 游戏示例](../examples/webgame/README.md)。
