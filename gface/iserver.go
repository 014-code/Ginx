package gface

// 服务器接口
type IServer interface {
	//启动服务器方法
	Start()
	//停止服务器方法
	Stop()
	//开启业务服务方法
	Serve()
	//路由，给当前服务对象注册一个路由业务方法
	AddRouter(router IRouter)
}
