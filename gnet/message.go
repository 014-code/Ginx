package gnet

type Message struct {
	Id      uint32
	Data    []byte
	DataLen uint32
}

// 创建一个Message消息包
func NewMsgPackage(id uint32, data []byte) *Message {
	return &Message{
		Id:      id,
		DataLen: uint32(len(data)),
		Data:    data,
	}
}

func (m Message) GetDataLen() uint32 {
	return m.DataLen
}

func (m Message) GetMsgID() uint32 {
	return m.Id
}

func (m Message) GetData() []byte {
	return m.Data
}

func (m Message) SetDataLen(len uint32) {
	m.DataLen = len
}

func (m Message) SetMsgID(id uint32) {
	m.Id = id
}

func (m Message) SetData(data []byte) {
	m.Data = data
}
