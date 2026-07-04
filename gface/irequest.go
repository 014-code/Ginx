package gface

type IRequest interface {
	//获取请求连接本身的信息
	GetConnection() IConnection
	//获取请求的数据
	GetData() []byte
}
