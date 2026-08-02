# Ginx

## English

Ginx is a lightweight Go game-server framework with length-prefixed messages, message-ID routing, per-connection read/write goroutines, configurable worker pools, and router lifecycle hooks.

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

Run the automated checks with:

```powershell
go test ./...
go vet ./...
go build ./...
```

See the [game server guide](docs/game-server-guide.md) for the message protocol, router setup, concurrency notes, and current production boundaries.

## 中文

Ginx 是一个轻量级 Go 游戏服务端框架，提供长度前缀消息协议、按消息 ID 路由、单连接读写协程、可配置 Worker 工作池和路由生命周期钩子。

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

运行自动化测试、静态检查和构建验证：

```powershell
go test ./...
go vet ./...
go build ./...
```

消息协议、路由注册、并发注意事项和当前生产使用边界，请参阅[游戏服务端使用指南](docs/game-server-guide.md)。
