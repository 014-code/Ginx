# Ginx 游戏服务端使用指南

## 框架能力

当前版本提供以下基础能力：

- TCP 长度前缀协议，支持消息 ID 和二进制消息体。
- 根据消息 ID 注册路由，每个路由包含 `PreHandle`、`Handle`、`PostHandle` 三个阶段。
- 每条连接独立的读协程和写协程，业务处理在独立协程中执行。
- 可配置的 Worker 工作池，按连接 ID 将同一连接的消息分配到同一个队列。
- 单条消息的最大长度校验，避免异常长度导致无限制内存分配。
- 连接断开后自动结束连接主循环。

连接、房间、AOI、玩家状态和可靠性策略属于业务层，可以在路由和连接管理器之上继续扩展。

## 启动服务

从项目根目录启动示例服务：

```powershell
go run ./main/server
```

示例服务默认监听 `127.0.0.1:7777`。配置文件是 `config/ginx.json`，支持以下字段：

| 字段 | 作用 |
| --- | --- |
| `Name` | 服务名称 |
| `Host` | 监听地址 |
| `TcpPort` | TCP 端口 |
| `MaxConn` | 最大连接数配置 |
| `MaxPacketSize` | 单个消息体允许的最大字节数 |
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
    server.AddRouter(1001, &LoginRouter{})
    server.Serve()
}
```

游戏项目建议给消息 ID 建立集中定义，例如：

| 范围 | 示例 | 用途 |
| --- | --- | --- |
| `1000-1999` | 登录、角色、鉴权 | 账号和角色流程 |
| `2000-2999` | 大厅、匹配、好友 | 大厅服务 |
| `3000-3999` | 移动、战斗、技能 | 对局实时消息 |
| `4000-4999` | 聊天、公告、邮件 | 社交和运营消息 |

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
- 当任务队列在 `WorkerTaskQueueWaitTime` 内仍然没有空间时，当前连接会被关闭，避免读协程永久阻塞。
- 需要顺序一致性的业务应在业务层增加玩家锁、串行队列或状态机。
- 路由中不要直接操作另一个连接的底层 socket；跨玩家推送应通过连接管理器统一调度。
- 连接会在 `HeartbeatMax` 时间内没有收到完整消息时自动关闭；客户端可以通过心跳消息刷新连接活跃时间。
- 断线清理、登录态校验、重连恢复和消息幂等需要由游戏业务层实现。

## 测试和提交前检查

运行框架测试和静态检查：

```powershell
go test ./...
go vet ./...
go build ./...
```

测试文件统一位于 `test/` 目录，覆盖数据包编码解码、超长包拒绝、消息 ID 路由、Worker 分发、同连接顺序、队列满拒绝、零 Worker 回退、真实 TCP 收发以及心跳超时。

## 当前边界

当前 `Server.Stop` 仍是预留接口，服务进程通常通过进程生命周期停止。生产环境接入前还应补充监听器关闭、连接管理、连接数限制、日志和指标、优雅停服以及协议版本兼容策略。
