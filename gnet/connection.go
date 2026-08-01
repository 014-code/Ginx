package gnet

import (
	"Ginx/gface"
	"errors"
	"fmt"
	"io"
	"net"
)

type Connection struct {
	//当前连接的socket TCP套接字
	Conn *net.TCPConn
	//当前连接的ID 也可以称作为SessionID，ID全局唯一
	ConnID uint32
	//当前连接的关闭状态
	isClosed bool
	//消息管理MsgId和对应处理方法的消息管理模块
	MsgHandler gface.IMsgHandle
	//告知该链接已经退出/停止的channel
	ExitBuffChan chan bool
	//无缓冲管道，用于读、写两个goroutine之间的消息通信
	msgChan chan []byte
}

// 创建连接的方法
func NewConntion(conn *net.TCPConn, connID uint32, msgHandler gface.IMsgHandle) *Connection {
	c := &Connection{
		Conn:         conn,
		ConnID:       connID,
		isClosed:     false,
		MsgHandler:   msgHandler,
		ExitBuffChan: make(chan bool, 1),
		msgChan:      make(chan []byte), //msgChan初始化
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
			if _, err := c.Conn.Write(data); err != nil {
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
	if c.isClosed == true {
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
	c.msgChan <- msg //将之前直接回写给conn.Write的方法 改为 发送给Channel 供Writer读取

	return nil
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
		//创建连接对象拆包解包对象
		pack := NewDataPack()

		//读取msg的Head
		headData := make([]byte, pack.GetHeadLen())

		_, err := io.ReadFull(c.GetTCPConnection(), headData)
		if err != nil {
			fmt.Println("Read Head Data error: ", err)
			return
		}
		//拆包
		msg, err := pack.Unpack(headData)
		if err != nil {
			fmt.Println("Unpack Head Data error: ", err)
			return
		}

		//根据长度读取data
		//构建一个和长度一样大小的缓冲区，长度头就是后续数据体的内容了
		var data []byte
		if msg.GetDataLen() > 0 {
			data = make([]byte, msg.GetDataLen())
			_, err := io.ReadFull(c.GetTCPConnection(), data)
			if err != nil {
				fmt.Println("Read Head Data error: ", err)
				return
			}
		}
		//set进去
		msg.SetData(data)
		//得到当前客户端的请求requster数据
		req := Request{
			conn: c,
			data: msg,
		}

		//从绑定好的消息和对应的处理方法中执行对应的Handle方法
		go c.MsgHandler.DoMsgHandler(&req)
	}
}

// 启动连接
func (c *Connection) Start() {
	//1 开启用户从客户端读取数据流程的Goroutine
	go c.StartReader()
	//2 开启用于写回客户端数据流程的Goroutine
	go c.StartWriter()

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
	//1. 如果当前链接已经关闭
	if c.isClosed == true {
		return
	}
	c.isClosed = true
	//TODO Connection Stop() 如果用户注册了该链接的关闭回调业务，那么在此刻应该显示调用

	// 关闭socket链接
	c.Conn.Close()

	//写入关闭
	c.ExitBuffChan <- true
	//关闭所有chan管道
	close(c.ExitBuffChan)
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
