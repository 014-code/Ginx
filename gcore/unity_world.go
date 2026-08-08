package gcore

import (
	"Ginx/gface"
	"errors"
	"sort"
	"sync"
)

// UnityWorld 保存 Unity MMO 客户端当前在线的玩家状态。
// 当前版本先使用全量广播，后续可以把广播范围替换成 AOI 查询结果。
type UnityWorld struct {
	players map[uint32]*UnityWorldPlayer
	lock    sync.RWMutex
}

// UnityWorldPlayer 是服务端维护的 Unity 玩家状态。
type UnityWorldPlayer struct {
	PID        int32
	ConnID     uint32
	Position   UnityPosition
	ActionData int32
	Connection gface.IConnection
}

// NewUnityWorld 创建一个 Unity 客户端使用的在线玩家世界。
func NewUnityWorld() *UnityWorld {
	return &UnityWorld{
		players: make(map[uint32]*UnityWorldPlayer),
	}
}

// AddPlayer 添加一个在线玩家，并返回添加前的玩家快照。
func (w *UnityWorld) AddPlayer(connection gface.IConnection) (*UnityWorldPlayer, []UnityWorldPlayer, error) {
	if w == nil || connection == nil {
		return nil, nil, errors.New("unity world connection is nil")
	}

	w.lock.Lock()
	defer w.lock.Unlock()

	connID := connection.GetConnId()
	if _, ok := w.players[connID]; ok {
		return nil, nil, errors.New("unity world player already exists")
	}

	player := &UnityWorldPlayer{
		PID:        int32(connID),
		ConnID:     connID,
		Connection: connection,
	}
	players := make([]UnityWorldPlayer, 0, len(w.players))
	for _, oldPlayer := range w.players {
		players = append(players, *oldPlayer)
	}
	sort.Slice(players, func(i, j int) bool {
		return players[i].PID < players[j].PID
	})
	w.players[connID] = player
	return playerCopy(player), players, nil
}

// RemovePlayer 删除一个在线玩家，并返回被删除的玩家。
func (w *UnityWorld) RemovePlayer(connID uint32) (*UnityWorldPlayer, error) {
	if w == nil {
		return nil, errors.New("unity world is nil")
	}

	w.lock.Lock()
	defer w.lock.Unlock()

	player, ok := w.players[connID]
	if !ok {
		return nil, errors.New("unity world player not found")
	}
	delete(w.players, connID)
	return playerCopy(player), nil
}

// GetPlayer 获取一个在线玩家的快照。
func (w *UnityWorld) GetPlayer(connID uint32) (*UnityWorldPlayer, error) {
	if w == nil {
		return nil, errors.New("unity world is nil")
	}

	w.lock.RLock()
	defer w.lock.RUnlock()

	player, ok := w.players[connID]
	if !ok {
		return nil, errors.New("unity world player not found")
	}
	return playerCopy(player), nil
}

// UpdatePlayer 更新玩家坐标和动作数据，并返回更新后的快照。
func (w *UnityWorld) UpdatePlayer(connID uint32, position UnityPosition, actionData int32) (*UnityWorldPlayer, error) {
	if w == nil {
		return nil, errors.New("unity world is nil")
	}

	w.lock.Lock()
	defer w.lock.Unlock()

	player, ok := w.players[connID]
	if !ok {
		return nil, errors.New("unity world player not found")
	}
	player.Position = position
	player.ActionData = actionData
	return playerCopy(player), nil
}

// Players 返回当前在线玩家快照，结果按 PID 排序。
func (w *UnityWorld) Players() []UnityWorldPlayer {
	if w == nil {
		return nil
	}

	w.lock.RLock()
	defer w.lock.RUnlock()

	players := make([]UnityWorldPlayer, 0, len(w.players))
	for _, player := range w.players {
		players = append(players, *player)
	}
	sort.Slice(players, func(i, j int) bool {
		return players[i].PID < players[j].PID
	})
	return players
}

// Broadcast 向当前所有在线玩家发送 Unity 消息。
func (w *UnityWorld) Broadcast(msgID uint32, data []byte) []error {
	return w.broadcast(msgID, data, 0, false)
}

// BroadcastExcept 向除指定连接以外的其他在线玩家发送 Unity 消息。
func (w *UnityWorld) BroadcastExcept(exceptConnID uint32, msgID uint32, data []byte) []error {
	return w.broadcast(msgID, data, exceptConnID, true)
}

func (w *UnityWorld) broadcast(msgID uint32, data []byte, exceptConnID uint32, exclude bool) []error {
	players := w.Players()
	errorsFound := make([]error, 0)
	for _, player := range players {
		if exclude && player.ConnID == exceptConnID {
			continue
		}
		if err := player.Connection.SendBuffMsg(msgID, data); err != nil {
			errorsFound = append(errorsFound, err)
		}
	}
	return errorsFound
}

func playerCopy(player *UnityWorldPlayer) *UnityWorldPlayer {
	copyPlayer := *player
	return &copyPlayer
}
