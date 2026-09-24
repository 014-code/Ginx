# 框架与示例边界

Ginx 是 TCP 框架，并提供可选的游戏服务组件。框架不规定账号体系、玩家成长、出生点或某个客户端的业务协议。

| 层 | 包 | 职责 |
| --- | --- | --- |
| 核心 | `gface`、`gnet` | 连接生命周期、消息分帧、路由、Worker、收发背压 |
| 基础设施 | `utils`、`limit`、`metrics` | 显式配置、限流、运行指标 |
| 可选组件 | `gcore` | AOI 空间查询、房间成员与广播、消息信封、结构化日志 |
| 可选组件 | `session`、`persist` | 单账号单会话映射、玩家形状的存储接口及内存/JSON 实现 |
| 示例应用 | `examples/gameapp`、`gameserver`、`gameclient`、`sqlitestore` | Gin 登录、TCP 鉴权、房间流程、等级/背包与 SQLite 事务 |
| 示例应用 | `examples/unity`、`unityserver`、`gameprotocol` | Unity 协议与玩家世界、访客编号、业务消息 ID |
| 兼容层 | `main/unityserver`、已弃用符号 | 保留旧启动入口和部分旧 API，不增加新业务能力 |

核心及可选组件不导入示例、Gin 或 SQLite。根 `go.mod` 仍共享示例依赖，但构建 TCP 核心不需要编译这些驱动。`persist.PlayerStore` 和 `gcore.GameMessage` 都不是使用 TCP 框架的前提；应用可以有自己的存储模型和消息体。

## 服务配置迁移

```go
config, err := utils.LoadConfig("config/ginx.json")
if err != nil {
    return err
}
server := gnet.NewServerWithConfig(config)
return server.StartWithError()
```

不使用文件时从 `gnet.DefaultConfig()` 开始修改配置。`LoadConfig` 在默认值上覆盖 JSON 字段，文件缺失或格式错误会返回错误，不修改全局对象。包导入不再触发磁盘读取。

`NewServerWithConfig` 按值复制配置，连接管理器、Worker、连接缓冲、限流、心跳和封包器使用同一份私有快照。后续修改原配置或 `utils.GlobalObject` 不影响实例。构造阶段不会监听端口或启动协程；运行期不支持热更新配置。公开的 `Server.IP`、`Port` 等旧字段仍保留，需在启动前设置，不要并发修改。

客户端可以使用 `gnet.NewDataPackWithLimit(4096)` 单独设置消息体上限；`0` 表示不限制。服务端入站和出站均按实例的 `MaxPacketSize` 校验，传输协议字节格式不变。

连接发送支持 `SendMsgContext`。`SendMsg` 可由 `SendTimeout` 限制等待时间，写协程可由 `WriteTimeout` 限制 socket 写入时间；`SendBuffMsg` 在其他发送者占用发送入口或出站队列满时立即返回 `ErrSendQueueFull`。`Server.Shutdown(ctx)` 会关闭监听、连接和 Worker，并等待已接收任务；超时返回后清理仍在后台继续。

旧 `NewServer()` 仍会调用全局 `Reload()` 搜索当前目录和父目录，文件不可读则保留原值，非法 JSON 则 panic；旧 `NewConntion`、`NewConnManager`、`NewMsgHandle`、`NewDataPack` 仍使用全局配置。这些兼容 API 不用于实例隔离或并发改配置。此前依赖“仅导入 utils 即加载文件”的应用需要改为显式加载。

## 会话与玩家身份

新应用通过 `session.New(session.Options{AccountPolicy: session.ReplaceExisting})` 创建管理器。默认 `Options{}` 使用 `RejectExisting`，已有未过期会话时 `Issue` 返回 `ErrAccountActive`；替换策略则返回旧会话，应用负责断开旧连接、清理房间。

应用验证账号、取得稳定 PlayerID 后调用 `Issue(accountID, playerID, ttl)`，再使用 `Bind(token, connID)` 绑定连接。管理器只维护映射，不分配编号、不校验密码、不启动后台回收协程。当前模型为单账号单会话，支持多设备登录需要另外的业务模型。

过期查询会失败；应用定期调用 `RemoveExpired`，根据返回快照清理连接。`Touch` 只更新活跃时间，不延长有效期。HTTP/TCP 示例完整演示这些操作；Unity 旧客户端只演示访客身份，没有完整的过期鉴权流程。

`session.NewManager` 和 `Login` 已弃用。其自动编号、立即绑定和无有效期行为集中在 `session/legacy.go`，供旧调用方兼容；新构造函数创建的管理器调用 `Login` 会返回错误。

## 协议和 Unity 迁移

`gcore.GameMessage` 保留可选的版本、序列号、PlayerID、RoomID 消息信封。消息语义和 ID 归应用所有，新代码参考 `examples/gameprotocol/messages.go`。`gcore/protocol_legacy.go` 仅保留原有常量值并标记弃用，避免现有客户端/路由突然失配。不会在兼容文件中新增业务消息。

| 原位置 | 新位置 |
| --- | --- |
| `unity/protocol.go` | `examples/unity/protocol.go` |
| `unity/world.go` | `examples/unity/world.go` |
| `main/unityserver/Server.go` 的业务实现 | `examples/unityserver/server.go` |
| Unity 启动命令 | `go run ./examples/unityserver/cmd` |

根 `unity/` 兼容包已删除，旧调用方必须将 `Ginx/unity` 改为 `Ginx/examples/unity`。旧启动命令仍调用同一个示例实现。Unity 访客编号、出生点、在线玩家同步、聊天和移动均归示例，不进入网络底层。

## 回归保护

`test/framework_boundary_test.go` 检查核心依赖方向、旧消息常量兼容、显式身份及重复登录策略，并通过真实 TCP 连接验证两个服务的配置隔离。已有 Worker、心跳、连接生命周期、Unity 协议与世界、HTTP/TCP 及 SQLite 测试继续覆盖行为。

```powershell
go test -count=1 ./...
go test -race -count=1 ./...
go vet ./...
go build ./...
pwsh -File test/start-server-smoke.ps1
```
