package gnet

import (
	"Ginx/gface"
	"context"
)

type Request struct {
	//已经和客户端建立好的连接
	conn gface.IConnection
	//客户端的请求数据
	data gface.IMessage
}

// Context 在连接关闭或停服时取消，旧连接实现回退为 Background。
func (r *Request) Context() context.Context {
	if c, ok := r.conn.(interface{ Context() context.Context }); ok {
		return c.Context()
	}
	return context.Background()
}

// 获取连接信息
func (r *Request) GetConnection() gface.IConnection {
	return r.conn
}

// 获取连接的请求数据
func (r *Request) GetData() []byte {
	return r.data.GetData()
}

// 获取请求的消息的ID
func (r *Request) GetMsgID() uint32 {
	return r.data.GetMsgID()
}
