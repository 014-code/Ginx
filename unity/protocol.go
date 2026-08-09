package unity

import (
	"encoding/binary"
	"errors"
	"fmt"
	"math"
)

// Unity 消息 ID 与 mmo_demo_unity3d 客户端保持一致。
const (
	UnityMsgSyncPID     uint32 = 1
	UnityMsgTalk        uint32 = 2
	UnityMsgMove        uint32 = 3
	UnityMsgBroadCast   uint32 = 200
	UnityMsgPlayerLeave uint32 = 201
	UnityMsgSyncPlayers uint32 = 202
	UnityMsgCreateRoom  uint32 = 203
	UnityMsgJoinRoom    uint32 = 204
	UnityMsgLeaveRoom   uint32 = 205
	UnityMsgRoomEvent   uint32 = 206
)

// UnityPosition 对应 Unity 客户端中的 Pb.Position。
type UnityPosition struct {
	X float32
	Y float32
	Z float32
	V float32
}

// UnitySyncPID 对应 Unity 客户端中的 Pb.SyncPid。
type UnitySyncPID struct {
	PID int32
}

// UnityPlayer 对应 Unity 客户端中的 Pb.Player。
type UnityPlayer struct {
	PID int32
	P   UnityPosition
}

// UnitySyncPlayers 对应 Unity 客户端中的 Pb.SyncPlayers。
type UnitySyncPlayers struct {
	Players []UnityPlayer
}

// UnityMovePackage 对应 Unity 客户端中的 Pb.MovePackege。
type UnityMovePackage struct {
	P          UnityPosition
	ActionData int32
}

// UnityTalk 对应 Unity 客户端中的 Pb.Talk。
type UnityTalk struct {
	Content string
}

// UnityBroadCast 对应 Unity 客户端中的 Pb.BroadCast。
type UnityBroadCast struct {
	PID        int32
	TP         int32
	Content    string
	P          *UnityPosition
	ActionData int32
}

// UnityRoomRequest 表示 Unity 客户端的房间操作请求。
type UnityRoomRequest struct {
	RoomID uint32
}

// UnityRoomResponse 表示 Unity 客户端的房间操作结果。
type UnityRoomResponse struct {
	RoomID      uint32
	Code        int32
	Message     string
	PlayerCount uint32
}

// UnityRoomEvent 表示房间内的业务广播消息。
type UnityRoomEvent struct {
	RoomID   uint32
	PlayerID int32
	Content  string
}

func (p UnityPosition) Marshal() ([]byte, error) {
	data := make([]byte, 0, 24)
	data = appendFixed32(data, 1, p.X)
	data = appendFixed32(data, 2, p.Y)
	data = appendFixed32(data, 3, p.Z)
	data = appendFixed32(data, 4, p.V)
	return data, nil
}

func (p *UnityPosition) Unmarshal(data []byte) error {
	if p == nil {
		return errors.New("unity position is nil")
	}
	return decodeUnityFields(data, func(fieldNum int, wireType int, value []byte, number uint64) error {
		switch fieldNum {
		case 1, 2, 3, 4:
			if wireType != 5 || len(value) != 4 {
				return fmt.Errorf("unity position field %d has invalid wire type", fieldNum)
			}
			fieldValue := mathFloat32(binary.LittleEndian.Uint32(value))
			switch fieldNum {
			case 1:
				p.X = fieldValue
			case 2:
				p.Y = fieldValue
			case 3:
				p.Z = fieldValue
			case 4:
				p.V = fieldValue
			}
		}
		return nil
	})
}

func (m UnitySyncPID) Marshal() ([]byte, error) {
	return appendVarint(nil, 1, uint64(int64(m.PID))), nil
}

func (m *UnitySyncPID) Unmarshal(data []byte) error {
	if m == nil {
		return errors.New("unity sync pid is nil")
	}
	return decodeUnityFields(data, func(fieldNum int, wireType int, value []byte, number uint64) error {
		if fieldNum == 1 {
			if wireType != 0 {
				return errors.New("unity sync pid has invalid pid wire type")
			}
			m.PID = int32(number)
		}
		return nil
	})
}

func (m UnityPlayer) Marshal() ([]byte, error) {
	position, err := m.P.Marshal()
	if err != nil {
		return nil, err
	}
	data := appendVarint(nil, 1, uint64(int64(m.PID)))
	data = appendBytes(data, 2, position)
	return data, nil
}

func (m *UnityPlayer) Unmarshal(data []byte) error {
	if m == nil {
		return errors.New("unity player is nil")
	}
	return decodeUnityFields(data, func(fieldNum int, wireType int, value []byte, number uint64) error {
		switch fieldNum {
		case 1:
			if wireType != 0 {
				return errors.New("unity player has invalid pid wire type")
			}
			m.PID = int32(number)
		case 2:
			if wireType != 2 {
				return errors.New("unity player has invalid position wire type")
			}
			return m.P.Unmarshal(value)
		}
		return nil
	})
}

func (m UnitySyncPlayers) Marshal() ([]byte, error) {
	data := make([]byte, 0, len(m.Players)*16)
	for _, player := range m.Players {
		playerData, err := player.Marshal()
		if err != nil {
			return nil, err
		}
		data = appendBytes(data, 1, playerData)
	}
	return data, nil
}

func (m *UnitySyncPlayers) Unmarshal(data []byte) error {
	if m == nil {
		return errors.New("unity sync players is nil")
	}
	m.Players = nil
	return decodeUnityFields(data, func(fieldNum int, wireType int, value []byte, number uint64) error {
		if fieldNum == 1 {
			if wireType != 2 {
				return errors.New("unity sync players has invalid player wire type")
			}
			player := UnityPlayer{}
			if err := player.Unmarshal(value); err != nil {
				return err
			}
			m.Players = append(m.Players, player)
		}
		return nil
	})
}

func (m UnityMovePackage) Marshal() ([]byte, error) {
	position, err := m.P.Marshal()
	if err != nil {
		return nil, err
	}
	data := appendBytes(nil, 1, position)
	data = appendVarint(data, 2, uint64(int64(m.ActionData)))
	return data, nil
}

func (m *UnityMovePackage) Unmarshal(data []byte) error {
	if m == nil {
		return errors.New("unity move package is nil")
	}
	return decodeUnityFields(data, func(fieldNum int, wireType int, value []byte, number uint64) error {
		switch fieldNum {
		case 1:
			if wireType != 2 {
				return errors.New("unity move package has invalid position wire type")
			}
			return m.P.Unmarshal(value)
		case 2:
			if wireType != 0 {
				return errors.New("unity move package has invalid action wire type")
			}
			m.ActionData = int32(number)
		}
		return nil
	})
}

func (m UnityTalk) Marshal() ([]byte, error) {
	return appendBytes(nil, 1, []byte(m.Content)), nil
}

func (m *UnityTalk) Unmarshal(data []byte) error {
	if m == nil {
		return errors.New("unity talk is nil")
	}
	return decodeUnityFields(data, func(fieldNum int, wireType int, value []byte, number uint64) error {
		if fieldNum == 1 {
			if wireType != 2 {
				return errors.New("unity talk has invalid content wire type")
			}
			m.Content = string(value)
		}
		return nil
	})
}

func (m UnityBroadCast) Marshal() ([]byte, error) {
	data := make([]byte, 0, 32)
	data = appendVarint(data, 1, uint64(int64(m.PID)))
	data = appendVarint(data, 2, uint64(int64(m.TP)))
	if m.Content != "" {
		data = appendBytes(data, 3, []byte(m.Content))
	}
	if m.P != nil {
		position, err := m.P.Marshal()
		if err != nil {
			return nil, err
		}
		data = appendBytes(data, 4, position)
	}
	if m.ActionData != 0 {
		data = appendVarint(data, 5, uint64(int64(m.ActionData)))
	}
	return data, nil
}

func (m *UnityBroadCast) Unmarshal(data []byte) error {
	if m == nil {
		return errors.New("unity broadcast is nil")
	}
	return decodeUnityFields(data, func(fieldNum int, wireType int, value []byte, number uint64) error {
		switch fieldNum {
		case 1, 2, 5:
			if wireType != 0 {
				return fmt.Errorf("unity broadcast field %d has invalid wire type", fieldNum)
			}
			switch fieldNum {
			case 1:
				m.PID = int32(number)
			case 2:
				m.TP = int32(number)
			case 5:
				m.ActionData = int32(number)
			}
		case 3:
			if wireType != 2 {
				return errors.New("unity broadcast has invalid content wire type")
			}
			m.Content = string(value)
		case 4:
			if wireType != 2 {
				return errors.New("unity broadcast has invalid position wire type")
			}
			position := &UnityPosition{}
			if err := position.Unmarshal(value); err != nil {
				return err
			}
			m.P = position
		}
		return nil
	})
}

func (m UnityRoomRequest) Marshal() ([]byte, error) {
	return appendVarint(nil, 1, uint64(m.RoomID)), nil
}

func (m *UnityRoomRequest) Unmarshal(data []byte) error {
	if m == nil {
		return errors.New("unity room request is nil")
	}
	return decodeUnityFields(data, func(fieldNum int, wireType int, value []byte, number uint64) error {
		if fieldNum == 1 {
			if wireType != 0 {
				return errors.New("unity room request has invalid room id wire type")
			}
			m.RoomID = uint32(number)
		}
		return nil
	})
}

func (m UnityRoomResponse) Marshal() ([]byte, error) {
	data := appendVarint(nil, 1, uint64(m.RoomID))
	data = appendVarint(data, 2, uint64(int64(m.Code)))
	if m.Message != "" {
		data = appendBytes(data, 3, []byte(m.Message))
	}
	data = appendVarint(data, 4, uint64(m.PlayerCount))
	return data, nil
}

func (m *UnityRoomResponse) Unmarshal(data []byte) error {
	if m == nil {
		return errors.New("unity room response is nil")
	}
	return decodeUnityFields(data, func(fieldNum int, wireType int, value []byte, number uint64) error {
		switch fieldNum {
		case 1, 2, 4:
			if wireType != 0 {
				return fmt.Errorf("unity room response field %d has invalid wire type", fieldNum)
			}
			switch fieldNum {
			case 1:
				m.RoomID = uint32(number)
			case 2:
				m.Code = int32(number)
			case 4:
				m.PlayerCount = uint32(number)
			}
		case 3:
			if wireType != 2 {
				return errors.New("unity room response has invalid message wire type")
			}
			m.Message = string(value)
		}
		return nil
	})
}

func (m UnityRoomEvent) Marshal() ([]byte, error) {
	data := appendVarint(nil, 1, uint64(m.RoomID))
	data = appendVarint(data, 2, uint64(int64(m.PlayerID)))
	if m.Content != "" {
		data = appendBytes(data, 3, []byte(m.Content))
	}
	return data, nil
}

func (m *UnityRoomEvent) Unmarshal(data []byte) error {
	if m == nil {
		return errors.New("unity room event is nil")
	}
	return decodeUnityFields(data, func(fieldNum int, wireType int, value []byte, number uint64) error {
		switch fieldNum {
		case 1, 2:
			if wireType != 0 {
				return fmt.Errorf("unity room event field %d has invalid wire type", fieldNum)
			}
			if fieldNum == 1 {
				m.RoomID = uint32(number)
			} else {
				m.PlayerID = int32(number)
			}
		case 3:
			if wireType != 2 {
				return errors.New("unity room event has invalid content wire type")
			}
			m.Content = string(value)
		}
		return nil
	})
}

func appendVarint(data []byte, fieldNum int, value uint64) []byte {
	data = append(data, byte(fieldNum<<3))
	for value >= 0x80 {
		data = append(data, byte(value)|0x80)
		value >>= 7
	}
	return append(data, byte(value))
}

func appendFixed32(data []byte, fieldNum int, value float32) []byte {
	data = append(data, byte(fieldNum<<3|5))
	var buffer [4]byte
	binary.LittleEndian.PutUint32(buffer[:], float32Bits(value))
	return append(data, buffer[:]...)
}

func appendBytes(data []byte, fieldNum int, value []byte) []byte {
	data = append(data, byte(fieldNum<<3|2))
	data = appendVarintValue(data, uint64(len(value)))
	return append(data, value...)
}

func appendVarintValue(data []byte, value uint64) []byte {
	for value >= 0x80 {
		data = append(data, byte(value)|0x80)
		value >>= 7
	}
	return append(data, byte(value))
}

func decodeUnityFields(data []byte, callback func(int, int, []byte, uint64) error) error {
	for offset := 0; offset < len(data); {
		tag, next, err := readUnityVarint(data, offset)
		if err != nil {
			return err
		}
		offset = next
		fieldNum := int(tag >> 3)
		wireType := int(tag & 7)
		if fieldNum <= 0 {
			return errors.New("unity protobuf field number is invalid")
		}

		var value []byte
		var number uint64
		switch wireType {
		case 0:
			number, offset, err = readUnityVarint(data, offset)
		case 1:
			if offset+8 > len(data) {
				return errors.New("unity protobuf fixed64 field is incomplete")
			}
			value = data[offset : offset+8]
			offset += 8
		case 2:
			length, lengthEnd, lengthErr := readUnityVarint(data, offset)
			if lengthErr != nil {
				return lengthErr
			}
			offset = lengthEnd
			if length > uint64(len(data)-offset) {
				return errors.New("unity protobuf bytes field is incomplete")
			}
			value = data[offset : offset+int(length)]
			offset += int(length)
		case 5:
			if offset+4 > len(data) {
				return errors.New("unity protobuf fixed32 field is incomplete")
			}
			value = data[offset : offset+4]
			offset += 4
		default:
			return fmt.Errorf("unity protobuf wire type %d is unsupported", wireType)
		}
		if err != nil {
			return err
		}
		if err := callback(fieldNum, wireType, value, number); err != nil {
			return err
		}
	}
	return nil
}

func readUnityVarint(data []byte, offset int) (uint64, int, error) {
	var value uint64
	for index := 0; index < 10; index++ {
		if offset >= len(data) {
			return 0, offset, errors.New("unity protobuf varint is incomplete")
		}
		current := data[offset]
		offset++
		if index == 9 && current > 1 {
			return 0, offset, errors.New("unity protobuf varint overflows uint64")
		}
		value |= uint64(current&0x7f) << (7 * index)
		if current < 0x80 {
			return value, offset, nil
		}
	}
	return 0, offset, errors.New("unity protobuf varint is too long")
}

func float32Bits(value float32) uint32 {
	return math.Float32bits(value)
}

func mathFloat32(value uint32) float32 {
	return math.Float32frombits(value)
}
