package gnet

import (
	"Ginx/gface"
	"Ginx/limit"
	"Ginx/metrics"
	"context"
	"errors"
	"fmt"
	"io"
	"net"
	"sync"
	"sync/atomic"
	"time"
)

type Connection struct {
	//当前连接的socket TCP套接字
	Conn *net.TCPConn
	//当前连接的ID 也可以称作为SessionID，ID全局唯一
	ConnID uint32
	//当前连接的关闭状态
	isClosed atomic.Bool
	stopOnce sync.Once
	ioWait   sync.WaitGroup
	//消息管理MsgId和对应处理方法的消息管理模块
	MsgHandler gface.IMsgHandle
	//告知该链接已经退出/停止的channel
	ExitBuffChan chan bool
	//统一出站队列，容量由 MaxMsgChanLen 决定。
	msgChan chan []byte
	// 发送串行化使用可取消信号量，非阻塞发送不等待其他发送者。
	sendGate chan struct{}
	ctx      context.Context
	cancel   context.CancelFunc
	//连接属性集合
	property map[string]interface{}
	//保护连接属性的读写锁
	propertyLock sync.RWMutex
	metrics      *metrics.Metrics
	requestLimit *limit.TokenBucket
	config       *Config
	dataPack     *DataPack
}

// 创建连接的方法
func NewConntion(conn *net.TCPConn, connID uint32, msgHandler gface.IMsgHandle) *Connection {
	return newConnection(conn, connID, msgHandler, nil)
}

func newConnection(conn *net.TCPConn, connID uint32, msgHandler gface.IMsgHandle, config *Config) *Connection {
	settings := effectiveConfig(config)
	ctx, cancel := context.WithCancel(context.Background())
	c := &Connection{
		ctx: ctx, cancel: cancel, sendGate: make(chan struct{}, 1),
		config:       config,
		dataPack:     &DataPack{config: config},
		Conn:         conn,
		ConnID:       connID,
		MsgHandler:   msgHandler,
		ExitBuffChan: make(chan bool, 1),
		msgChan:      make(chan []byte, settings.MaxMsgChanLen),
		property:     make(map[string]interface{}),
	}
	if settings.MessageRateLimit > 0 {
		burst := settings.MessageRateBurst
		if burst <= 0 {
			burst = settings.MessageRateLimit
		}
		c.requestLimit, _ = limit.NewTokenBucket(settings.MessageRateLimit, burst)
	}

	return c
}

/*
写消息Goroutine， 用户将数据发送给客户端
*/
func (c *Connection) StartWriter() {
	defer c.Stop()

	fmt.Println("[Writer Goroutine is running]")
	defer fmt.Println(c.RemoteAddr().String(), "[conn Writer exit!]")

	for {
		select {
		case data := <-c.msgChan:
			if c.isClosed.Load() {
				return
			}
			if timeout := effectiveConfig(c.config).WriteTimeout; timeout > 0 {
				if err := c.Conn.SetWriteDeadline(time.Now().Add(time.Duration(timeout) * time.Millisecond)); err != nil {
					return
				}
			}
			//有数据要写给客户端
			if err := writeFull(c.Conn, data); err != nil {
				fmt.Println("Send Data error:, ", err, " Conn Writer exit")
				return
			}
		case <-c.ExitBuffChan:
			//conn已经关闭
			return
		}
	}
}

// 直接将Message数据发送数据给远程的TCP客户端
func (c *Connection) SendMsg(msgId uint32, data []byte) error {
	ctx := context.Background()
	if timeout := effectiveConfig(c.config).SendTimeout; timeout > 0 {
		var cancel context.CancelFunc
		ctx, cancel = context.WithTimeout(ctx, time.Duration(timeout)*time.Millisecond)
		defer cancel()
	}
	return c.SendMsgContext(ctx, msgId, data)
}

// SendMsgContext 等待入队，可取消；成功不代表消息已写入或被对端接收。
func (c *Connection) SendMsgContext(ctx context.Context, msgId uint32, data []byte) error {
	return c.send(ctx, msgId, data, false)
}

// SendBuffMsg 不等待发送者或队列，竞争时返回 ErrSendQueueFull。
func (c *Connection) SendBuffMsg(msgId uint32, data []byte) error {
	return c.send(context.Background(), msgId, data, true)
}

func (c *Connection) send(ctx context.Context, msgId uint32, data []byte, nonblocking bool) error {
	if c.isClosed.Load() {
		return ErrConnectionClosed
	}
	if err := ctx.Err(); err != nil {
		return err
	}
	if nonblocking {
		select {
		case c.sendGate <- struct{}{}:
		default:
			return ErrSendQueueFull
		}
	} else {
		select {
		case c.sendGate <- struct{}{}:
		case <-ctx.Done():
			return ctx.Err()
		case <-c.ctx.Done():
			return ErrConnectionClosed
		}
	}
	defer func() { <-c.sendGate }()
	if c.isClosed.Load() {
		return ErrConnectionClosed
	}
	if err := ctx.Err(); err != nil {
		return err
	}
	msg, err := c.dataPack.Pack(NewMsgPackage(msgId, data))
	if err != nil {
		return fmt.Errorf("pack message: %w", err)
	}
	if err := ctx.Err(); err != nil {
		return err
	}
	if nonblocking {
		select {
		case c.msgChan <- msg:
		default:
			return ErrSendQueueFull
		}
	} else {
		select {
		case c.msgChan <- msg:
		case <-ctx.Done():
			return ctx.Err()
		case <-c.ctx.Done():
			return ErrConnectionClosed
		}
	}
	c.metrics.AddBytesOut(uint64(len(msg)))
	return nil
}

// Context 在连接关闭时取消，路由可将它传入支持取消的操作。
func (c *Connection) Context() context.Context { return c.ctx }

// 设置连接属性
func (c *Connection) SetProperty(key string, value interface{}) {
	c.propertyLock.Lock()
	defer c.propertyLock.Unlock()
	c.property[key] = value
}

// 获取连接属性
func (c *Connection) GetProperty(key string) (interface{}, error) {
	c.propertyLock.RLock()
	defer c.propertyLock.RUnlock()
	if value, ok := c.property[key]; ok {
		return value, nil
	}
	return nil, errors.New("property not found")
}

// 移除连接属性
func (c *Connection) RemoveProperty(key string) {
	c.propertyLock.Lock()
	defer c.propertyLock.Unlock()
	delete(c.property, key)
}

func (c *Connection) RemoteAddr() net.Addr {
	return c.Conn.RemoteAddr()
}

/*
*
开启读取器方法
*/
func (c *Connection) StartReader() {
	//最后使用关闭
	defer c.Stop()
	//循环读
	for {
		settings := effectiveConfig(c.config)
		if settings.HeartbeatMax > 0 {
			deadline := time.Now().Add(time.Duration(settings.HeartbeatMax) * time.Second)
			if err := c.GetTCPConnection().SetReadDeadline(deadline); err != nil {
				fmt.Println("Set read deadline error: ", err)
				return
			}
		}

		// DataPack 会完整读取消息头和消息体，同时处理 TCP 半包和粘包。
		msg, err := c.dataPack.ReadMessage(c.GetTCPConnection())
		if err != nil {
			fmt.Println("Read Message error: ", err)
			return
		}
		if c.requestLimit != nil && !c.requestLimit.Allow() {
			fmt.Println("Connection message rate limit exceeded, ConnID = ", c.ConnID)
			return
		}
		if c.metrics != nil {
			c.metrics.IncMessages()
			c.metrics.AddBytesIn(uint64(msg.GetDataLen()))
		}

		// 得到当前客户端的请求数据。
		req := Request{
			conn: c,
			data: msg,
		}

		//从绑定好的消息和对应的处理方法中执行对应的Handle方法
		if err := c.MsgHandler.SendMsgToTaskQueue(&req); err != nil {
			fmt.Println("Send request to worker queue error: ", err)
			return
		}
	}
}

func (c *Connection) setMetrics(value *metrics.Metrics) {
	c.metrics = value
}

// writeFull 保证一条已经封包的消息完整写入 TCP 连接。
func writeFull(writer io.Writer, data []byte) error {
	for len(data) > 0 {
		written, err := writer.Write(data)
		if err != nil {
			return err
		}
		if written <= 0 {
			return io.ErrShortWrite
		}
		data = data[written:]
	}
	return nil
}

// 启动连接。
func (c *Connection) Start() {
	c.startIO()
	c.wait()
}

// 启动连接读写流程
func (c *Connection) startIO() {
	//1 开启用户从客户端读取数据流程的Goroutine
	c.startReader()
	//2 开启用于写回客户端数据流程的Goroutine
	c.startWriter()
}

func (c *Connection) startReader() {
	c.ioWait.Add(1)
	go func() {
		defer c.ioWait.Done()
		c.StartReader()
	}()
}

func (c *Connection) startWriter() {
	c.ioWait.Add(1)
	go func() {
		defer c.ioWait.Done()
		c.StartWriter()
	}()
}

// 等待连接退出
func (c *Connection) wait() {
	<-c.ExitBuffChan
	c.ioWait.Wait()
}

// 停止连接
func (c *Connection) Stop() {
	c.stopOnce.Do(func() {
		c.isClosed.Store(true)
		c.cancel()
		// 关闭socket链接
		c.Conn.Close()

		//写入关闭
		c.ExitBuffChan <- true
		//关闭所有chan管道
		close(c.ExitBuffChan)
	})
}

// 获取连接ID方法
func (c *Connection) GetConnId() uint32 {
	return c.ConnID
}

func (c *Connection) GetConnection() net.Conn {
	return c.Conn
}

// 获取原生socket连接方法
func (c *Connection) GetTCPConnection() *net.TCPConn {
	return c.Conn
}
