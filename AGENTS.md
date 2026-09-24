# Ginx Agent 开发指南

本文档用于指导 Agent 在 Ginx 项目中开发、测试和维护代码。修改代码前应先阅读本文档，并结合当前源码确认接口，不要直接照搬旧版 Zinx 教程中的实现。

## 项目定位

Ginx 是一个面向游戏服务端的 Go TCP 框架，当前已经具备以下能力：

- TCP 长度前缀消息协议。
- 消息 ID 路由和 `PreHandle`、`Handle`、`PostHandle` 生命周期。
- 每条连接独立的读协程和写协程。
- 支持无缓冲 `SendMsg` 和有缓冲 `SendBuffMsg` 两种出站消息发送方式。
- 每条连接支持并发安全的字符串键值属性，用于保存会话级业务数据。
- 可配置 Worker 工作池和任务队列背压。
- 连接管理器、最大连接数限制和连接生命周期钩子。
- 心跳超时和 `Server.Stop()` 优雅停服。

连接、玩家、房间、AOI、战斗状态和数据持久化属于业务层，不要把这些业务逻辑直接塞进框架底层。

## 目录职责

| 目录 | 职责 |
| --- | --- |
| `gface/` | 对外接口定义，例如 `IConnection`、`IServer`、`IConnManager`、`IRouter`。 |
| `gnet/` | 框架核心实现，包括 `Server`、`Connection`、`ConnManager`、`MsgHandle`、协议封包。 |
| `gcore/` | 可选游戏通用能力：AOI、房间、消息信封和结构化日志，不制定玩法。 |
| `examples/` | HTTP/TCP 与 Unity 示例、业务消息 ID、成长规则及 SQLite 适配器，不属于框架 API。 |
| `utils/` | 默认配置、显式文件加载和旧全局配置兼容，不在导入时读取磁盘。 |
| `config/` | 默认运行配置，文件名为 `ginx.json`。 |
| `main/` | 可运行的服务端、客户端和教程示例。 |
| `test/` | 所有框架测试，新增测试统一放在这里。 |
| `docs/` | 游戏服务端使用说明和教程文档。 |
| `tools/protocolgen/` | Python 标准库协议生成、定义校验和兼容性检查，构建期使用。 |
| `schema/` | 应用协议定义，消息 ID 和业务字段不属于框架 API。 |
| `wsbridge/` | 可选固定上游 WebSocket→TCP 桥接，独立关闭生命周期；不实现身份或玩法。 |

## 核心架构

### Server 和 Connection 的关系

当前版本由 `Server` 统一管理连接生命周期：

```text
Server.Start()
    -> StartWorkerPool()
    -> AcceptTCP()
    -> NewConntion()
    -> ConnManager.Add(connection)
    -> startConnection()
        -> startWriter()
        -> OnConnStart
        -> startReader()
        -> wait()
        -> OnConnStop
        -> ConnManager.Remove(connection.GetConnId())
```

`Connection` 不持有 `TcpServer`，也不在 `NewConntion()` 中自行加入连接管理器。这样可以避免构造函数中的空指针、重复注册和连接数限制绕过。

连接删除使用连接 ID：

```go
s.connMgr.Remove(connection.GetConnId())
```

当前接口是 `Remove(connID uint32)`，不能写成 `Remove(connection)`。如果教程中出现 `c.TcpServer.GetConnMgr().Remove(c)`，那是另一种旧版设计，不能直接复制到当前代码。

### 连接管理器

`ConnManager.Add()` 内部使用读写锁保护连接表，并负责检查连接 ID 重复和 `MaxConn` 限制。

- `MaxConn > 0`：限制最大连接数。
- `MaxConn <= 0`：不限制连接数。
- `ClearConn()` 会先清理管理器，再调用所有连接的 `Stop()`。

不要在 `Server` 中先用 `Len()` 判断、再调用 `Add()` 来实现限制，这会产生重复逻辑和并发时序问题。连接是否能加入，应以 `Add()` 的返回错误为准。

### 连接属性

连接属性通过以下接口操作：

```go
connection.SetProperty("PlayerID", uint64(10001))
value, err := connection.GetProperty("PlayerID")
connection.RemoveProperty("PlayerID")
```

属性表由连接内部的读写锁保护，`SetProperty` 会覆盖同名值，`GetProperty` 找不到值时返回错误，`RemoveProperty` 删除不存在的键不会报错。读取 `interface{}` 属性时必须检查类型断言，不要假设属性一定存在或类型一定正确。属性只服务于当前连接的短期会话数据，不能替代玩家持久化、数据库连接或全局服务容器。

Server 会先启动写协程，再执行 `OnConnStart`，最后启动读协程。Hook 中可以发送欢迎消息并初始化属性，不能等待客户端回复；`OnConnStop` 适合读取属性并清理业务状态。

### gcore 游戏能力模块

`gcore` 是独立于网络生命周期的游戏通用模块：

- `AOIManager` 只维护实体坐标、网格和邻近实体，不直接操作连接。
- `Room` 和 `RoomManager` 负责成员、容量、状态和广播，不负责玩家登录和游戏帧循环。
- `GameMessage` 是 DataPack 之上的可选消息信封；具体业务消息 ID 由应用定义，参考 `examples/gameprotocol`。`gcore` 同名旧常量只保留兼容，不再扩展。
- `Logger` 输出 JSON 行日志，不负责文件轮转、远程传输和日志采集。

新增游戏业务时优先组合这些模块，不要把房间状态、AOI 网格或日志文件逻辑塞回 `gnet.Connection`。

### Worker 工作池

- `WorkerPoolSize > 0` 时，服务启动阶段创建 Worker 和任务队列。
- 消息根据 `ConnID % WorkerPoolSize` 分配队列。
- 同一连接进入同一个队列，从而保持入队顺序。
- `WorkerPoolSize == 0` 时，消息回退到临时协程处理。
- `WorkerTaskQueueWaitTime` 控制任务队列满时的等待时间。
- `StopWorkerPool()` 会关闭队列并排空已经入队的任务。

路由中不要长时间阻塞 Worker；耗时任务应拆分、异步化或交给业务层专用队列，否则会影响同一 Worker 上的其他连接消息。

### 消息协议

协议使用小端序，数据布局为：

```text
4 bytes  data length
4 bytes  message id
N bytes  data
```

TCP 是字节流，读取消息头和消息体时必须使用 `io.ReadFull` 或等价的完整读取逻辑。不要假设一次 `Read` 就能拿到完整数据包。所有数据包长度都必须经过 `MaxPacketSize` 校验。

## 代码风格

- 延续现有代码风格，注释优先使用简短中文注释。
- 保留当前公开 API 名称，例如已有的 `NewConntion` 拼写不要在无迁移计划时改名。
- 变量名和接收者遵循现有风格，例如 `c`、`s`、`mh`。
- 使用 `gofmt`，不要手工调整 Go 排版。
- 优先复用 `gface` 接口和 `gnet` 现有实现，不要为单个功能新增平行抽象。
- 修改范围保持集中，不要顺手重构无关文件或改动已有配置语义。
- 不要在构造函数中隐藏网络注册、启动协程或其他难以感知的副作用，除非当前模块已有明确约定。
- 不要删除用户已有的未提交改动；先查看 `git status` 和 `git diff`，在现有改动上继续工作。

## 测试约定

新增单元测试统一放到 `test/` 目录，测试包名保持 `package test`。网络测试应：

- 使用随机可用端口，避免占用固定服务端口。
- 为拨号、读取和等待连接状态设置超时。
- 使用 `t.Cleanup()` 关闭客户端、连接、Worker 和服务端资源。
- 保存并恢复 `utils.GlobalObject`，避免配置污染其他测试。
- 对同一连接的消息顺序、队列满、连接数限制和停服行为进行回归验证。

提交前运行：

```powershell
Get-ChildItem -Recurse -File -Filter *.go gcore,gface,gnet,main,test | ForEach-Object { gofmt -w $_.FullName }
go test -count=1 ./...
go vet ./...
go build ./...
git diff --check
```

如果环境支持 C 编译器，再运行：

```powershell
go test -race -count=1 ./...
```

## 配置注意事项

新代码使用 `gnet.NewServerWithConfig(config)`，配置由 `gnet.DefaultConfig()` 或 `utils.LoadConfig(path)` 显式提供。服务持有独立快照，向连接、Worker、连接管理器和封包器传递；包导入和新构造函数不读取文件。文件加载失败必须由应用处理。

旧 `gnet.NewServer()` 仍调用 `utils.GlobalObject.Reload()`，从当前目录或父目录加载 `config/ginx.json`，仅供兼容。不要把旧全局 API 用于多实例或运行时并发改配置。示例默认相对路径，应从项目根目录运行。完整迁移说明见 `docs/framework-boundaries.md`。

主要配置字段：

| 字段 | 说明 |
| --- | --- |
| `TcpPort` | 服务监听端口。 |
| `MaxConn` | 最大连接数，非正数表示不限制。 |
| `MaxPacketSize` | 单个消息体最大长度。 |
| `MaxMsgChanLen` | 每条连接的缓冲发包队列容量。 |
| `WorkerPoolSize` | Worker 数量，`0` 表示关闭 Worker 池。 |
| `MaxWorkerTaskLen` | 每个 Worker 队列容量。 |
| `WorkerTaskQueueWaitTime` | 队列满时的等待毫秒数。 |
| `HeartbeatMax` | 连接读超时时间，`0` 表示关闭心跳超时。 |
| `WriteTimeout` | 单包 socket 写入超时，单位毫秒，`0` 表示不限制。 |
| `SendTimeout` | `SendMsg` 等待发送者和队列的超时，单位毫秒，`0` 表示不限制。 |

`SendMsgContext` 和 `Shutdown(ctx)` 通过可选接口 `gface.ContextConnection`、`gface.ShutdownServer` 暴露，旧接口实现无需新增方法。`gface.RequestContext(request)` 在连接关闭时取消；路由需要将它传入可取消操作。`Shutdown` 超时仅结束等待，清理继续；`Stop()` 无限等待同一清理流程。不要在路由或连接 Hook 中同步等待本服务停服。

## Agent 工作流程

1. 先读取 `AGENTS.md`、`README.md` 和相关模块源码。
2. 执行 `git status --short`，确认工作区中哪些改动属于用户。
3. 使用 `rg` 定位接口、调用方和已有测试。
4. 修改前说明影响范围，使用 `apply_patch` 做小范围编辑。
5. 先运行相关测试，再运行完整测试、`go vet` 和 `go build`。
6. 检查 `git diff --check`，确认没有无关文件和格式问题。
7. 只有用户明确要求时才创建 commit 或推送远端；提交信息使用简短的英文 Conventional Commit 格式，例如：

```text
feat(1.1v): add room broadcast support
fix(connection): prevent send after close
test(worker): cover queue shutdown
```

任何涉及协议、并发、连接生命周期或公开接口的改动，都必须同步补充 `test/` 测试和 `docs/` 说明。

## 协议生成工具

`tools/protocolgen/generate.py` 要求 Python 3.10+，无第三方依赖。默认读取
`schema/gameapp.json` 并输出 `examples/gameapp/protocol/`；自定义定义必须明确 `--out`。
修改定义后运行生成器，再运行 `python tools/protocolgen/generate.py --check` 和
`python -m unittest discover -s test -p 'test_*.py' -v`。Python 测试也放在 `test/`。
不要手改生成代码、协议表或 lock；兼容性比较使用独立的已发布定义，通过 `--baseline` 指定。
不要把工具、Python 运行时或生成的业务定义引入 `gnet`/`gface`/`gcore`。完整约定见
`docs/protocol-toolchain.md`。

## Web 游戏示例与桥接

`examples/webgame/` 独立拥有 Vue/TS/Phaser 前端、访客认证、房间世界、移动与计分规则。
它不改变现有 TCP API。`wsbridge` 复用 DataPack，要求每条二进制 WS 消息恰好一包，
必须校验 Origin、大小、容量和超时；应用停止时显式 Close，不能只停 http.Server。
新增 Go 测试仍在 `test/`；浏览器测试在 `test/webgame/`，从 frontend 执行 `npm run test:e2e`。
前端依赖锁文件必须提交，node_modules/dist/测试截图不提交。运行指南见 `docs/webgame-guide.md`。

## Runtime Service Modules

The optional `session/` package owns account, token, player, and connection mappings. Applications supply player IDs to `Issue` and explicitly choose `RejectExisting` (default) or `ReplaceExisting` through `session.New(Options)`. Automatic IDs only remain in deprecated `NewManager` / `Login` compatibility code. The `limit/` package provides token buckets, `metrics/` provides atomic runtime snapshots, and optional `persist/` provides a player-shaped storage interface plus memory and JSON implementations. The transport does not require session or player storage. Keep application policy in `examples/` or downstream applications.

`MessageRateLimit` and `MessageRateBurst` are optional per-connection inbound limits. Both are disabled when the rate is zero. `gnet.Server.GetMetrics()` exposes connection, message, byte, and recovered-router-panic counters.
