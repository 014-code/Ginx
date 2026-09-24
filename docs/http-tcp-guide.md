# Gin HTTP 与 TCP 联合入口

`examples/gameserver` 在同一个进程中运行 Gin HTTP 和 Ginx TCP，二者调用同一个
`gameapp.Service`，共享 `session.Manager`、`persist.PlayerStore` 和 `gcore.RoomManager`。
业务代码位于 `examples/gameapp/`，SQLite 适配器位于 `examples/sqlitestore/`；
网络核心 `gnet/` 不依赖 Gin、SQLite、账号或房间业务。示例不是框架的稳定公共 API。
原来的 tutorial 和 Unity 入口继续独立运行。新入口使用 JSON 消息体，不兼容旧 Unity 的 Protobuf 业务消息。

## Windows 一键启动（推荐）

安装 PowerShell 7，开启 Docker Desktop 并切换到 Linux 容器。双击仓库根目录的
`start-server.cmd`，或在 PowerShell 中执行：

```powershell
./start-server.ps1
```

不要求本机安装 Go。脚本定位自身所在仓库，从其他工作目录调用也可以。
首次缺少 `config/accounts.local.json` 时会提示输入账号（默认 `alice`）和确认密码，
创建 PlayerID 为 `1001` 的本地账号。密码要求 8..72 个 UTF-8 字节，隐藏输入，
只通过标准输入传给哈希程序，配置中只保存 bcrypt 哈希；不提供默认密码。
已有账号文件和玩家存档不会覆盖。账号文件和 `data/` 已被 Git 忽略，不能提交凭据。

脚本自动复用 `gin-go-mod` / `gin-go-build` 缓存，在 Docker 中构建后后台运行，
等待 HTTP `/healthz` 和 TCP 连接检查均通过才报告成功。第一次可能需要下载镜像和依赖。
容器默认名 `ginx-game`，默认 HTTP `127.0.0.1:8080`，TCP 端口取自 `config/ginx.json`
（当前为 `7777`）；两个端口只发布到本机。关闭脚本窗口不会停止服务。

```powershell
# 自定义端口；TcpPort 未提供时读取项目配置。
./start-server.ps1 -HttpPort 18080 -TcpPort 17777
# 自动化场景：必须预先准备账号，不会等待交互输入。
./start-server.ps1 -NonInteractive
# 查看日志 / 优雅停止（数据保留）。
docker logs --follow ginx-game
docker stop ginx-game
```

默认使用 SQLite `data/players.db`。支持 `-Storage sqlite|json`，还支持
`-ContainerName`、`-AccountsPath`、`-PlayersPath` 和 `-DockerImage`。
旧 JSON 文件不会自动迁移或删除；需要继续使用时运行 `./start-server.ps1 -Storage json`，
此模式仅支持旧登录、房间和查询，不支持成长写入。不要把 JSON 文件路径当作 SQLite 数据库。
相对文件路径都相对于仓库根目录。多个实例应使用不同容器名、端口和玩家存档文件。
启动时只把配置副本的 `Host` 改为 `0.0.0.0`，生成到 `data/runtime/<容器名>/config/ginx.json`，
不会修改原始 `config/ginx.json`。服务器二进制也位于对应的运行目录。

重复运行时，已就绪的本仓库同名容器会直接复用，不会重启或重新构建；修改源码、账号配置、
存档路径或启动参数后先 `docker stop ginx-game`，再运行脚本。已停止的本仓库同名容器会被替换，
挂载数据保留。其他项目的同名容器会报错，不会删除。端口冲突或本次新容器启动失败时，
脚本报告错误并清理本次容器，保留账号、配置和存档供重试。

## 手动启动

从仓库根目录执行。需要 Go 1.26.4 或更高版本；Docker 可用 `golang:1.26-bookworm`。

先生成 bcrypt 密码哈希。下面的密码仅是本地示例，请替换：

```powershell
'your-local-password' | go run ./examples/gameserver -hash-password
```

创建 `config/accounts.local.json`，把命令输出的哈希填入 `password_hash`：

```json
[
  {
    "account_id": "alice",
    "player_id": 1001,
    "password_hash": "替换为生成的 bcrypt 哈希"
  }
]
```

该文件和 `data/` 已被 Git 忽略。没有默认账号、明文密码或自动注册接口；
账号 ID 和 PlayerID 必须唯一。PlayerID 是持久身份，同一账号重新登录或服务重启不会重新分配。
修改账号配置后重启生效；不要把已有 PlayerID 分配给其他账号。

```powershell
go run ./examples/gameserver
```

- HTTP 默认 `127.0.0.1:8080`，可用 `-http` 修改。
- TCP 默认显式加载 `config/ginx.json`，默认地址 `127.0.0.1:7777`；可用 `-config` 指定配置路径。加载失败会终止启动，加载结果作为服务实例的独立配置。
- Token 默认绝对有效期 1 小时，可用 `-token-ttl 30m` 修改，心跳不延长有效期。
- 存储默认 `-storage sqlite`，数据库为 `data/players.db`，可用 `-players` 修改；账号文件可用 `-accounts` 修改。
- `-storage json` 使用旧 `data/players.json`；不自动导入到 SQLite，不支持成长写入。
- 初始房间为 `1`，容量 `100`；自定义入口可向 `gameapp.New` 传入多个房间配置。
- Ctrl+C / SIGTERM 触发 HTTP 请求收尾、会话与房间清理、TCP 停服。
- TCP 端口被占用时联合入口启动失败，并关闭已经创建的 HTTP 监听器。

Docker PowerShell 示例（挂载仓库，复用 Go 缓存）：

```powershell
'your-local-password' | docker run --rm -i -v "${PWD}:/workspace" -w /workspace -v gin-go-mod:/go/pkg/mod -v gin-go-build:/root/.cache/go-build golang:1.26-bookworm go run ./examples/gameserver -hash-password
```

跨容器访问 TCP 时，将 `config/ginx.json` 的 `Host` 设置为 `0.0.0.0`。先构建到挂载目录，
再直接运行二进制，确保 Docker 的停止信号由服务器接收：

```powershell
New-Item -ItemType Directory -Force data/bin | Out-Null
docker run --rm -v "${PWD}:/workspace" -w /workspace -v gin-go-mod:/go/pkg/mod -v gin-go-build:/root/.cache/go-build golang:1.26-bookworm go build -o data/bin/gameserver ./examples/gameserver
docker run --rm --name ginx-game -p 127.0.0.1:8080:8080 -p 127.0.0.1:7777:7777 -v "${PWD}:/workspace" -w /workspace golang:1.26-bookworm ./data/bin/gameserver -http 0.0.0.0:8080
```

## HTTP 协议

| 方法与路径 | 鉴权 | 行为 |
| --- | --- | --- |
| `GET /healthz` | 无 | 返回 `{"status":"ok"}`，表示 HTTP 入口可响应；不检测外部存储 |
| `POST /api/v1/login` | 账号密码 | 校验 bcrypt 哈希，加载或创建玩家，签发 Token |
| `GET /api/v1/me` | Bearer Token | 当前玩家存档、TCP 在线状态、房间 ID |
| `GET /api/v1/rooms` | Bearer Token | 房间 ID、容量、人数、状态 |
| `GET /api/v1/progress` | Bearer Token | 已解码的等级、经验、背包和奖励领取状态 |

登录请求：

```json
{"account_id":"alice","password":"your-local-password"}
```

登录成功：

```json
{"token":"随机不透明令牌","player_id":1001,"expires_at":"2026-09-24T13:00:00Z"}
```

其他请求在 Header 中传 `Authorization: Bearer <token>`。不接受 URL 查询参数中的 Token。
`/me` 返回 `{"player":{...},"online":false,"room_id":0}`；`Player.Data` 为字节数组，JSON 中使用 Base64。
第一次登录会创建最小玩家存档，后续登录读取现有存档，不覆盖业务数据。

错误响应为 `{"code":"..."}`：参数错误 `400 invalid_request`；密码错误、无效或过期 Token
为 `401 unauthorized`；登录限流为 `429 rate_limited`；存储错误为 `500 internal_error`。
登录默认全入口每秒 10 次、突发 20 次；请求体上限 4 KiB。不会返回内部存储错误或密码哈希。
所有响应设置 `Cache-Control: no-store`。

PowerShell 登录和查询：

```powershell
$loginBody = @{ account_id = 'alice'; password = 'your-local-password' } | ConvertTo-Json
$loginResult = Invoke-RestMethod http://127.0.0.1:8080/api/v1/login -Method Post -ContentType application/json -Body $loginBody
$headers = @{ Authorization = "Bearer $($loginResult.token)" }
Invoke-RestMethod http://127.0.0.1:8080/api/v1/rooms -Headers $headers
Invoke-RestMethod http://127.0.0.1:8080/api/v1/me -Headers $headers
```

## TCP 协议

继续使用现有小端序分帧：4 字节 JSON 字节长度 + 4 字节消息 ID + UTF-8 JSON。
客户端必须使用完整读取逻辑；长度继续受 `MaxPacketSize` 约束。

| 消息 ID | 请求消息体 | 行为 |
| --- | --- | --- |
| `1001` | `{"token":"HTTP 登录返回的 token"}` | 绑定当前连接，响应包含 `player_id` |
| `1002` | `{}` | 鉴权后的心跳 |
| `2001` | `{"room_id":1}` | 加入/切换房间，响应包含房间快照 |
| `2002` | `{}` | 离开当前房间 |
| `3001` | `{}` | 查询成长存档，需要 TCP 鉴权 |
| `3002` | `{}` | 入房后领取一次新手奖励，事务提交后响应 |
| `3003` | `{"item_id":"potion"}` | 入房后消耗一瓶药水，事务提交后响应 |

响应使用相同消息 ID：成功为 `{"code":"ok","data":{...}}`（无数据时省略 data），
失败为 `{"code":"unauthorized"}`、`invalid_request`、`room_not_found`、`room_unavailable`、
`not_in_room`、`reward_already_claimed`、`item_unavailable` 或 `internal_error`。
JSON 必须是单个对象，不接受多余字段、拼接对象或 `null`；客户端不能声明自己的 PlayerID。
未知消息 ID 保持框架原有行为：忽略并记录日志。

连接建立后先发送 `1001` 并等待成功，再发送 `2001`；后续定期发送 `1002`。
未完成鉴权不能加入房间。一个 Token 只能绑定一条 TCP 连接，重复绑定同一连接幂等，
不能拿同一个 Token 抢占其他连接或在已经鉴权的连接上切换账号。
重复入同一房间幂等；切换到满房失败时保留旧房间；离房和断线都更新 HTTP 可见人数。

HTTP 重复登录会撤销旧 Token，并关闭其旧 TCP 连接、移除旧房间成员。
断线时撤销绑定会话，需要重新 HTTP 登录；本版没有断线续接或自动恢复房间。
Token 到期后所有请求立即拒绝，后台每秒清理过期会话和连接；未鉴权连接最多保留约 31 秒，
`HeartbeatMax` 若更短则更早关闭。服务重启会清空 Token 和房间，玩家文件保留。

## 并发与生命周期

`gameapp.Service` 的锁串行化登录替换、连接绑定、房间迁移和断线清理。
HTTP 查询和 TCP 操作共用该服务，不在两个适配层各自维护玩家状态。
存储操作仍在业务锁内执行，示例优先可读性；大量玩家时需要细化锁粒度与异步调度。

## 成长与事务存档示例

完整操作说明和客户端见 [examples/README.md](../examples/README.md)。首次登录创建玩家，
空业务数据解释为等级 1、经验 0、空背包。入房后 `3002` 一次性发放 100 经验和 2 瓶药水，
等级变为 2；`3003` 每次扣 1 瓶药水，不模拟战斗回血。经验和数量由服务器固定，客户端不能指定。
`Progress` 的版本、等级、经验、背包和领取标记编码成 JSON，保存在 `Player.Data` 中。

SQLite 适配器复用 `persist.PlayerStore`，额外提供仅用于示例的 `Update` 事务方法，
框架接口不变。单条 `Save/Delete` 由 SQLite 隐式事务执行；成长更新使用显式 immediate 事务，
读取、规则校验、修改、保存、提交在同一事务内，失败或取消回滚。
设置 3 秒忙等待和 FULL 同步，使用默认回滚日志而非 WAL，数据库目录需要可写。
业务变更提交成功后才向客户端回复，不依赖断线、定时或停服时再补存。
重连/重启不会重置领奖标记；Token 和房间仍不持久化。

重复领取会返回 `reward_already_claimed`，不会重复发放。消耗操作没有请求 ID 去重，
如果响应丢失，应先查询背包，不要盲目重发；生产业务还需自己的幂等协议、迁移、备份和恢复策略。
示例是单服务进程，不提供分布式会话或多实例登录协调；SQLite 事务测试不代表整个应用支持多实例。
依赖目前共用根 `go.mod`，但只有示例适配器导入 SQLite，框架包不会编译或加载该驱动。

`session.Manager.Issue` 创建未绑定会话，`Bind` 完成绑定，`RemoveExpired` 回收过期会话。
`Session.Bound` 区分未绑定和合法的连接 ID 0。示例通过 `session.New` 显式选择 `ReplaceExisting`，
由业务提供 PlayerID。Unity 的临时编号也由示例自己分配。旧 `NewManager` / `Login` 仅作为已弃用兼容入口，
保留自动编号、立即绑定、不设置有效期的语义；新应用不要使用，详见 [框架与示例边界](framework-boundaries.md)。

`gnet.Server.StartWithError()` 返回监听失败，`Addr()` 返回实际地址（支持随机端口）。
现有 `Start()` / `Serve()` 调用保持可用。初始化时先启动 Writer，让 Hook 可以发欢迎消息；
Hook 完成后再启动 Reader，避免第一条请求早于业务初始化。Hook setter 和读取使用锁保护，
回调在锁外执行。关闭时等待读写协程退出，写失败也会关闭连接。
Hook 应迅速返回，不能等待客户端回复；入站处理在 Hook 返回后才启动。

部署到非本机环境时，应为 HTTP 配置 HTTPS，并为 TCP 配置可信网络或加密隧道，避免明文传输凭据。
本版不包含注册、密码重置、数据库账号服务、管理后台或 WebSocket。

## 验证

TCP JSON 消息定义已集中到 `schema/gameapp.json`。编号及请求结构来自
`examples/gameapp/protocol/`，既有消息 ID、JSON 字段、8 字节 TCP 头及 `gameapp.Msg*`
公开名称保持不变。消息表见生成的 [协议参考](../examples/gameapp/protocol/protocol.md)，
维护方式见 [Python 协议工具链](protocol-toolchain.md)。提交前运行
`python tools/protocolgen/generate.py --check` 检查定义与生成物一致。

`test/gameapp_test.go` 覆盖真实 HTTP 登录 → TCP 鉴权 → 入房 → HTTP 查询一致性、离房、断线、
重复登录、容量限制、过期、存储重启和联合停服；`session_token_test.go` 覆盖连接 ID 0、
并发绑定、Token 替换与过期；`server_hook_order_test.go` 验证欢迎消息与初始化时序。

Windows 启动脚本的实际 Docker 验证可运行 `pwsh -File test/start-server-smoke.ps1`，
覆盖首次账号创建与登录、真实示例客户端成长操作、含空格路径、不同工作目录、重复启动、停止后重建、存档保留、
端口冲突清理和非交互缺少账号。使用临时账号和随机端口，结束后清理测试容器及临时账号，
保留 `data/runtime/` 中被 Git 忽略的构建产物。

```powershell
go test -count=1 ./...
go test -race -count=1 ./...
go vet ./...
go build ./...
```
