package main

import (
	"Ginx/gface"
	"Ginx/gnet"
	"fmt"
)

// ping test 自定义路由
type PingRouter struct {
	gnet.BaseRouter //一定要先基础BaseRouter
}

// Test PreHandle
func (this *PingRouter) PreHandle(request gface.IRequest) {
	fmt.Println("Call Router PreHandle")
	_, err := request.GetConnection().GetConnection().Write([]byte("before ping ....\n"))
	if err != nil {
		fmt.Println("call back ping ping ping error")
	}
}

// Test Handle
func (this *PingRouter) Handle(request gface.IRequest) {
	fmt.Println("Call PingRouter Handle")
	_, err := request.GetConnection().GetConnection().Write([]byte("ping...ping...ping\n"))
	if err != nil {
		fmt.Println("call back ping ping ping error")
	}
}

// Test PostHandle
func (this *PingRouter) PostHandle(request gface.IRequest) {
	fmt.Println("Call Router PostHandle")
	_, err := request.GetConnection().GetConnection().Write([]byte("After ping .....\n"))
	if err != nil {
		fmt.Println("call back ping ping ping error")
	}
}

func main() {
	//创建一个server句柄
	s := gnet.NewServer()

	s.AddRouter(&PingRouter{})

	//2 开启服务
	s.Serve()
}
