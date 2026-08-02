package main

import (
	"Ginx/gnet"
	"fmt"
	"io"
	"net"
	"time"
)

type tutorialMessage struct {
	msgID uint32
	data  string
}

func main() {
	conn, err := net.Dial("tcp", "127.0.0.1:7777")
	if err != nil {
		fmt.Println("client start err, exit!", err)
		return
	}
	defer conn.Close()

	messages := []tutorialMessage{
		{msgID: 0, data: "tutorial ping"},
		{msgID: 1, data: "tutorial heartbeat"},
	}

	for {
		for _, item := range messages {
			if err := sendMessage(conn, item.msgID, []byte(item.data)); err != nil {
				fmt.Println("send message error: ", err)
				return
			}

			msgID, data, err := readMessage(conn)
			if err != nil {
				fmt.Println("read message error: ", err)
				return
			}
			fmt.Println("recv from server : msgId=", msgID, ", data=", string(data))
			time.Sleep(2 * time.Second)
		}
	}
}

func sendMessage(conn net.Conn, msgID uint32, data []byte) error {
	packet, err := gnet.NewDataPack().Pack(gnet.NewMsgPackage(msgID, data))
	if err != nil {
		return err
	}
	_, err = conn.Write(packet)
	return err
}

func readMessage(conn net.Conn) (uint32, []byte, error) {
	pack := gnet.NewDataPack()
	header := make([]byte, pack.GetHeadLen())
	if _, err := io.ReadFull(conn, header); err != nil {
		return 0, nil, err
	}

	message, err := pack.Unpack(header)
	if err != nil {
		return 0, nil, err
	}
	data := make([]byte, message.GetDataLen())
	if _, err = io.ReadFull(conn, data); err != nil {
		return 0, nil, err
	}
	return message.GetMsgID(), data, nil
}
