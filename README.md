# Ginx

## Runtime Services

The framework also includes `session`, `limit`, `metrics`, and `persist` packages. See [Session And Room Guide](docs/session-room-guide.md) for login sessions, room messages, rate limiting, metrics, and player persistence.

### 中文运行时服务

框架同时提供 `session`、`limit`、`metrics` 和 `persist` 包。登录会话、房间协议、限流、指标以及玩家持久化的使用方式，请参阅 [会话与房间指南](docs/session-room-guide.md)。

## English

Ginx is a lightweight Go game-server framework with length-prefixed messages, message-ID routing, per-connection read/write goroutines, configurable worker pools, buffered outbound messages, connection properties, connection lifecycle hooks, and graceful shutdown.

### Quick start

Run commands from the repository root so `config/ginx.json` is loaded:

```powershell
go run ./main/server
```

The example server registers message IDs `0` and `1`. Run the matching clients in separate terminals:

```powershell
go run ./main/client0
go run ./main/client1
```

A complete tutorial example is available in `main/tutorial`:

```powershell
go run ./main/tutorial/server
go run ./main/tutorial/client
```

Run the server and client in separate terminals. The client sends a ping and a heartbeat message every two seconds.

### Unity MMO client

The repository contains a backend compatible with the legacy Unity MMO demo protocol. Start it from the repository root:

```powershell
go run ./main/unityserver
```

Open `Assets/Scene/Login.unity` in the Unity `5.4.1f1` client project, then connect to `127.0.0.1:7777`. The Unity server supports player-ID assignment, existing-player synchronization, join/leave broadcasts, chat, and movement messages.

See the [Unity client compatibility guide](docs/unity-client-compatibility.md) for the startup sequence, packet format, Protobuf messages, message flow, and troubleshooting notes.

Run the automated checks with:

```powershell
go test ./...
go vet ./...
go build ./...
```

See the [game server guide](docs/game-server-guide.md) for the message protocol, router setup, concurrency notes, and current production boundaries.

See the [game core guide](docs/game-core-guide.md) for AOI, room management, game protocol messages, and structured logging.

## 中文

Ginx 是一个轻量级 Go 游戏服务端框架，提供长度前缀消息协议、按消息 ID 路由、单连接读写协程、可配置 Worker 工作池、带缓冲发包、连接属性、连接生命周期钩子和优雅停服。

### 快速开始

请在项目根目录运行以下命令，确保能加载 `config/ginx.json`：

```powershell
go run ./main/server
```

示例服务注册了消息 ID `0` 和 `1`。请在两个独立终端中运行对应客户端：

```powershell
go run ./main/client0
go run ./main/client1
```

完整教程示例位于 `main/tutorial` 目录：

```powershell
go run ./main/tutorial/server
go run ./main/tutorial/client
```

请在两个独立终端中运行服务端和客户端。客户端每两秒发送一次 ping 消息和心跳消息。

### Unity MMO 客户端

项目提供了兼容旧版 Unity MMO 示例客户端协议的服务端。在项目根目录执行：

```powershell
go run ./main/unityserver
```

使用 Unity `5.4.1f1` 打开客户端项目中的 `Assets/Scene/Login.unity`，然后连接 `127.0.0.1:7777`。Unity 兼容服务端支持玩家 ID 分配、已有玩家同步、玩家加入和离线广播、聊天及移动消息。

完整启动顺序、数据包格式、Protobuf 消息、消息时序和排查方式，请参阅[Unity 客户端兼容说明](docs/unity-client-compatibility.md)。

运行自动化测试、静态检查和构建验证：

```powershell
go test ./...
go vet ./...
go build ./...
```

消息协议、路由注册、并发注意事项和当前生产使用边界，请参阅[游戏服务端使用指南](docs/game-server-guide.md)。

AOI、游戏房间、游戏协议和日志体系的使用方式，请参阅[游戏通用能力指南](docs/game-core-guide.md)。
### 运行时服务

框架同时提供 `session`、`limit`、`metrics` 和 `persist` 包。登录会话、房间协议、限流、指标以及玩家持久化的使用方式，请参阅 [会话与房间指南](docs/session-room-guide.md)。
