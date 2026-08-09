package gcore

import (
	"bytes"
	"encoding/binary"
	"errors"
)

const (
	// GameProtocolMagic 用于快速判断数据是否属于 Ginx 游戏协议。
	GameProtocolMagic uint32 = 0x584E4947
	// GameProtocolVersion 表示当前游戏协议版本。
	GameProtocolVersion uint8 = 1
	// GameProtocolHeaderLen 表示游戏协议内部消息头长度。
	GameProtocolHeaderLen = 24
	// MaxGamePayloadSize 防止解包时根据异常长度分配过大的内存。
	MaxGamePayloadSize uint32 = 4 * 1024 * 1024
)

// GameMessageID 表示游戏业务消息类型。
type GameMessageID uint16

const (
	// GameMsgLoginRequest 表示登录请求。
	GameMsgLoginRequest GameMessageID = 1001
	// GameMsgLoginResponse 表示登录响应。
	GameMsgLoginResponse GameMessageID = 1002
	// GameMsgEnterRoomRequest 表示进入房间请求。
	GameMsgEnterRoomRequest GameMessageID = 2001
	// GameMsgEnterRoomResponse 表示进入房间响应。
	GameMsgEnterRoomResponse GameMessageID = 2002
	// GameMsgLeaveRoomRequest 表示离开房间请求。
	GameMsgLeaveRoomRequest GameMessageID = 2003
	// GameMsgRoomBroadcast 表示房间广播消息。
	GameMsgRoomBroadcast GameMessageID = 2004
	// GameMsgPlayerMove 表示玩家移动消息。
	GameMsgPlayerMove GameMessageID = 3001
	// GameMsgAOIChange 表示 AOI 视野变化消息。
	GameMsgAOIChange GameMessageID = 3002
	// GameMsgHeartbeat 表示游戏层心跳消息。
	GameMsgHeartbeat GameMessageID = 9001
)

const (
	// GameMessageFlagReliable 表示业务层希望可靠处理的消息。
	GameMessageFlagReliable uint8 = 1 << iota
	// GameMessageFlagResponse 表示该消息是对请求的响应。
	GameMessageFlagResponse
)

// GameMessage 是 DataPack 之上的游戏业务消息。
//
// Ginx 的 DataPack 负责 TCP 分帧，本结构负责记录版本、消息类型、序列号、
// 玩家和房间上下文。Encode 结果可以直接作为 DataPack 的消息体发送。
type GameMessage struct {
	Version   uint8
	Flags     uint8
	MessageID GameMessageID
	Sequence  uint32
	PlayerID  uint32
	RoomID    uint32
	Payload   []byte
}

// NewGameMessage 创建一个使用当前协议版本的游戏消息。
func NewGameMessage(messageID GameMessageID, sequence, playerID, roomID uint32, payload []byte) *GameMessage {
	return &GameMessage{
		Version:   GameProtocolVersion,
		MessageID: messageID,
		Sequence:  sequence,
		PlayerID:  playerID,
		RoomID:    roomID,
		Payload:   append([]byte(nil), payload...),
	}
}

// Encode 将游戏消息编码为小端序二进制数据。
func (gm *GameMessage) Encode() ([]byte, error) {
	if gm == nil {
		return nil, errors.New("game message is nil")
	}
	if gm.Version == 0 {
		gm.Version = GameProtocolVersion
	}
	if gm.Version != GameProtocolVersion {
		return nil, errors.New("unsupported game protocol version")
	}
	if len(gm.Payload) > int(MaxGamePayloadSize) {
		return nil, errors.New("game message payload is too large")
	}

	data := bytes.NewBuffer(make([]byte, 0, GameProtocolHeaderLen+len(gm.Payload)))
	writeUint32(data, GameProtocolMagic)
	data.WriteByte(gm.Version)
	data.WriteByte(gm.Flags)
	writeUint16(data, uint16(gm.MessageID))
	writeUint32(data, gm.Sequence)
	writeUint32(data, gm.PlayerID)
	writeUint32(data, gm.RoomID)
	writeUint32(data, uint32(len(gm.Payload)))
	_, _ = data.Write(gm.Payload)
	return data.Bytes(), nil
}

// DecodeGameMessage 解码一条完整的游戏协议消息。
func DecodeGameMessage(data []byte) (*GameMessage, error) {
	if len(data) < GameProtocolHeaderLen {
		return nil, errors.New("game message header is incomplete")
	}
	if binary.LittleEndian.Uint32(data[0:4]) != GameProtocolMagic {
		return nil, errors.New("invalid game protocol magic")
	}
	if data[4] != GameProtocolVersion {
		return nil, errors.New("unsupported game protocol version")
	}

	payloadLen := binary.LittleEndian.Uint32(data[20:24])
	if payloadLen > MaxGamePayloadSize {
		return nil, errors.New("game message payload is too large")
	}
	if uint64(GameProtocolHeaderLen)+uint64(payloadLen) != uint64(len(data)) {
		return nil, errors.New("game message length is invalid")
	}

	return &GameMessage{
		Version:   data[4],
		Flags:     data[5],
		MessageID: GameMessageID(binary.LittleEndian.Uint16(data[6:8])),
		Sequence:  binary.LittleEndian.Uint32(data[8:12]),
		PlayerID:  binary.LittleEndian.Uint32(data[12:16]),
		RoomID:    binary.LittleEndian.Uint32(data[16:20]),
		Payload:   append([]byte(nil), data[GameProtocolHeaderLen:]...),
	}, nil
}

func writeUint16(data *bytes.Buffer, value uint16) {
	var buffer [2]byte
	binary.LittleEndian.PutUint16(buffer[:], value)
	_, _ = data.Write(buffer[:])
}

func writeUint32(data *bytes.Buffer, value uint32) {
	var buffer [4]byte
	binary.LittleEndian.PutUint32(buffer[:], value)
	_, _ = data.Write(buffer[:])
}
