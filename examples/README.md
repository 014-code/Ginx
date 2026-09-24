# 游戏业务示例

这里演示如何组合 Ginx，不定义通用框架的业务规则。等级、背包、新手奖励和 SQLite
都只存在于示例包，`gnet`、`gcore`、`gface`、`persist.PlayerStore` 不因示例增加业务 API。

| 包 | 用途 |
| --- | --- |
| `gameserver` | 可运行的 HTTP + TCP 服务入口 |
| `gameapp` | 共享会话、房间、HTTP/TCP 路由与成长规则 |
| `webgame` | Vue 3 + TS + Phaser 浏览器联机竞技场，WS→TCP 实测 Ginx；独立 Docker 一键启动 |
| `gameapp/protocol` | 从 `schema/gameapp.json` 生成的 TCP 消息编号与请求/响应结构 |
| `sqlitestore` | 纯 Go SQLite 驱动与事务存档适配器，不需要 C 编译器 |
| `gameclient` | 登录、鉴权、入房、领奖、消耗、查询的命令行客户端 |
| `gameprotocol` | 示例业务消息 ID，不扩展框架协议常量 |
| `unity` | Unity 专用编解码、玩家世界与出生点规则 |
| `unityserver` | Unity 路由、访客身份与连接 Hook；入口为 `unityserver/cmd` |

原 `gameapp/` 和 `main/gameserver` 已迁移至这里。Unity 推荐入口为 `go run ./examples/unityserver/cmd`，旧 `main/unityserver` 仅转发到同一实现；教程入口不变。框架、可选组件和兼容层的分工见 [框架与示例边界](../docs/framework-boundaries.md)。
所有命令从仓库根目录运行。具体账号配置、端口和错误码参阅 [HTTP/TCP 指南](../docs/http-tcp-guide.md)。

TCP JSON 消息定义统一在 `schema/gameapp.json`，修改后执行
`python tools/protocolgen/generate.py`，提交前执行同一命令加 `--check`。
旧 `gameapp.Msg*` 名称仍然可用，业务校验和响应 `Reply` 不变。生成器是独立通用工具，
详见 [协议工具链](../docs/protocol-toolchain.md)。

## 启动与体验

Windows 开启 Docker Desktop 后双击根目录 `start-server.cmd`，或执行：

```powershell
./start-server.ps1
```

首次引导创建账号；默认本机 HTTP 8080、TCP 7777，数据库 `data/players.db` 挂载在宿主机。
账号配置继续使用独立 bcrypt 哈希文件，不存入玩家数据库。旧 JSON 不自动导入，
可以通过 `-Storage json` 保留旧登录/房间演示，但成长修改要求 SQLite。
如果已有旧服务，先 `docker stop ginx-game`，再运行脚本，以启用新版入口。

本机有 Go 时，手动启动命令为 `go run ./examples/gameserver`。

客户端会产生真实的示例业务变更：领取一次奖励、尝试消耗一瓶药水。
下面在 PowerShell 隐藏输入密码，通过标准输入传入 Docker 客户端，不把密码写入命令历史：

```powershell
$credential = Get-Credential -UserName alice -Message 'Ginx example account'
$credential.GetNetworkCredential().Password | docker run --rm -i -v "${PWD}:/workspace" -w /workspace -v gin-go-mod:/go/pkg/mod -v gin-go-build:/root/.cache/go-build golang:1.26-bookworm go run ./examples/gameclient -account $credential.UserName -http http://host.docker.internal:8080 -tcp host.docker.internal:7777
Remove-Variable credential
```

`host.docker.internal` 用于 Docker Desktop 容器访问宿主机端口。使用自定义端口时同步修改参数。
本机 Go 可将密码通过标准输入传入 `go run ./examples/gameclient -account alice`。
客户端只打印玩家 ID 和业务响应，不打印 Token/密码。客户端退出即断开 TCP，会话随之失效。

第一次运行结束应看到：

```json
{"version":1,"level":2,"experience":100,"inventory":{"potion":1},"starter_claimed":true}
```

重复运行时领取返回 `reward_already_claimed`，继续消耗剩余药水；没有药水时返回
`item_unavailable`，数量不会变成负数。停止、重启服务器再登录，等级、库存和领取状态保留。

## 保存时机

首次登录创建最小玩家记录。业务存档 `Progress` 在空数据时使用初始值；第一次修改写入
版本 1 的 JSON。奖励标记、经验、等级和物品数量一起提交，不能只保存其中一部分。

领取奖励和消耗物品均执行 `Update`：开启 immediate 事务，读取存档，校验与修改，
写入数据和时间戳，提交，最后回复成功。任何错误回滚，无需等到玩家断线或服务器停服。
适配器提供 `Close`，正常停服先关闭 HTTP/TCP，再关闭数据库。

SQLite 使用一张 `example_players` 表：文本 PlayerID 主键（保留完整 uint64）、唯一 AccountID、
业务数据 BLOB、更新时间。没有 ORM、自动版本迁移、装备系统或跨服逻辑。
读取损坏/未知版本成长数据会报错，不会静默重置。

本示例只用于理解组合与事务边界。它仍用业务全局锁，账号配置也未做数据库管理；
消耗请求没有通用幂等键，响应丢失时应查询存档而非自动重试。真实游戏需要按业务扩展这些策略。

## 验证

```powershell
docker run --rm -v "${PWD}:/workspace" -w /workspace -v gin-go-mod:/go/pkg/mod -v gin-go-build:/root/.cache/go-build golang:1.26-bookworm go test -count=1 ./test -run 'TestExample|TestGame'
pwsh -File test/start-server-smoke.ps1
```

测试覆盖：真实 HTTP/TCP 成长流程、重复领奖、背包耗尽、事务回滚/取消、并发读改写、
重开数据库恢复、数据库错误、启动脚本的重建与数据保留。新增 Go 测试仍统一放在根 `test/`。
