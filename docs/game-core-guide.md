# Ginx 游戏通用能力指南

## 中文

`gcore` 是放置游戏通用能力的新模块包。它不依赖 `gnet.Server` 的具体实现，只通过 `gface.IConnection` 完成房间广播，因此可以在不同的服务端入口和游戏业务中复用。

## AOI

AOI 使用二维网格保存实体位置。查询时返回实体所在格子及周围八个格子中的其他实体：

```go
aoi, err := gcore.NewAOIManager(1000, 1000, 10, 10)
if err != nil {
    panic(err)
}

_ = aoi.AddEntity(playerID, gcore.Position{X: 100, Y: 200})
change, err := aoi.MoveEntity(playerID, gcore.Position{X: 110, Y: 200})
if err == nil {
    // change.Entered: 新进入视野的玩家
    // change.Left:    离开视野的玩家
}
```

AOI 只负责空间关系，不负责给玩家发包。业务层拿到 `Entered` 和 `Left` 后，再从 `ConnManager` 查询连接并发送 AOI 消息。当前网格视野是九宫格，不是精确圆形距离；需要精确距离时由业务层继续过滤。

## 游戏房间

房间提供成员管理、容量限制、等待/运行/关闭状态和广播能力：

```go
rooms := gcore.NewRoomManager()
room, err := rooms.CreateRoom(1001, 10)
if err != nil {
    panic(err)
}

_ = room.Join(connection)
_ = room.Start()
broadcastErrors := room.Broadcast(2004, []byte("room state changed"))
for _, broadcastError := range broadcastErrors {
    fmt.Println("broadcast error:", broadcastError.ConnID, broadcastError.Err)
}
```

广播会先取得成员快照，再在锁外调用连接发送方法，避免慢客户端阻塞房间的加入和离开。`Broadcast` 使用 `SendBuffMsg`，队列满时会返回错误；可靠的结算、扣款和战斗结果消息应由业务层设计可靠投递。

## 游戏协议

`DataPack` 负责 TCP 分帧，`gcore.GameMessage` 是可选的游戏消息信封：协议版本、标志、消息类型、序列号、玩家 ID、房间 ID 和消息体。TCP 框架不要求使用它。以下消息常量来自 `Ginx/examples/gameprotocol`。

```go
gameMessage := gcore.NewGameMessage(
    gameprotocol.GameMsgPlayerMove,
    sequence,
    playerID,
    roomID,
    []byte(`{"x":100,"y":200}`),
)
gameMessage.Flags = gcore.GameMessageFlagReliable
payload, err := gameMessage.Encode()
if err == nil {
    _ = connection.SendBuffMsg(uint32(gameMessage.MessageID), payload)
}
```

协议使用小端序并带有 Magic、版本和长度校验。解包时使用 `DecodeGameMessage`，不要绕过校验直接读取字节。新增业务消息类型应在应用自己的协议包集中定义，可参考 `examples/gameprotocol/messages.go`。`gcore` 中同名旧常量仅用于兼容，已标记 Deprecated，不再扩展。

## 日志体系

`gcore.Logger` 输出 JSON 行日志，支持级别、服务名和结构化字段：

```go
logger := gcore.NewLogger("room-server", nil)
logger.SetLevel(gcore.LogLevelInfo)
_ = logger.Info("player entered room",
    gcore.Field("player_id", playerID),
    gcore.Field("room_id", roomID),
)
```

可以使用 `With` 固定服务实例字段：

```go
roomLogger := logger.With(gcore.Field("room_id", roomID))
_ = roomLogger.Warn("room is almost full", gcore.Field("players", room.PlayerCount()))
```

日志模块只负责结构化输出和并发写入，不负责日志文件轮转、压缩、远程采集或告警。生产环境应把输出交给进程管理器或日志采集系统。

## 组合边界

- `gnet` 负责连接、收发、路由和 Worker；`gcore` 不启动网络协程。
- `gcore.AOIManager` 不保存 `IConnection`，避免空间数据和网络状态耦合。
- `gcore.Room` 可以保存连接接口并执行广播，但玩家登录态和房间业务状态仍由游戏服务负责。
- `GameMessage.Payload` 只承载一条业务消息；大型资源、录像和持久化数据不要直接放进消息体。
- 连接属性适合保存短期会话数据，不要把数据库连接或全局服务对象塞进属性表。

## English

`gcore` contains reusable game-server building blocks: grid-based AOI, room membership and broadcast, a versioned game message envelope, and structured JSON logging.

- `AOIManager` tracks positions and returns nearby entity changes. It does not know about network connections.
- `Room` and `RoomManager` handle membership, capacity, lifecycle state, and broadcasts.
- `GameMessage` adds version, flags, sequence, player ID, room ID, and payload metadata above the transport-level `DataPack`.
- `Logger` writes thread-safe JSON lines and leaves rotation, shipping, and alerting to the deployment environment.

Use the package as a set of composable primitives. Keep authentication, combat state, persistence, reliable delivery, and exact game rules in the game service layer.
