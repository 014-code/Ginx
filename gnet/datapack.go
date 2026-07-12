package gnet

import (
	"Ginx/gface"
	"bytes"
	"encoding/binary"
)

type DataPack struct {
}

func (d DataPack) GetHeadLen() uint32 {
	//Id uint32(4字节) +  DataLen uint32(4字节)
	return 8
}

func (d DataPack) Pack(msg gface.IMessage) ([]byte, error) {
	//创建缓冲区
	buffer := bytes.NewBuffer([]byte{})

	//写长度
	if err := binary.Write(buffer, binary.LittleEndian, msg.GetMsgID()); err != nil {
		return nil, err
	}
	//写Id
	if err := binary.Write(buffer, binary.LittleEndian, msg.GetData()); err != nil {
		return nil, err
	}
	//写内容
	if err := binary.Write(buffer, binary.LittleEndian, msg.GetData()); err != nil {
		return nil, err
	}
	return buffer.Bytes(), nil
}

func (d DataPack) Unpack(bd []byte) (gface.IMessage, error) {
	//创建缓冲区
	reader := bytes.NewReader(bd)

	//创建一个空Message对象
	message := &Message{}
	//读长度
	if err := binary.Read(reader, binary.LittleEndian, &message.DataLen); err != nil {
		return nil, err
	}
	//读Id
	if err := binary.Read(reader, binary.LittleEndian, &message.Id); err != nil {
		return nil, err
	}
	//读内容
	if err := binary.Read(reader, binary.LittleEndian, &message.Data); err != nil {
		return nil, err
	}
	return message, nil
}

func NewDataPack() *DataPack {
	return &DataPack{}
}
