package gface

import "net"

// 连接接口
type IConnection interface {
	//启动连接
	Start()
	//停止连接
	Stop()
	//从当前连接获取原始的socket
	GetConnId() uint32
	//获取当前连接的原生socket
	GetConnection() net.Conn
	//获取远程客户端地址信息
	RemoteAddr() net.Addr
	//直接将Message数据发送数据给远程的TCP客户端
	SendMsg(msgId uint32, data []byte) error
	//直接将Message数据发送数据给远程的TCP客户端(有缓冲)
	SendBuffMsg(msgId uint32, data []byte) error
	//设置连接属性
	SetProperty(key string, value interface{})
	//获取连接属性
	GetProperty(key string) (interface{}, error)
	//移除连接属性
	RemoveProperty(key string)
}

// 连接生命周期回调方法
type HookFunc func(connection IConnection)
