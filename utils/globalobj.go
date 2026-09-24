package utils

import (
	"encoding/json"
	"io/ioutil"
)

/*
存储一切有关Zinx框架的全局参数，供其他模块使用
一些参数也可以通过 用户根据 zinx.json来配置
*/
type GlobalObj struct {
	Host    string //当前服务器主机IP
	TcpPort int    //当前服务器主机监听端口号
	Name    string //当前服务器名称

	MaxPacketSize           uint32 //都需数据包的最大值
	MaxMsgChanLen           uint32 //连接发送消息缓冲队列的最大长度
	MaxConn                 int    //当前服务器主机允许的最大链接个数
	WorkerPoolSize          uint32 //业务工作Worker池的数量
	MaxWorkerTaskLen        uint32 //业务工作Worker对应负责的任务队列最大任务存储数量
	WorkerTaskQueueWaitTime uint32 //Worker任务队列最大等待时间，单位毫秒
	HeartbeatMax            int    //当前连接允许的最大心跳超时时间，单位秒
	MessageRateLimit        int    //每条连接每秒允许处理的最大消息数，0表示关闭
	MessageRateBurst        int    //每条连接允许的突发消息数，0表示使用MessageRateLimit
}

/*
定义一个全局的对象
*/
var GlobalObject *GlobalObj

// 读取用户的配置文件
func (g *GlobalObj) Reload() {
	data, err := ioutil.ReadFile("config/ginx.json")
	if err != nil {
		data, err = ioutil.ReadFile("../config/ginx.json")
		if err != nil {
			return
		}
	}
	//将json数据解析到struct中
	//fmt.Printf("json :%s\n", data)
	err = json.Unmarshal(data, g)
	if err != nil {
		panic(err)
	}
}

/*
提供init方法，默认加载
*/
func init() {
	//初始化GlobalObject变量，设置一些默认值
	GlobalObject = &GlobalObj{
		Name:                    "ZinxServerApp",
		TcpPort:                 7777,
		Host:                    "0.0.0.0",
		MaxConn:                 12000,
		MaxPacketSize:           4096,
		MaxMsgChanLen:           1024,
		WorkerPoolSize:          10,
		MaxWorkerTaskLen:        1024,
		WorkerTaskQueueWaitTime: 100,
		HeartbeatMax:            10,
		MessageRateLimit:        0,
		MessageRateBurst:        0,
	}

	//从配置文件中加载一些用户配置的参数
	GlobalObject.Reload()
}
