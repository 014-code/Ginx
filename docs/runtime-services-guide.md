# Runtime Services Guide

## English

The framework now provides small building blocks for the next game-server layer:

1. `limit.TokenBucket` provides concurrency-safe rate limiting.
2. `metrics.Metrics` provides atomic counters and immutable snapshots.
3. `persist.PlayerStore` defines a storage boundary, with memory and JSON implementations.

These packages do not own player login, room state, or game rules. Compose them in an application service and keep the network layer responsible for transport and lifecycle only.

## 中文

框架提供了下一层游戏服务端能力的基础组件：

1. `limit.TokenBucket` 提供并发安全的令牌桶限流。
2. `metrics.Metrics` 提供原子计数器和不可变指标快照。
3. `persist.PlayerStore` 定义存储边界，并提供内存和 JSON 文件实现。

这些包不负责玩家登录、房间状态和具体游戏规则。应用层负责组合它们，网络层继续只处理传输和连接生命周期。
