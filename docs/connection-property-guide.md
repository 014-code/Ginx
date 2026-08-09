# Ginx 连接属性指南

## 中文

### 作用

连接属性用于给当前连接绑定轻量级的会话数据，例如玩家 ID、房间 ID、登录状态和场景名称。属性保存在 `Connection` 内部，不需要把 `TcpServer` 反向引用塞进连接对象。

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

### 接口行为

| 方法 | 行为 |
| --- | --- |
| `SetProperty(key, value)` | 设置属性；同名键会覆盖旧值。 |
| `GetProperty(key)` | 返回属性值；属性不存在时返回错误。 |
| `RemoveProperty(key)` | 删除属性；删除不存在的键不会报错。 |

属性值类型是 `interface{}`，读取时要检查类型：

```go
value, err := connection.GetProperty("PlayerID")
if err != nil {
    return
}
playerID, ok := value.(uint64)
if !ok {
    return
}
fmt.Println(playerID)
```

属性读写由连接内部的读写锁保护，可以同时被路由、连接 Hook 和业务协程访问。属性不是永久存储，连接断开后应在 `OnConnStop` 中完成业务清理或把必要数据写入持久化系统。

### 启动 Hook 的执行顺序

当前服务器会先启动连接读写协程，再调用 `OnConnStart`，因此下面的写法可以正常发送欢迎消息：

```go
server.SetOnConnStart(func(connection gface.IConnection) {
    connection.SetProperty("Name", "Ginx Player")
    if err := connection.SendMsg(2, []byte("connection ready")); err != nil {
        fmt.Println(err)
    }
})
```

不要在属性中保存大型对象、数据库连接、全局 Server 或其他拥有复杂生命周期的资源。玩家数据、重连恢复和持久化应该由业务层负责。

### 相关测试

属性测试位于 `test/connection_property_test.go`，覆盖设置、读取、覆盖、空值、删除、并发访问，以及从 `OnConnStart` 写入并在 `OnConnStop` 读取属性。

## English

Connection properties are lightweight, per-connection session metadata. Typical values include a player ID, room ID, login state, or scene name.

```go
connection.SetProperty("PlayerID", uint64(10001))
value, err := connection.GetProperty("PlayerID")
connection.RemoveProperty("PlayerID")
```

`SetProperty` overwrites an existing key. `GetProperty` returns an error when the key is missing. `RemoveProperty` is idempotent. Access is protected by an internal read/write lock, but the value is still an `interface{}` and must be type-checked by the caller.

The `OnConnStart` hook runs after the connection reader and writer goroutines have started, so it can initialize properties and send a welcome message. The `OnConnStop` hook can read the properties and release business state. Properties are not persistence; save important player state in the business layer before the connection is gone.
