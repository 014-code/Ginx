package gcore

import (
	"Ginx/gface"
	"errors"
	"sort"
	"sync"
)

// RoomState 表示游戏房间当前所处的生命周期状态。
type RoomState uint8

const (
	// RoomStateWaiting 表示房间等待玩家加入。
	RoomStateWaiting RoomState = iota
	// RoomStateRunning 表示房间已经开始游戏。
	RoomStateRunning
	// RoomStateClosed 表示房间已经关闭。
	RoomStateClosed
)

// String 返回房间状态的可读名称。
func (rs RoomState) String() string {
	switch rs {
	case RoomStateWaiting:
		return "waiting"
	case RoomStateRunning:
		return "running"
	case RoomStateClosed:
		return "closed"
	default:
		return "unknown"
	}
}

// BroadcastError 记录房间广播给某个连接时产生的错误。
type BroadcastError struct {
	ConnID uint32
	Err    error
}

// Room 表示一个可以容纳多个连接的游戏房间。
//
// Room 只负责房间成员和广播，不负责创建连接、玩家登录和游戏帧循环。
// 这些职责由 Server 或具体游戏业务实现。
type Room struct {
	ID         uint32
	MaxPlayers int
	state      RoomState
	members    map[uint32]gface.IConnection
	memberLock sync.RWMutex
}

// NewRoom 创建一个房间。MaxPlayers 小于等于 0 时表示不限制人数。
func NewRoom(roomID uint32, maxPlayers int) *Room {
	return &Room{
		ID:         roomID,
		MaxPlayers: maxPlayers,
		state:      RoomStateWaiting,
		members:    make(map[uint32]gface.IConnection),
	}
}

// Join 将连接加入房间。
func (r *Room) Join(connection gface.IConnection) error {
	if connection == nil {
		return errors.New("room connection is nil")
	}

	r.memberLock.Lock()
	defer r.memberLock.Unlock()

	if r.state == RoomStateClosed {
		return errors.New("room is closed")
	}
	if _, ok := r.members[connection.GetConnId()]; ok {
		return errors.New("connection already joined room")
	}
	if r.MaxPlayers > 0 && len(r.members) >= r.MaxPlayers {
		return errors.New("room is full")
	}
	r.members[connection.GetConnId()] = connection
	return nil
}

// Leave 将连接从房间中移除。
func (r *Room) Leave(connID uint32) error {
	r.memberLock.Lock()
	defer r.memberLock.Unlock()

	if _, ok := r.members[connID]; !ok {
		return errors.New("connection is not in room")
	}
	delete(r.members, connID)
	return nil
}

// Start 将房间从等待状态切换到运行状态。
func (r *Room) Start() error {
	r.memberLock.Lock()
	defer r.memberLock.Unlock()

	if r.state == RoomStateClosed {
		return errors.New("room is closed")
	}
	r.state = RoomStateRunning
	return nil
}

// Close 关闭房间并清理房间成员引用，但不会停止底层连接。
func (r *Room) Close() {
	r.memberLock.Lock()
	defer r.memberLock.Unlock()
	r.state = RoomStateClosed
	r.members = make(map[uint32]gface.IConnection)
}

// State 获取房间当前状态。
func (r *Room) State() RoomState {
	r.memberLock.RLock()
	defer r.memberLock.RUnlock()
	return r.state
}

// PlayerCount 获取当前房间人数。
func (r *Room) PlayerCount() int {
	r.memberLock.RLock()
	defer r.memberLock.RUnlock()
	return len(r.members)
}

// Members 返回房间成员快照，调用方可以安全地在锁外遍历。
func (r *Room) Members() []gface.IConnection {
	r.memberLock.RLock()
	defer r.memberLock.RUnlock()

	connIDs := make([]uint32, 0, len(r.members))
	connections := make(map[uint32]gface.IConnection, len(r.members))
	for connID, connection := range r.members {
		connIDs = append(connIDs, connID)
		connections[connID] = connection
	}
	sort.Slice(connIDs, func(i, j int) bool { return connIDs[i] < connIDs[j] })

	result := make([]gface.IConnection, 0, len(connIDs))
	for _, connID := range connIDs {
		result = append(result, connections[connID])
	}
	return result
}

// Broadcast 向房间内全部连接发送消息。
// 网络发送在锁外执行，避免慢客户端阻塞房间成员的加入和退出。
func (r *Room) Broadcast(msgID uint32, data []byte) []BroadcastError {
	return r.broadcast(msgID, data, 0, false)
}

// BroadcastExcept 向房间内除指定连接外的其他连接发送消息。
func (r *Room) BroadcastExcept(excludeConnID uint32, msgID uint32, data []byte) []BroadcastError {
	return r.broadcast(msgID, data, excludeConnID, true)
}

func (r *Room) broadcast(msgID uint32, data []byte, excludeConnID uint32, exclude bool) []BroadcastError {
	members := r.Members()
	errorsFound := make([]BroadcastError, 0)
	for _, connection := range members {
		if exclude && connection.GetConnId() == excludeConnID {
			continue
		}
		if err := connection.SendBuffMsg(msgID, data); err != nil {
			errorsFound = append(errorsFound, BroadcastError{
				ConnID: connection.GetConnId(),
				Err:    err,
			})
		}
	}
	return errorsFound
}

// RoomManager 管理服务器内的全部游戏房间。
type RoomManager struct {
	rooms map[uint32]*Room
	lock  sync.RWMutex
}

// NewRoomManager 创建房间管理器。
func NewRoomManager() *RoomManager {
	return &RoomManager{
		rooms: make(map[uint32]*Room),
	}
}

// CreateRoom 创建并登记一个新房间。
func (rm *RoomManager) CreateRoom(roomID uint32, maxPlayers int) (*Room, error) {
	rm.lock.Lock()
	defer rm.lock.Unlock()

	if _, ok := rm.rooms[roomID]; ok {
		return nil, errors.New("room already exists")
	}
	room := NewRoom(roomID, maxPlayers)
	rm.rooms[roomID] = room
	return room, nil
}

// GetRoom 获取指定房间。
func (rm *RoomManager) GetRoom(roomID uint32) (*Room, error) {
	rm.lock.RLock()
	defer rm.lock.RUnlock()

	room, ok := rm.rooms[roomID]
	if !ok {
		return nil, errors.New("room not found")
	}
	return room, nil
}

// RemoveRoom 关闭并移除指定房间。
func (rm *RoomManager) RemoveRoom(roomID uint32) error {
	rm.lock.Lock()
	room, ok := rm.rooms[roomID]
	if ok {
		delete(rm.rooms, roomID)
	}
	rm.lock.Unlock()

	if !ok {
		return errors.New("room not found")
	}
	room.Close()
	return nil
}

// Len 获取当前房间数量。
func (rm *RoomManager) Len() int {
	rm.lock.RLock()
	defer rm.lock.RUnlock()
	return len(rm.rooms)
}
