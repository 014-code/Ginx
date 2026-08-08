# Unity 客户端兼容说明

## 1. 客户端项目

Unity 客户端目录：

```text
C:\Users\14\Desktop\mmo_demo_unity3d
```

项目使用 Unity `5.4.1f1`，启动场景为 `Assets/Scene/Login.unity`，登录成功后进入 `Assets/Scene/Main.unity`。

## 2. 启动服务端

在 Ginx 根目录执行：

```powershell
go run ./main/unityserver
```

默认监听：

```text
127.0.0.1:7777
```

Unity 登录界面填写：

```text
IP:   127.0.0.1
Port: 7777
```

如果客户端运行在另一台机器上，需要把服务端配置中的 `Host` 改为可被客户端访问的地址，并在 Unity 中填写该地址。

## 3. 消息协议

Unity 客户端使用 Ginx 默认 `DataPack` 分帧：

```text
DataLen uint32，小端序
MsgID   uint32，小端序
Data    Protobuf 数据
```

当前兼容的消息 ID：

| ID | 方向 | 数据 | 作用 |
| --- | --- | --- | --- |
| 1 | 服务端 -> 客户端 | `SyncPid` | 分配玩家 ID |
| 2 | 客户端 -> 服务端 | `Talk` | 发送聊天 |
| 3 | 客户端 -> 服务端 | `Position` | 上报位置和朝向 |
| 200 | 服务端 -> 客户端 | `BroadCast` | 聊天、玩家加入、移动广播 |
| 201 | 服务端 -> 客户端 | `SyncPid` | 玩家下线通知 |
| 202 | 服务端 -> 客户端 | `SyncPlayers` | 同步当前已存在玩家 |

Protobuf 消息的 Go 定义和编解码实现位于：

```text
gcore/unity_protocol.go
```

在线玩家状态位于：

```text
gcore/unity_world.go
```

## 4. 当前服务端行为

客户端建立连接后，服务端会依次发送：

1. `SyncPid`，分配与 Ginx `ConnID` 相同的玩家 ID。
2. `SyncPlayers`，同步当前已经在线的其他玩家。
3. `BroadCast(TP=2)`，广播新玩家加入。

客户端发送消息 ID `2` 时，服务端解析 `Talk`，并使用 `BroadCast(TP=1)` 广播聊天内容。

客户端发送消息 ID `3` 时，服务端解析 `Position`，更新玩家状态，并使用 `BroadCast(TP=3)` 广播移动信息。

连接断开时，服务端使用消息 ID `201` 广播离线玩家的 PID。

当前版本使用全量广播，尚未按 AOI 视野过滤玩家。后续可以把 `UnityWorld.Broadcast()` 替换为 AOI 范围广播。

## 5. 注意事项

Unity 客户端是旧项目，建议先使用 Unity `5.4.1f1` 打开。如果新版本 Unity 自动升级项目，建议先复制一份客户端目录。

服务端示例 `main/server` 仍然保留原有 Ping/Hello 教程，不负责 Unity MMO 协议；联调时必须启动 `main/unityserver`。
