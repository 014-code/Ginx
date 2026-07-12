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
}

/*
*
函数类型：连接时的处理
参数1：socket原生连接
参数2：客户端请求的数据
参数3：客户端请求的数据长度
返回值：错误内容
*/
type HandFunc func(*net.TCPConn, []byte, int) error
