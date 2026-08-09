# Session And Room Guide

## English

Ginx keeps login state in the `session` package and keeps room membership in `gcore.RoomManager`.
The two concerns are deliberately separate:

- `session.Manager` maps account ID, token, player ID, and connection ID.
- `gcore.Room` owns membership, capacity, lifecycle state, and broadcast.
- `gnet.Connection` properties keep short-lived values such as `PlayerID`, `SessionToken`, and `RoomID`.
- Persistent player data must use `persist.PlayerStore`; connection properties are not a database.

### Session lifecycle

```go
manager := session.NewManager(1000)
gameSession, replaced, err := manager.Login("account-1", connection.GetConnId())
if err != nil {
    return
}
connection.SetProperty("PlayerID", gameSession.PlayerID)
connection.SetProperty("SessionToken", gameSession.Token)
_ = manager.Touch(gameSession.Token)
_, _ = manager.LogoutByConnID(connection.GetConnId())
```

Logging in the same account replaces the previous session. The returned `replaced` value lets the business layer notify or close the old connection. `Session` values returned by the manager are snapshots, so callers cannot mutate manager state accidentally.

The Unity demo currently creates a temporary account named `unity-conn-{ConnID}` during `OnConnStart`, because the legacy client has no login packet. A new client can add a real account login message and call the same manager from its router.

### Room messages

The Unity adapter reserves these message IDs:

| ID | Meaning |
| --- | --- |
| `203` | Create room request/response |
| `204` | Join room request/response |
| `205` | Leave room request/response |
| `206` | Room event broadcast |

Room requests contain a protobuf-style `RoomID` field. Responses contain `RoomID`, `Code`, `Message`, and `PlayerCount`. A successful Unity demo connection joins room `1` automatically. A room event is broadcast to every member in the room through the single connection outbound queue, so messages from the same connection retain enqueue order.

### Persistence and runtime services

`persist.PlayerStore` is the storage boundary. `persist.MemoryStore` is useful for tests, while `persist.JSONFileStore` is suitable for local development and small prototypes. Production services should implement the same interface with a database or storage service.

`limit.TokenBucket` can be used for login, movement, chat, or room-event limits. Framework-level per-connection message limiting is disabled by default and can be enabled in `config/ginx.json`:

```json
{
  "MessageRateLimit": 100,
  "MessageRateBurst": 20
}
```

`gnet.Server.GetMetrics()` returns current connections, received messages, inbound and outbound bytes, and recovered router panics.

## 中文

Ginx 将登录状态放在 `session` 包，将房间成员关系放在 `gcore.RoomManager` 中，两个职责保持分离：

- `session.Manager` 维护账号、Token、玩家 ID 和连接 ID 的映射。
- `gcore.Room` 负责成员、容量、生命周期状态和广播。
- `gnet.Connection` 属性只保存 `PlayerID`、`SessionToken`、`RoomID` 等短期会话数据。
- 玩家长期数据必须通过 `persist.PlayerStore` 保存，不能把连接属性当数据库使用。

### 会话生命周期

```go
manager := session.NewManager(1000)
gameSession, replaced, err := manager.Login("account-1", connection.GetConnId())
if err != nil {
    return
}
connection.SetProperty("PlayerID", gameSession.PlayerID)
connection.SetProperty("SessionToken", gameSession.Token)
_ = manager.Touch(gameSession.Token)
_, _ = manager.LogoutByConnID(connection.GetConnId())
```

同一个账号重复登录时，旧会话会被替换，`replaced` 可以交给业务层通知旧连接下线。管理器返回的是快照副本，调用方不会意外修改内部状态。

当前 Unity 示例客户端没有登录消息，所以示例在 `OnConnStart` 中使用 `unity-conn-{ConnID}` 作为临时账号。新客户端接入正式登录协议后，仍然复用同一个会话管理器。

### 房间消息

Unity 适配层使用以下消息 ID：

| ID | 含义 |
| --- | --- |
| `203` | 创建房间请求/响应 |
| `204` | 加入房间请求/响应 |
| `205` | 离开房间请求/响应 |
| `206` | 房间事件广播 |

请求包含 Protobuf 风格的 `RoomID` 字段，响应包含 `RoomID`、`Code`、`Message` 和 `PlayerCount`。Unity 示例连接成功后会自动加入房间 `1`。房间事件通过连接统一出站队列广播，同一连接内保持入队顺序。

### 持久化和运行时服务

`persist.PlayerStore` 是持久化边界。`persist.MemoryStore` 适合单元测试，`persist.JSONFileStore` 适合本地开发和小型原型，生产环境可用数据库实现同一接口。

`limit.TokenBucket` 可用于登录、移动、聊天和房间事件限流。框架级的单连接消息限流默认关闭，可在 `config/ginx.json` 中配置：

```json
{
  "MessageRateLimit": 100,
  "MessageRateBurst": 20
}
```

通过 `gnet.Server.GetMetrics()` 可以读取当前连接数、入站消息数、入站/出站字节数以及被恢复的路由 panic 次数。
