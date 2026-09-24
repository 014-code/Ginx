# Session And Room Guide

## English

Ginx keeps login state in the `session` package and keeps room membership in `gcore.RoomManager`.
The two concerns are deliberately separate:

- `session.Manager` maps account ID, token, player ID, and connection ID.
- `gcore.Room` owns membership, capacity, lifecycle state, and broadcast.
- `gnet.Connection` properties keep short-lived values such as `PlayerID`, `SessionToken`, and `RoomID`.
- Persistent data belongs to the application. `persist.PlayerStore` is an optional storage boundary; connection properties are not a database.

### Session lifecycle

```go
manager, err := session.New(session.Options{AccountPolicy: session.ReplaceExisting})
if err != nil {
    return
}
// The application resolves this stable ID after verifying credentials.
gameSession, replaced, err := manager.Issue("account-1", 1001, time.Hour)
if err != nil {
    return
}
_ = replaced // The application must clean up a replaced connection/room, if present.
if _, err := manager.Bind(gameSession.Token, connection.GetConnId()); err != nil {
    _, _ = manager.LogoutByToken(gameSession.Token)
    return
}
connection.SetProperty("PlayerID", gameSession.PlayerID)
connection.SetProperty("SessionToken", gameSession.Token)
_ = manager.Touch(gameSession.Token)
_, _ = manager.LogoutByConnID(connection.GetConnId())
```

This example explicitly chooses `ReplaceExisting`. The default `session.Options{}` rejects an existing unexpired session with `ErrAccountActive`. The returned `replaced` value lets the business layer notify or close the old connection and clean up rooms. `Session` values are snapshots. This is a single-account, single-session component, not a multi-device login model. Expiration invalidates lookups; the application schedules `RemoveExpired` and closes affected connections. `Touch` does not extend the absolute expiry.

The Unity example creates temporary accounts named `unity-conn-{ConnID}` and allocates visitor IDs itself during `OnConnStart`, because the legacy client has no login packet. Its token has a 24-hour validity, but the legacy movement/chat routes do not enforce expiration or run a sweeper. The HTTP/TCP example demonstrates enforced expiry. Deprecated `NewManager` / `Login` retain automatic IDs for existing callers only.

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

`persist.PlayerStore` is an optional player-shaped storage interface. `persist.MemoryStore` is useful for tests, while `persist.JSONFileStore` is suitable for local development and small prototypes. Applications may implement this interface or use their own storage model. `examples/sqlitestore` demonstrates transactions; levels and inventory belong to `examples/gameapp`.

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
- 玩家长期数据由应用负责，可选用 `persist.PlayerStore` 或自有存储模型，不能把连接属性当数据库使用。

### 会话生命周期

```go
manager, err := session.New(session.Options{AccountPolicy: session.ReplaceExisting})
if err != nil {
    return
}
// 应用验证账号后取得稳定玩家 ID，管理器不负责分配。
gameSession, replaced, err := manager.Issue("account-1", 1001, time.Hour)
if err != nil {
    return
}
_ = replaced // 非空时由应用清理旧连接和房间。
if _, err := manager.Bind(gameSession.Token, connection.GetConnId()); err != nil {
    _, _ = manager.LogoutByToken(gameSession.Token)
    return
}
connection.SetProperty("PlayerID", gameSession.PlayerID)
connection.SetProperty("SessionToken", gameSession.Token)
_ = manager.Touch(gameSession.Token)
_, _ = manager.LogoutByConnID(connection.GetConnId())
```

上例显式选择 `ReplaceExisting`，同账号重复登录会替换旧会话，应用通过 `replaced` 清理旧连接和房间。默认 `session.Options{}` 使用 `RejectExisting`，已有未过期会话时返回 `ErrAccountActive`。管理器返回快照副本；当前组件是单账号单会话模型，不支持多设备登录。到期后查询失效，应用需定期调用 `RemoveExpired` 并清理连接；`Touch` 不延长绝对有效期。

Unity 示例客户端没有登录消息，所以示例在 `OnConnStart` 中自行分配访客编号，并使用 `unity-conn-{ConnID}` 作为临时账号。Token 有效期为 24 小时，但旧移动/聊天路由没有实现过期鉴权和回收；完整过期处理见 HTTP/TCP 示例。旧 `NewManager` / `Login` 仅保留自动编号兼容行为，已标记弃用。

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

`persist.PlayerStore` 是可选的玩家存储接口。`persist.MemoryStore` 适合单元测试，`persist.JSONFileStore` 适合本地开发和小型原型；应用可实现同一接口，也可使用自己的存储模型。`examples/sqlitestore` 演示事务适配，等级和背包规则位于 `examples/gameapp`。

`limit.TokenBucket` 可用于登录、移动、聊天和房间事件限流。框架级的单连接消息限流默认关闭，可在 `config/ginx.json` 中配置：

```json
{
  "MessageRateLimit": 100,
  "MessageRateBurst": 20
}
```

通过 `gnet.Server.GetMetrics()` 可以读取当前连接数、入站消息数、入站/出站字节数以及被恢复的路由 panic 次数。
