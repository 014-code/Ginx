package gface

type IMessage interface {
	//获取消息长度
	GetDataLen() uint32
	//获取消息ID
	GetMsgID() uint32
	//获取消息内容
	GetData() []byte
	//设置消息长度
	SetDataLen(uint32)
	//设置消息ID
	SetMsgID(uint32)
	//设置消息内容
	SetData([]byte)
}
