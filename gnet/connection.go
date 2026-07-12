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
	//当前连接的ID SessionID全局唯一
	ConnID uint32
	//当前连接的关闭状态
	isClosed bool
	//该连接的处理方法api
	handleAPI gface.HandFunc
	//告知该链接已经退出/停止的chan
	ExitBuffChan chan bool
	//该连接的处理方法router
	Router gface.IRouter
}

func (c *Connection) RemoteAddr() net.Addr {
	//TODO implement me
	panic("implement me")
}

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
	if _, err := c.Conn.Write(msg); err != nil {
		fmt.Println("Write msg id ", msgId, " error ")
		c.ExitBuffChan <- true
		return errors.New("conn Write error")
	}

	return nil
}

// 创建连接对象
func NewConntion(conn *net.TCPConn, connID uint32, router gface.IRouter, callbackFunc gface.HandFunc) *Connection {
	c := &Connection{
		Conn:         conn,
		ConnID:       connID,
		isClosed:     false,
		Router:       router,
		ExitBuffChan: make(chan bool, 1),
		handleAPI:    callbackFunc,
	}

	return c
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
			c.ExitBuffChan <- true
			continue
		}
		//拆包
		msg, err := pack.Unpack(headData)
		if err != nil {
			fmt.Println("Unpack Head Data error: ", err)
			c.ExitBuffChan <- true
			continue
		}

		//根据长度读取data
		//构建一个和长度一样大小的缓冲区，长度头就是后续数据体的内容了
		var data []byte
		if msg.GetDataLen() > 0 {
			data = make([]byte, msg.GetDataLen())
			_, err := io.ReadFull(c.GetTCPConnection(), data)
			if err != nil {
				fmt.Println("Read Head Data error: ", err)
				c.ExitBuffChan <- true
				continue
			}
		}
		//set进去
		msg.SetData(data)
		//得到当前客户端的请求requster数据
		req := Request{
			conn: c,
			data: msg,
		}

		go func(requester gface.IRequest) {
			//注册路由的三个方法
			c.Router.PreHandle(requester)
			c.Router.Handle(requester)
			c.Router.PostHandle(requester)
		}(&req)
	}
}

// 启动连接
func (c *Connection) Start() {
	//开启读取者
	go c.StartReader()

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
