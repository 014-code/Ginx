package gnet

import "Ginx/gface"

type Request struct {
	//已经和客户端建立好的连接
	conn gface.IConnection
	//客户端的请求数据
	data []byte
}

// 获取连接信息
func (r *Request) GetConnection() gface.IConnection {
	return r.conn
}

// 获取连接的请求数据
func (r *Request) GetData() []byte {
	return r.data
}
