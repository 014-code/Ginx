package gnet

import (
	"Ginx/gface"
	"Ginx/limit"
	"Ginx/metrics"
	"Ginx/utils"
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
	//消息管理MsgId和对应处理方法的消息管理模块
	MsgHandler gface.IMsgHandle
	//告知该链接已经退出/停止的channel
	ExitBuffChan chan bool
	//无缓冲管道，用于读、写两个goroutine之间的消息通信
	msgChan chan []byte
	//有缓冲管道，用于临时缓存需要发送给客户端的消息
	//保护有缓冲消息入队和连接关闭之间的并发关系
	buffSendLock sync.Mutex
	//连接属性集合
	property map[string]interface{}
	//保护连接属性的读写锁
	propertyLock sync.RWMutex
	metrics      *metrics.Metrics
	requestLimit *limit.TokenBucket
}

// 创建连接的方法
func NewConntion(conn *net.TCPConn, connID uint32, msgHandler gface.IMsgHandle) *Connection {
	c := &Connection{
		Conn:         conn,
		ConnID:       connID,
		MsgHandler:   msgHandler,
		ExitBuffChan: make(chan bool, 1),
		msgChan:      make(chan []byte, utils.GlobalObject.MaxMsgChanLen),
		property:     make(map[string]interface{}),
	}
	if utils.GlobalObject.MessageRateLimit > 0 {
		burst := utils.GlobalObject.MessageRateBurst
		if burst <= 0 {
			burst = utils.GlobalObject.MessageRateLimit
		}
		c.requestLimit, _ = limit.NewTokenBucket(utils.GlobalObject.MessageRateLimit, burst)
	}

	return c
}

/*
写消息Goroutine， 用户将数据发送给客户端
*/
func (c *Connection) StartWriter() {

	fmt.Println("[Writer Goroutine is running]")
	defer fmt.Println(c.RemoteAddr().String(), "[conn Writer exit!]")

	for {
		select {
		case data := <-c.msgChan:
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
	c.buffSendLock.Lock()
	defer c.buffSendLock.Unlock()

	if c.isClosed.Load() {
		return errors.New("Connection closed when send msg")
	}
	//将data封包，并且发送
	dp := NewDataPack()
	msg, err := dp.Pack(NewMsgPackage(msgId, data))
	if err != nil {
		fmt.Println("Pack error msg id = ", msgId)
		return errors.New("Pack error msg ")
	}

	//写回客户端
	select {
	case c.msgChan <- msg:
		if c.metrics != nil {
			c.metrics.AddBytesOut(uint64(len(msg)))
		}
		return nil
	case <-c.ExitBuffChan:
		return errors.New("Connection closed when send msg")
	}
}

// 直接将Message数据发送数据给远程的TCP客户端(有缓冲)
func (c *Connection) SendBuffMsg(msgId uint32, data []byte) error {
	c.buffSendLock.Lock()
	defer c.buffSendLock.Unlock()

	if c.isClosed.Load() {
		return errors.New("Connection closed when send buff msg")
	}

	//将data封包，并且发送
	dp := NewDataPack()
	msg, err := dp.Pack(NewMsgPackage(msgId, data))
	if err != nil {
		fmt.Println("Pack error msg id = ", msgId)
		return errors.New("Pack error msg ")
	}

	//写回客户端，缓冲队列满时直接返回，避免业务协程永久阻塞
	select {
	case c.msgChan <- msg:
		if c.metrics != nil {
			c.metrics.AddBytesOut(uint64(len(msg)))
		}
		return nil
	default:
		return errors.New("Connection outbound msg queue is full")
	}
}

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
		if utils.GlobalObject.HeartbeatMax > 0 {
			deadline := time.Now().Add(time.Duration(utils.GlobalObject.HeartbeatMax) * time.Second)
			if err := c.GetTCPConnection().SetReadDeadline(deadline); err != nil {
				fmt.Println("Set read deadline error: ", err)
				return
			}
		}

		// DataPack 会完整读取消息头和消息体，同时处理 TCP 半包和粘包。
		msg, err := NewDataPack().ReadMessage(c.GetTCPConnection())
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
		if utils.GlobalObject.WorkerPoolSize > 0 {
			if err := c.MsgHandler.SendMsgToTaskQueue(&req); err != nil {
				fmt.Println("Send request to worker queue error: ", err)
				return
			}
		} else {
			go c.MsgHandler.DoMsgHandler(&req)
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
	go c.StartReader()
	//2 开启用于写回客户端数据流程的Goroutine
	go c.StartWriter()
}

// 等待连接退出
func (c *Connection) wait() {
	for {
		select {
		case <-c.ExitBuffChan:
			//得到退出消息就走
			return
		}
	}
}

// 停止连接
func (c *Connection) Stop() {
	c.stopOnce.Do(func() {
		c.isClosed.Store(true)
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

// 获取远程客户端地址方法
func (c *Connection) GetRemoteAddr() net.Addr {
	return c.Conn.RemoteAddr()
}
