# Ginx

## 协议开发工具链

`tools/protocolgen` 提供 Python 标准库实现的通用协议生成器：从 JSON 定义生成 Go
消息常量/请求响应结构、Python/C# 常量、协议表和定义快照，并检查重复编号、字段类型、
协议兼容性及生成物是否过期。它不进入服务端运行时，不需要 Redis 或 Python 后端服务。

```powershell
python tools/protocolgen/generate.py
python tools/protocolgen/generate.py --check
```

需要 Python 3.10+；生成物随源码保存，正常编译/运行 Go 服务不需要 Python。
定义格式、自定义输出和 Docker 命令见 [协议工具链](docs/protocol-toolchain.md)。

## HTTP + TCP game server

The application under `examples/` demonstrates Gin HTTP login, TCP authentication, rooms and SQLite-backed progression. These business rules are not framework APIs.
See [HTTP/TCP guide](docs/http-tcp-guide.md) for account setup, Docker commands and the protocol.

新增 Gin 联合入口：HTTP 登录获取 Token，再通过 TCP 鉴权和加入房间。
完整示例位于 `examples/`：SQLite 事务存档、一次性新手奖励、等级和背包消耗。
这些规则只属于示例，不进入 `gnet`、`gcore` 或框架存储接口。
账号配置、启动命令、接口协议及验证方式见 [HTTP 与 TCP 联合入口](docs/http-tcp-guide.md)。

Windows 一键启动：先开启 Docker Desktop（Linux 容器），再双击根目录的
`start-server.cmd`（需要 PowerShell 7）。无需本机安装 Go；首次引导创建账号，
随后自动构建并后台启动 HTTP/TCP 服务。命令行可运行 `./start-server.ps1`，
停止服务执行 `docker stop ginx-game`。默认仅监听本机，详细参数见上述指南。

框架核心只负责传输、连接和路由；Unity 世界、消息 ID、账号和成长规则位于 `examples/`。
新服务使用 `gnet.NewServerWithConfig`，由应用显式加载配置。Unity 使用 `Ginx/examples/unity`，旧 `Ginx/unity` 导入路径已移除；旧启动命令仍可用。
包职责、接口迁移和兼容范围见 [框架与示例边界](docs/framework-boundaries.md)。

## Runtime Services

The transport uses `limit` and `metrics`; `session`, `persist` and `gcore` are optional reusable components. See [Session And Room Guide](docs/session-room-guide.md) for login sessions, room messages, rate limiting, metrics, and player persistence.

### 中文运行时服务

`limit`、`metrics` 提供限流和指标；`session`、`persist`、`gcore` 是按需组合的可选组件。登录会话、房间协议、限流、指标以及玩家持久化的使用方式，请参阅 [会话与房间指南](docs/session-room-guide.md)。

## English

Ginx is a lightweight Go game-server framework with length-prefixed messages, message-ID routing, per-connection read/write goroutines, configurable worker pools, buffered outbound messages, connection properties, connection lifecycle hooks, and graceful shutdown.

### Quick start

Run the complete tutorial from the repository root so `config/ginx.json` is loaded:

```powershell
go run ./main/tutorial/server
go run ./main/tutorial/client
```

Run the server and client in separate terminals. The client sends a ping and a heartbeat message every two seconds.

### Unity MMO client

The repository contains a backend compatible with the legacy Unity MMO demo protocol. Start it from the repository root:

```powershell
go run ./examples/unityserver/cmd
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

请在项目根目录运行完整教程示例，确保能加载 `config/ginx.json`：

```powershell
go run ./main/tutorial/server
go run ./main/tutorial/client
```

请在两个独立终端中运行服务端和客户端。客户端每两秒发送一次 ping 消息和心跳消息。

### Unity MMO 客户端

项目提供了兼容旧版 Unity MMO 示例客户端协议的服务端。在项目根目录执行：

```powershell
go run ./examples/unityserver/cmd
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

`limit`、`metrics` 提供限流和指标；`session`、`persist`、`gcore` 是按需组合的可选组件。登录会话、房间协议、限流、指标以及玩家持久化的使用方式，请参阅 [会话与房间指南](docs/session-room-guide.md)。
