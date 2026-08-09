# Ginx 带缓冲发包指南

## 中文

### 为什么需要带缓冲发包

`SendMsg()` 和 `SendBuffMsg()` 现在进入同一个连接出站队列，由单独的写协程顺序写出。`SendMsg()` 在队列满时等待，`SendBuffMsg()` 在队列满时立即返回错误。

`SendBuffMsg()` 使用每条连接独立的缓冲队列，适合游戏服务端中的状态同步、公告、广播和连续回包等场景。

```go
func (r *PingRouter) Handle(request gface.IRequest) {
    err := request.GetConnection().SendBuffMsg(0, []byte("pong"))
    if err != nil {
        fmt.Println("send buffered message error:", err)
    }
}
```

### 配置缓冲区

在 `config/ginx.json` 中配置每条连接的缓冲队列容量：

```json
{
  "MaxMsgChanLen": 1024
}
```

容量只属于单条连接，不是所有连接共享的全局队列。连接数较多时，应根据消息大小、推送频率和内存预算设置容量。

### 当前实现的行为

| 情况 | 行为 |
| --- | --- |
| 队列有空间 | 消息入队并立即返回 `nil`。 |
| 队列已满 | 立即返回 `Connection outbound msg queue is full`，不会永久阻塞。 |
| 连接已经停止 | 返回 `Connection closed when send buff msg`。 |
| 写协程发送失败 | 记录错误并结束写协程，连接后续由连接生命周期清理。 |
| 停服时仍有未发送消息 | 连接关闭，尚未写出的缓冲消息会被丢弃。 |

教程中直接执行 `c.msgBuffChan <- msg` 的不足是队列满时会阻塞；直接关闭 `msgBuffChan` 也会和并发发送产生 `send on closed channel`。当前实现只关闭连接退出信号，不关闭发送队列，并使用统一出站队列、入队锁和非阻塞入队处理这些问题。

### 使用建议

- 低优先级、允许丢弃的实时状态消息使用 `SendBuffMsg()`。
- 登录结果、扣款结果、关键状态确认等重要消息使用 `SendMsg()`，并在业务层处理错误。
- 同一条业务链路不要混用两种发送方法来依赖严格顺序；两种方法使用不同的内部通道，混用时不保证跨通道顺序。
- 收到队列满错误时，可以丢弃低优先级消息、降低推送频率，或交给业务层的可靠消息队列处理。
- 不要把 `MaxMsgChanLen` 设置得过大。每条连接都会创建自己的队列，连接数乘以队列容量会直接影响内存占用。

### 相关测试

相关测试位于 `test/connection_test.go`，包括：

- 带缓冲消息可以通过真实 TCP 写协程发送。
- 缓冲队列满时快速返回错误。
- 连接停止后再次发送返回关闭错误。

## English

### Buffered outbound messages

`SendMsg()` and `SendBuffMsg()` use the same per-connection outbound queue and one writer goroutine. `SendMsg()` waits when the queue is full, while `SendBuffMsg()` returns immediately with an error.

```go
if err := request.GetConnection().SendBuffMsg(0, []byte("pong")); err != nil {
    fmt.Println("send buffered message error:", err)
}
```

The queue capacity is configured with `MaxMsgChanLen`. A full queue returns an error instead of blocking forever. Sending after connection shutdown returns a closed-connection error. Pending buffered messages are discarded when the connection is stopped.

`SendMsg()` and `SendBuffMsg()` share one queue, so messages accepted by the same connection are written in enqueue order. Calls made concurrently by different goroutines still need business-level sequencing when their order matters.
