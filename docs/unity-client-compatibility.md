# Unity 客户端兼容说明

本文说明如何使用 Ginx 启动与旧版 Unity MMO 示例项目兼容的服务端，并介绍双方使用的数据包格式、Protobuf 消息、玩家生命周期和当前限制。

## 1. 项目位置和版本

Unity 客户端目录：

```text
C:\Users\14\Desktop\mmo_demo_unity3d
```

客户端工程版本：

```text
Unity 5.4.1f1
```

Build Settings 中的场景顺序为：

```text
0  Assets/Scene/Login.unity
1  Assets/Scene/Main.unity
```

登录按钮调用 `Login.OnLogin()`，保存登录界面中的 IP 和端口，然后通过 `SceneManager.LoadScene(1)` 进入主场景。主场景中的 `PlayerController.Start()` 会调用 `NetMgr.SendConnect()` 建立 TCP 连接。

## 2. 本机启动步骤

### 2.1 启动 Ginx 服务端

在 Ginx 项目根目录执行：

```powershell
go run ./main/unityserver
```

服务端读取 `config/ginx.json`，默认监听：

```text
127.0.0.1:7777
```

不要使用 `go run ./main/server` 与 Unity 联调。`main/server` 是消息 ID `0` 和 `1` 的 Ping/Hello 基础示例，不包含 MMO Protobuf 路由。

### 2.2 启动 Unity 客户端

1. 使用 Unity Hub 添加目录 `C:\Users\14\Desktop\mmo_demo_unity3d`。
2. 使用 Unity `5.4.1f1` 或能够兼容该工程的旧版本打开项目。
3. 打开 `Assets/Scene/Login.unity`。
4. 点击 Unity 编辑器顶部的 Play。
5. 在登录界面填写 `127.0.0.1` 和 `7777`。
6. 点击登录按钮进入主场景。
7. 查看 Unity Console 和 Ginx 服务端终端日志。

客户端原始默认地址是 `192.168.2.225:8999`，与 Ginx 默认配置不同。本机联调时需要在登录界面改成 `127.0.0.1:7777`。

### 2.3 跨机器启动

客户端和服务端不在同一台机器时：

1. 将 `config/ginx.json` 中的 `Host` 改为服务端实际监听地址，通常可以使用 `0.0.0.0`。
2. 在防火墙中允许 TCP 端口 `7777`。
3. 在 Unity 登录界面填写服务端局域网 IP，不要填写客户端自己的 `127.0.0.1`。

## 3. TCP 数据包格式

Unity 客户端和 Ginx 都使用小端序的 8 字节消息头：

```text
+-------------------+-------------------+----------------------+
| DataLen uint32    | MsgID uint32      | Protobuf Data        |
| 4 bytes LE        | 4 bytes LE        | DataLen bytes        |
+-------------------+-------------------+----------------------+
```

- `DataLen` 表示后续 Protobuf 消息体长度，不包含 8 字节消息头。
- `MsgID` 用于选择 Ginx 路由和 Unity 客户端处理逻辑。
- `Data` 使用客户端 `Assets/Scripts/Data/Msg.cs` 中定义的 Protobuf 格式。
- Ginx 的 TCP 分帧由 `gnet.DataPack` 和 `Connection.StartReader()` 处理。

Go 端兼容协议位于：

```text
gcore/unity_protocol.go
```

## 4. 消息 ID

| ID | 方向 | Protobuf 数据 | 作用 |
| --- | --- | --- | --- |
| `1` | 服务端 -> 客户端 | `SyncPid` | 给新连接分配玩家 PID |
| `2` | 客户端 -> 服务端 | `Talk` | 客户端发送聊天内容 |
| `3` | 客户端 -> 服务端 | `Position` | 客户端上报位置和朝向 |
| `200` | 服务端 -> 客户端 | `BroadCast` | 广播聊天、玩家加入和玩家移动 |
| `201` | 服务端 -> 客户端 | `SyncPid` | 通知其他客户端玩家已经离线 |
| `202` | 服务端 -> 客户端 | `SyncPlayers` | 给新连接同步已经在线的玩家 |

## 5. Protobuf 数据结构

服务端实现了客户端需要的以下消息：

```text
SyncPid
  Pid int32

Position
  X float
  Y float
  Z float
  V float

Player
  Pid int32
  P   Position

SyncPlayers
  Ps repeated Player

MovePackege
  P          Position
  ActionData int32

Talk
  Content string

BroadCast
  Pid        int32
  Tp         int32
  Content    string
  P          Position
  ActionData int32
```

`Position.V` 表示 Unity 玩家 Y 轴朝向。客户端当前发送消息 ID `3` 时直接发送 `Position`，没有发送 `MovePackege`；服务端仍然保留了 `MovePackege` 编解码，方便后续扩展动作同步。

## 6. BroadCast.TP 类型

Unity 客户端通过 `BroadCast.Tp` 区分消息用途：

| TP | 作用 | 使用字段 |
| --- | --- | --- |
| `1` | 聊天广播 | `Pid`、`Content` |
| `2` | 玩家加入或玩家出生 | `Pid`、`P` |
| `3` | 玩家位置和朝向更新 | `Pid`、`P`、`ActionData` |
| `4` | 预留的坐标移动广播 | `Pid`、`P` |

当前服务端发送 `TP=1`、`TP=2` 和 `TP=3`。客户端已经保留 `TP=4` 的处理逻辑，但服务端暂时没有使用。

## 7. 玩家连接时序

新客户端完成 TCP 连接后，服务端执行以下流程：

```text
Unity client
    |
    | TCP connect
    v
Ginx OnConnStart
    |
    |-- MsgID 1: SyncPid
    |      分配 PID，当前 PID 与 Ginx ConnID 相同
    |
    |-- MsgID 202: SyncPlayers
    |      同步连接前已经在线的玩家
    |
    `-- MsgID 200: BroadCast(TP=2)
           向全部在线客户端广播新玩家加入
```

Unity 客户端先把 `SyncPid.Pid` 加入本地玩家 ID 集合，再收到自己的 `BroadCast(TP=2)`。客户端会把这条消息识别为自己的初始位置更新，从而完成主玩家初始化。

## 8. 聊天、移动和下线流程

### 聊天

客户端发送：

```text
MsgID 2 + Talk
```

服务端解析聊天内容，然后向全部在线连接发送：

```text
MsgID 200 + BroadCast(TP=1)
```

### 移动

客户端发送：

```text
MsgID 3 + Position
```

服务端更新 `UnityWorld` 中的玩家坐标，然后向全部在线连接发送：

```text
MsgID 200 + BroadCast(TP=3)
```

广播包含发送者自身。发送者需要收到服务端确认后的移动数据，客户端才能完成初始位置和后续同步逻辑。

### 下线

连接关闭时，服务端先从 `UnityWorld` 删除玩家，然后向剩余连接发送：

```text
MsgID 201 + SyncPid
```

## 9. 服务端代码位置

```text
gcore/unity_protocol.go       Unity Protobuf 编解码
gcore/unity_world.go          在线玩家状态和广播
main/unityserver/Server.go    Unity MMO 路由及连接 Hook
test/unity_protocol_test.go   协议单元测试
test/unity_world_test.go      玩家世界单元测试
```

## 10. 验证命令

```powershell
go test ./...
go vet ./...
go build ./...
```

协议测试覆盖坐标、广播、玩家列表、移动包和错误数据；世界状态测试覆盖玩家加入、状态更新、离线和排除指定连接的广播。

## 11. 当前限制

### 全量广播

当前 `UnityWorld.Broadcast()` 会向全部在线玩家广播聊天和移动消息，还没有根据 AOI 过滤视野。玩家数量增加后，应使用 `gcore.AOIManager` 查询附近玩家，再只向进入视野范围的连接发送消息。

### 状态只保存在内存

玩家 PID、位置和在线关系只保存在当前服务进程中，服务重启后会清空。当前实现不包含账号登录、数据库存储、断线重连和跨服状态同步。

### 旧 Unity 客户端的 TCP 读取方式

客户端 `SocketClient.OnRead()` 直接处理固定大小的 `byteBuffer`，没有使用 `EndRead()` 返回的实际读取长度，也没有完整保存半包。TCP 不保证一次读取正好得到一个完整消息，因此网络波动时可能出现半包解析失败。

服务端的数据包格式已经与客户端一致，但这个客户端读取问题无法由服务端彻底规避。正式使用前应在 Unity 客户端增加接收缓存，按照 `DataLen + 8` 循环拆包，并保留不足一帧的数据等待下一次读取。

## 12. 常见问题

### 客户端点击登录后没有进入可移动状态

确认启动的是：

```powershell
go run ./main/unityserver
```

同时检查 Unity Console 是否成功收到消息 ID `1`、`202` 和 `200`。

### 服务端没有收到连接

检查 Unity 登录界面地址是否为 `127.0.0.1:7777`，并确认 `config/ginx.json` 中的 `Host` 和 `TcpPort` 与客户端一致。

### 收到消息但 Protobuf 解析失败

确认消息 ID 和 Protobuf 类型匹配。例如消息 ID `1` 的数据必须是 `SyncPid`，不能发送普通字符串。

### 第二个客户端看不到第一个客户端

第二个客户端连接时应收到消息 ID `202`。检查服务端日志是否执行了玩家加入流程，并检查 Unity 客户端是否成功解析 `SyncPlayers.Ps`。
