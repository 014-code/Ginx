package gnet

import (
	"Ginx/gface"
	"Ginx/utils"
	"bytes"
	"encoding/binary"
	"errors"
	"io"
)

type DataPack struct {
}

func (d DataPack) GetHeadLen() uint32 {
	//DataLen uint32(4字节) +  Id uint32(4字节)
	return 8
}

func (d DataPack) Pack(msg gface.IMessage) ([]byte, error) {
	if msg == nil {
		return nil, errors.New("message is nil")
	}
	if msg.GetDataLen() != uint32(len(msg.GetData())) {
		return nil, errors.New("message data length does not match payload")
	}
	if utils.GlobalObject.MaxPacketSize > 0 && msg.GetDataLen() > utils.GlobalObject.MaxPacketSize {
		return nil, errors.New("too large msg data sent")
	}

	//创建缓冲区
	buffer := bytes.NewBuffer([]byte{})

	//写长度
	if err := binary.Write(buffer, binary.LittleEndian, msg.GetDataLen()); err != nil {
		return nil, err
	}
	//写Id
	if err := binary.Write(buffer, binary.LittleEndian, msg.GetMsgID()); err != nil {
		return nil, err
	}
	//写内容
	if err := binary.Write(buffer, binary.LittleEndian, msg.GetData()); err != nil {
		return nil, err
	}
	return buffer.Bytes(), nil
}

func (d DataPack) Unpack(bd []byte) (gface.IMessage, error) {
	if uint32(len(bd)) != d.GetHeadLen() {
		return nil, errors.New("invalid message header length")
	}

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

	//判断数据包的长度是否超出我们允许的最大包长度
	if utils.GlobalObject.MaxPacketSize > 0 && message.DataLen > utils.GlobalObject.MaxPacketSize {
		return nil, errors.New("too large msg data received")
	}
	return message, nil
}

// ReadMessage 从 TCP 字节流中读取一条完整消息。
// io.ReadFull 会处理半包，当前消息读取完成后，后续粘包数据会留在 reader 中。
func (d DataPack) ReadMessage(reader io.Reader) (gface.IMessage, error) {
	if reader == nil {
		return nil, errors.New("message reader is nil")
	}

	header := make([]byte, d.GetHeadLen())
	if _, err := io.ReadFull(reader, header); err != nil {
		return nil, err
	}

	message, err := d.Unpack(header)
	if err != nil {
		return nil, err
	}
	if message.GetDataLen() == 0 {
		message.SetData([]byte{})
		return message, nil
	}

	data := make([]byte, message.GetDataLen())
	if _, err := io.ReadFull(reader, data); err != nil {
		return nil, err
	}
	message.SetData(data)
	return message, nil
}

func NewDataPack() *DataPack {
	return &DataPack{}
}
