# Ginx 游戏服务端使用指南

## 框架能力

当前版本提供以下基础能力：

- TCP 长度前缀协议，支持消息 ID 和二进制消息体。
- `gcore` 提供 AOI、游戏房间、游戏协议和结构化日志等通用游戏服务能力。
- 根据消息 ID 注册路由，每个路由包含 `PreHandle`、`Handle`、`PostHandle` 三个阶段。
- 每条连接独立的读协程和写协程，业务处理在独立协程中执行。
- 提供阻塞式 `SendMsg` 和非阻塞式 `SendBuffMsg` 两种服务端回包方式，二者共享同一条连接出站队列。
- 可配置的 Worker 工作池，按连接 ID 将同一连接的消息分配到同一个队列。
- 单条消息的最大长度校验，避免异常长度导致无限制内存分配。
- 连接管理器支持按 ID 获取、移除和批量关闭连接，并通过 `MaxConn` 限制在线连接数。
- 每条连接支持并发安全的自定义属性，可保存玩家 ID、房间 ID 和登录状态等会话数据。
- 可注册连接建立与断开钩子，用于初始化会话、上线通知和断线清理。
- `Server.Stop()` 会停止接收新连接、关闭所有连接并等待 Worker 队列完成已入队任务。

连接、房间、AOI、玩家状态和可靠性策略属于业务层，可以在路由和连接管理器之上继续扩展。

## 启动服务

从项目根目录启动示例服务：

```powershell
go run ./main/tutorial/server
```

示例服务默认监听 `127.0.0.1:7777`。配置文件是 `config/ginx.json`，支持以下字段：

| 字段 | 作用 |
| --- | --- |
| `Name` | 服务名称 |
| `Host` | 监听地址 |
| `TcpPort` | TCP 端口 |
| `MaxConn` | 最大连接数配置 |
| `MaxPacketSize` | 单个消息体允许的最大字节数 |
| `MaxMsgChanLen` | 每条连接的缓冲发包队列容量 |
| `WorkerPoolSize` | Worker 工作池数量，设置为 `0` 时关闭工作池 |
| `MaxWorkerTaskLen` | 每个 Worker 任务队列的最大容量 |
| `WorkerTaskQueueWaitTime` | 任务队列满时的最大等待时间，单位毫秒 |
| `HeartbeatMax` | 连接最大空闲时间，单位秒，设置为 `0` 时关闭 |

如果配置文件不可用，框架会保留代码中的默认值。部署时建议把配置文件和服务启动目录固定下来，并显式填写 `MaxPacketSize`。

## 注册游戏消息

服务端通过消息 ID 将协议消息分配给业务路由：

```go
type LoginRouter struct {
    gnet.BaseRouter
}

func (r *LoginRouter) Handle(request gface.IRequest) {
    playerData := request.GetData()
    _ = playerData

    err := request.GetConnection().SendMsg(1002, []byte("login accepted"))
    if err != nil {
        fmt.Println(err)
    }
}

func main() {
    server := gnet.NewServer()
    server.SetOnConnStart(func(connection gface.IConnection) {
        fmt.Println("player connected:", connection.GetConnId())
    })
    server.SetOnConnStop(func(connection gface.IConnection) {
        fmt.Println("player disconnected:", connection.GetConnId())
    })
    server.AddRouter(1001, &LoginRouter{})
    server.Serve()
}
```

## 完整教程示例

`main/tutorial` 提供一个可以直接运行的服务端和客户端示例：

```powershell
go run ./main/tutorial/server
go run ./main/tutorial/client
```

请在两个独立终端中运行。服务端注册消息 `0` 和 `1` 两个路由，客户端每两秒发送一次 ping 和 heartbeat 消息，服务端分别返回 `pong` 和 heartbeat 确认。这个示例同时展示了 `DataPack` 的封包/拆包、Worker Pool 的消息处理以及 `HeartbeatMax` 的活跃时间刷新。

示例源码：

- `main/tutorial/server/Server.go`
- `main/tutorial/client/Client.go`

## 连接管理和停服

`GetConnMgr()` 返回连接管理器，可在游戏业务中按连接 ID 查询连接并向指定玩家发送消息。连接 ID 由服务端递增分配，连接断开后会自动从管理器移除。

```go
connection, err := server.GetConnMgr().Get(playerConnID)
if err == nil {
    _ = connection.SendBuffMsg(4001, []byte("server announcement"))
}
```

当 `MaxConn > 0` 时，达到上限后的新 TCP 连接会立即关闭；`MaxConn <= 0` 表示不限制连接数。应用关闭时调用 `server.Stop()`：监听器先关闭，现有连接随后关闭，已经进入 Worker 队列的任务会被处理完成后退出。

`SendMsg` 和 `SendBuffMsg` 共享每条连接的出站队列，并由同一个写协程顺序写出。`SendMsg` 在队列满时等待，适合关键消息；`SendBuffMsg` 队列满时立即返回错误，适合广播和实时状态。两个方法混用时，已经成功入队的消息仍按入队顺序发送；多个业务协程同时调用时，需要由业务层决定调用顺序。连接停止时尚未写出的消息会被丢弃，关键数据不应只依赖内存中的发送队列。

游戏项目建议给消息 ID 建立集中定义，例如：

| 范围 | 示例 | 用途 |
| --- | --- | --- |
| `1000-1999` | 登录、角色、鉴权 | 账号和角色流程 |
| `2000-2999` | 大厅、匹配、好友 | 大厅服务 |
| `3000-3999` | 移动、战斗、技能 | 对局实时消息 |
| `4000-4999` | 聊天、公告、邮件 | 社交和运营消息 |

## 连接属性

连接属性适合保存当前连接的轻量级会话数据。属性只属于当前连接，连接关闭后不会自动持久化：

```go
server.SetOnConnStart(func(connection gface.IConnection) {
    connection.SetProperty("PlayerID", uint64(10001))
    connection.SetProperty("Home", "lobby")
})

server.SetOnConnStop(func(connection gface.IConnection) {
    playerID, err := connection.GetProperty("PlayerID")
    if err == nil {
        fmt.Println("disconnect player:", playerID)
    }
    connection.RemoveProperty("Home")
})
```

`SetProperty` 会覆盖同名属性，`GetProperty` 找不到属性时返回错误，`RemoveProperty` 删除不存在的属性不会报错。属性读写由连接内部的读写锁保护，可以在路由、连接 Hook 和业务协程之间安全访问；属性值仍然是 `interface{}`，读取后应进行类型断言并检查结果。

不要把大型对象、数据库连接、全局服务或需要长期保存的数据直接放入连接属性。玩家状态持久化和断线恢复应由业务层负责。

## 消息协议

每条消息的字节布局如下，整数使用小端序：

```text
4 bytes  data length
4 bytes  message id
N bytes  data
```

客户端和服务端都应先读取 8 字节消息头，再根据 `data length` 读取消息体。TCP 是字节流，不能假设一次 `Read` 就能拿到完整消息；示例客户端使用 `io.ReadFull` 处理这一点。

## 并发和业务边界

- 同一连接的网络读取和网络写入分别由独立协程负责。
- 当 `WorkerPoolSize > 0` 时，服务启动阶段会创建 Worker 工作池；连接消息通过 `ConnID % WorkerPoolSize` 分配到任务队列。
- 同一连接的消息进入同一个 Worker 队列，因此同一连接内保持入队顺序；不同连接可以并行处理。
- 当 `WorkerPoolSize = 0` 时，每条入站消息回退为临时协程处理。
- `SendBuffMsg` 的队列容量由 `MaxMsgChanLen` 控制，队列满时业务应记录错误、丢弃低优先级消息或交给业务层重试队列。
- 当任务队列在 `WorkerTaskQueueWaitTime` 内仍然没有空间时，当前连接会被关闭，避免读协程永久阻塞。
- 需要顺序一致性的业务应在业务层增加玩家锁、串行队列或状态机。
- 路由中不要直接操作另一个连接的底层 socket；跨玩家推送应通过连接管理器统一调度。
- 连接会在 `HeartbeatMax` 时间内没有收到完整消息时自动关闭；客户端可以通过心跳消息刷新连接活跃时间。
- 调用 `Server.Stop()` 后不再接收新连接；连接关闭钩子可用于持久化玩家状态或回收房间资源。
- 断线清理、登录态校验、重连恢复和消息幂等需要由游戏业务层实现。

## 测试和提交前检查

运行框架测试和静态检查：

```powershell
go test ./...
go vet ./...
go build ./...
```

测试文件统一位于 `test/` 目录，覆盖数据包编码解码、超长包拒绝、消息 ID 路由、Worker 分发、同连接顺序、队列满拒绝、Worker 停服排空、零 Worker 回退、真实 TCP 收发、带缓冲发包、发送队列满、连接属性读写、属性并发访问、Hook 属性传递、心跳超时、连接管理和服务生命周期。

## 当前边界

当前版本已提供基础的连接管理、连接数限制、生命周期钩子和优雅停服。生产环境接入前还应补充结构化日志与指标、进程信号处理、房间/玩家状态持久化、连接级背压策略以及协议版本兼容策略。
