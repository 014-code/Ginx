# Python 协议工具链

`tools/protocolgen/generate.py` 是框架的通用构建工具，不是客户端 SDK，也不是 Python
服务端运行时。它读取应用提供的协议定义，生成代码、协议表及可比较的定义快照。
TCP 连接、Worker、路由、封包与业务执行仍由 Go 负责；Go 编译和服务启动不需要 Python。

## 快速使用

需要 Python 3.10+，仅使用标准库，无需 `pip install`。从仓库根目录运行：

```powershell
# 校验定义，不生成文件
python tools/protocolgen/generate.py --validate
# 生成，已有且内容一致的文件不重写
python tools/protocolgen/generate.py
# CI/提交前只读检查，缺失或过期返回非零
python tools/protocolgen/generate.py --check
# 工具自身测试
python -m unittest discover -s test -p 'test_*.py' -v
```

没有本地 Python 时可以使用 Docker，不需要另起常驻服务：

```powershell
docker run --rm -v "${PWD}:/workspace" -w /workspace python:3.12-slim python tools/protocolgen/generate.py --check
docker run --rm -e PYTHONDONTWRITEBYTECODE=1 -v "${PWD}:/workspace" -w /workspace python:3.12-slim python -m unittest discover -s test -p 'test_*.py' -v
```

默认输入为 `schema/gameapp.json`，输出为 `examples/gameapp/protocol/`，这两个默认位置
相对于工具所在仓库确定，不依赖工作目录。它们只是现有 HTTP/TCP 示例的接入配置，
不代表框架要求应用实现登录、房间或成长功能。其他项目明确指定自己的输入、输出：

```powershell
python tools/protocolgen/generate.py --schema path/to/protocol.json --out path/to/generated
python tools/protocolgen/generate.py --schema path/to/protocol.json --out path/to/generated --check
```

自定义路径相对于当前工作目录解析。自定义 `--schema` 必须配合 `--out`，除非只运行
`--validate`，避免误写仓库默认生成目录。

## 协议定义格式 v1

第一版采用严格 JSON，不支持 YAML、模板执行或动态导入。以下是一份完整定义：

```json
{
  "schema_version": 1,
  "protocol_version": 1,
  "go_package": "protocol",
  "csharp_namespace": "MyServer.Protocol",
  "encoding": "json",
  "messages": [
    {
      "name": "Echo",
      "id": 0,
      "description": "通用回显消息",
      "request": [{"name": "text", "type": "string"}],
      "response": [{"name": "text", "type": "string"}]
    }
  ]
}
```

- `schema_version` 是工具定义格式版本，目前必须为整数 `1`。
- `protocol_version` 是应用协议版本，范围为 `1..4294967295`。它只生成元数据常量，
  **不会插入 TCP 消息头，也不会自动完成版本协商**。
- `messages` 包含 1..4096 个消息；同一份定义内 ID 和名称必须唯一。
- 消息 ID 为 `0..4294967295`，对应 `DataPack` 的 uint32 消息 ID，不是可选
  `gcore.GameMessage` 信封中的 uint16 ID。
- 一个条目描述使用相同消息 ID 的请求和响应。`request`、`response` 均为必填字段数组；
  空数组生成空结构体，其 JSON 表示为 `{}`。本版不表达单向推送或不同 ID 的响应配对。
- 消息名使用以大写字母开头的 ASCII 字母数字，例如 `Echo`、`QueryStatus`。
- 字段名使用小写 snake_case，例如 `room_id`，生成 Go 字段 `RoomID`。转换后重名也会拒绝。
- 每侧最多 128 个字段。`description` 可省略，只用于生成文档，不进入代码逻辑。
- 顶层、消息及字段中的未知键、重复 JSON 键、非法名称、非整数 ID、NaN/Infinity、
  不支持的字段类型都会导致失败。输入文件大小上限为 2 MiB。

### 字段类型

| 定义类型 | Go 类型 | JSON 说明 |
| --- | --- | --- |
| `string` | `string` | 字符串 |
| `bool` | `bool` | 布尔值 |
| `int32` / `uint32` | 对应 Go 整数类型 | JSON 数字 |
| `int64` / `uint64` | 对应 Go 整数类型 | JSON 数字，接收方需保留整数精度 |
| `float64` | `float64` | 有限浮点数；业务自行定义有效范围 |
| `bytes` | `[]byte` | `encoding/json` 使用 Base64 字符串；nil 为 null |
| `json` | `json.RawMessage` | 保留原始 JSON，工具不校验其内部结构 |
| `[]T` | `[]T` 的映射类型 | 一维列表；`[]bytes` 对应 `[][]byte` |

不支持自定义嵌套类型、枚举、map、默认值、验证表达式或多维列表。复杂结果可临时用
`json` 表达，但不能据此宣称其业务结构已被工具校验。

`optional: true` 生成指针字段和 `omitempty` 标签，nil 在编码时省略。未声明 optional
的字段生成普通字段，不带 `omitempty`。**这只是类型与编码约定，不是运行时必填校验**：
缺失字段、业务范围、未知字段、null 的策略仍由应用解码和校验。缺失和显式 null 通常
不能仅靠这些结构体区分；列表 nil 和空列表也有不同 JSON 表示。

基础编码行为沿用 Go `encoding/json`，见官方包说明：
`https://pkg.go.dev/encoding/json`。Python 解析器通过 `object_pairs_hook` 显式拒绝
重复 JSON 键，而不是接受默认的后值覆盖行为：`https://docs.python.org/3/library/json.html`。

## 生成物与职责

| 文件 | 内容 |
| --- | --- |
| `protocol.gen.go` | uint32 消息 ID、版本常量、请求/响应结构体及 JSON 标签 |
| `protocol_gen.py` | 消息 ID 和版本常量，不是 Python 客户端或编解码器 |
| `ProtocolIds.g.cs` | C# 消息 ID 和版本常量，不依赖 Unity，不是客户端 SDK |
| `protocol.md` | 消息编号、字段、类型及说明 |
| `protocol.lock.json` | 规范化协议定义快照，供以后兼容性比较，不是依赖锁 |

生成顺序固定，不包含时间戳或本机绝对路径。代码头包含定义的 SHA-256，用于辨认来源；
真正的一致性检查会重新渲染并比较完整内容，不只检查哈希。输入字段/消息重排不会改变输出。
Go 输出应当与 `gofmt` 一致；Python/C# 目前只生成常量，不生成它们的结构体或序列化器。

输出目录中其他文件保持不动，不递归删除。只覆盖带有本工具生成标记的目标；遇到同名
手写文件、符号链接或输入/输出冲突会拒绝。写入采用单文件临时文件替换；多文件不是事务，
中断后重新生成并执行 `--check`。请使用可信本地目录，不将这个 CLI 当成多用户文件服务。
`--check`、`--validate` 不写文件；检查忽略 Windows CRLF 与 LF 的差异。

退出码：`0` 成功；`1` 表示 `--check` 发现生成物缺失/过期；`2` 表示输入、兼容性、
参数或文件操作错误。不要手改生成物，应该修改源定义后重新生成，并将生成物一起提交。

## 协议兼容性检查

`--check` 检查“当前定义与生成物一致”，不自动知道过去发布过什么。兼容性检查需显式传入
**之前发布并保留的定义或 lock 文件**：

```powershell
python tools/protocolgen/generate.py --schema path/to/protocol.json --baseline path/to/previous.lock.json --validate
python tools/protocolgen/generate.py --schema path/to/protocol.json --out path/to/generated --baseline path/to/previous.lock.json --check
```

比较策略是保守的：

- 说明文字可改；协议版本不能降低。
- 新增消息必须提升 `protocol_version`。
- 旧消息不允许删除、改名、改 ID；字段名称、类型和 optional 属性不允许改变，也不允许增删。
- Go 包名、C# 命名空间和编码方式不得改变。
- 即使提升版本，也不自动豁免破坏性变更。需要应用明确做新协议/双版本迁移，再建立新的基线。

连“增加可选字段”也拒绝，是因为既有示例使用 `DisallowUnknownFields`；不能假定老接收端
会忽略新字段。检查不覆盖语义变化、`json` 内部结构、运行时验证和自动版本协商。
本次待生成的 `protocol.lock.json` 不能同时作为写入时的 baseline；请使用独立、未被覆盖的
发布快照，避免新旧定义实际比较的是同一个文件。

## 当前仓库的接入范围

`schema/gameapp.json` 仅记录 HTTP/TCP 示例已有的 7 个 TCP 消息。`examples/gameapp/tcp.go`
使用生成的请求结构和编号，保留既有 `gameapp.Msg*` 公开别名和 `Reply` API。响应生成类型
通过测试验证能读取现有 `Reply`；动态 `data` 仍由业务决定，不更改存档或 HTTP 返回结构。
空对象请求仍共享原有严格解码流程，没有引入自动路由或校验器。

TCP 头保持 `4 bytes DataLen LE + 4 bytes MsgID LE`，消息体仍为 JSON。教程的文本协议、
Unity Protobuf 协议及 `examples/gameprotocol` 消息信封是独立协议，不纳入这一份定义，也不迁移。
`gnet`、`gface`、`gcore` 不导入生成包或执行 Python。工具是通用的，仓库内业务定义只是实际使用者。

## 验证

```powershell
python tools/protocolgen/generate.py --check
python -m unittest discover -s test -p 'test_*.py' -v
go test -count=1 ./...
go test -race -count=1 ./...
go vet ./...
go build ./...
git diff --check
```

Python 测试位于 `test/test_protocolgen.py`，覆盖定义错误、跨语言常量、生成幂等、兼容性、
文件安全和只读检查。Go 测试统一位于 `test/`，新增 `protocolgen_test.go` 覆盖既有消息 ID、
JSON、TCP 分帧、响应兼容及格式，已有 HTTP/TCP 真实联调继续覆盖应用行为。
