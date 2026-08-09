package main

import (
	"Ginx/gface"
	"Ginx/gnet"
	"fmt"
)

type PingRouter struct {
	gnet.BaseRouter
}

func (this *PingRouter) Handle(request gface.IRequest) {
	fmt.Println("Call PingRouter Handle")
	fmt.Println("recv from client : msgId=", request.GetMsgID(), ", data=", string(request.GetData()))

	if err := request.GetConnection().SendBuffMsg(0, []byte("pong from tutorial server")); err != nil {
		fmt.Println(err)
	}
}

type HeartbeatRouter struct {
	gnet.BaseRouter
}

func (this *HeartbeatRouter) Handle(request gface.IRequest) {
	fmt.Println("Call HeartbeatRouter Handle")
	fmt.Println("recv from client : msgId=", request.GetMsgID(), ", data=", string(request.GetData()))

	if err := request.GetConnection().SendBuffMsg(1, []byte("heartbeat accepted")); err != nil {
		fmt.Println(err)
	}
}

func main() {
	server := gnet.NewServer()
	server.SetOnConnStart(func(connection gface.IConnection) {
		fmt.Println("client connected:", connection.GetConnId())
	})
	server.SetOnConnStop(func(connection gface.IConnection) {
		fmt.Println("client disconnected:", connection.GetConnId())
	})
	server.AddRouter(0, &PingRouter{})
	server.AddRouter(1, &HeartbeatRouter{})
	server.Serve()
}
