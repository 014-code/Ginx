package utils

import (
	"encoding/json"
	"io/ioutil"
	"os"
)

/*
保存 Ginx 服务配置，可显式从 ginx.json 加载。
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
	WriteTimeout            uint32 //单包 socket 写入超时，毫秒；0 不限制
	SendTimeout             uint32 //SendMsg 等待发送锁和入队的总超时，毫秒；0 不限制
	MessageRateLimit        int    //每条连接每秒允许处理的最大消息数，0表示关闭
	MessageRateBurst        int    //每条连接允许的突发消息数，0表示使用MessageRateLimit
}

/*
定义一个全局的对象
*/
// GlobalObject 仅供旧 API 使用；包导入时不再隐式读取磁盘。
var GlobalObject = func() *GlobalObj { config := DefaultConfig(); return &config }()

// LoadConfig 显式读取指定文件，在默认值上覆盖字段，不修改全局状态。
func LoadConfig(path string) (GlobalObj, error) {
	config := DefaultConfig()
	data, err := os.ReadFile(path)
	if err != nil {
		return config, err
	}
	err = json.Unmarshal(data, &config)
	return config, err
}

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

// DefaultConfig 返回独立的默认配置，不读取文件。
func DefaultConfig() GlobalObj {
	return GlobalObj{
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
}
