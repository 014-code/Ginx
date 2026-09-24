package unity

import (
	"Ginx/gcore"
	"Ginx/gface"
	"errors"
	"sort"
	"sync"
)

const (
	defaultWorldWidth  = 1000
	defaultWorldHeight = 1000
	defaultCellWidth   = 10
	defaultCellHeight  = 10
)

// UnityWorld 保存 Unity MMO 客户端当前在线的玩家状态和 AOI 关系。
type UnityWorld struct {
	players   map[uint32]*UnityWorldPlayer
	playerIDs map[int32]uint32
	aoi       *gcore.AOIManager
	lock      sync.RWMutex
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
	world, err := NewUnityWorldWithAOI(defaultWorldWidth, defaultWorldHeight, defaultCellWidth, defaultCellHeight)
	if err != nil {
		panic(err)
	}
	return world
}

// NewUnityWorldWithAOI 创建一个使用指定世界范围和 AOI 网格的 Unity 世界。
func NewUnityWorldWithAOI(worldWidth, worldHeight, cellWidth, cellHeight float64) (*UnityWorld, error) {
	aoi, err := gcore.NewAOIManager(worldWidth, worldHeight, cellWidth, cellHeight)
	if err != nil {
		return nil, err
	}
	return &UnityWorld{
		players:   make(map[uint32]*UnityWorldPlayer),
		playerIDs: make(map[int32]uint32),
		aoi:       aoi,
	}, nil
}

// AddPlayer 添加一个在线玩家，并返回添加前的玩家快照。
func (w *UnityWorld) AddPlayer(connection gface.IConnection) (*UnityWorldPlayer, []UnityWorldPlayer, error) {
	return w.AddPlayerWithPID(connection, int32(connection.GetConnId()))
}

// AddPlayerWithPID 添加一个指定 PID 的在线玩家，并返回加入前的附近玩家快照。
func (w *UnityWorld) AddPlayerWithPID(connection gface.IConnection, pid int32) (*UnityWorldPlayer, []UnityWorldPlayer, error) {
	if w == nil || connection == nil {
		return nil, nil, errors.New("unity world connection is nil")
	}

	w.lock.Lock()
	defer w.lock.Unlock()

	connID := connection.GetConnId()
	if _, ok := w.players[connID]; ok {
		return nil, nil, errors.New("unity world player already exists")
	}
	if _, ok := w.playerIDs[pid]; ok {
		return nil, nil, errors.New("unity world player id already exists")
	}

	player := &UnityWorldPlayer{
		PID:        pid,
		ConnID:     connID,
		Connection: connection,
	}
	if err := w.aoi.AddEntity(connID, unityAOIPosition(player.Position)); err != nil {
		return nil, nil, err
	}
	w.players[connID] = player
	w.playerIDs[pid] = connID
	return playerCopy(player), w.nearbyPlayersLocked(connID), nil
}

// RemovePlayer 删除一个在线玩家，并返回被删除的玩家。
func (w *UnityWorld) RemovePlayer(connID uint32) (*UnityWorldPlayer, error) {
	player, _, err := w.RemovePlayerWithVisibility(connID)
	return player, err
}

// RemovePlayerWithVisibility 删除玩家，并返回删除前的附近玩家快照。
func (w *UnityWorld) RemovePlayerWithVisibility(connID uint32) (*UnityWorldPlayer, []UnityWorldPlayer, error) {
	if w == nil {
		return nil, nil, errors.New("unity world is nil")
	}

	w.lock.Lock()
	defer w.lock.Unlock()

	player, ok := w.players[connID]
	if !ok {
		return nil, nil, errors.New("unity world player not found")
	}
	nearbyPlayers := w.nearbyPlayersLocked(connID)
	if err := w.aoi.RemoveEntity(connID); err != nil {
		return nil, nil, err
	}
	delete(w.players, connID)
	delete(w.playerIDs, player.PID)
	return playerCopy(player), nearbyPlayers, nil
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
	result, err := w.UpdatePlayerWithVisibility(connID, position, actionData)
	if err != nil {
		return nil, err
	}
	return result.Player, nil
}

// UnityWorldMoveResult 保存玩家移动后的可见范围变化。
type UnityWorldMoveResult struct {
	Player  *UnityWorldPlayer
	Entered []UnityWorldPlayer
	Left    []UnityWorldPlayer
	Visible []UnityWorldPlayer
}

// UpdatePlayerWithVisibility 更新玩家状态，并返回进入、离开和当前可见玩家。
func (w *UnityWorld) UpdatePlayerWithVisibility(connID uint32, position UnityPosition, actionData int32) (*UnityWorldMoveResult, error) {
	if w == nil {
		return nil, errors.New("unity world is nil")
	}

	w.lock.Lock()
	defer w.lock.Unlock()

	player, ok := w.players[connID]
	if !ok {
		return nil, errors.New("unity world player not found")
	}
	change, err := w.aoi.MoveEntity(connID, unityAOIPosition(position))
	if err != nil {
		return nil, err
	}
	player.Position = position
	player.ActionData = actionData
	return &UnityWorldMoveResult{
		Player:  playerCopy(player),
		Entered: w.playersFromIDsLocked(change.Entered),
		Left:    w.playersFromIDsLocked(change.Left),
		Visible: w.nearbyPlayersLocked(connID),
	}, nil
}

// NearbyPlayers 返回指定连接附近的玩家，不包含玩家自己。
func (w *UnityWorld) NearbyPlayers(connID uint32) []UnityWorldPlayer {
	if w == nil {
		return nil
	}
	w.lock.RLock()
	defer w.lock.RUnlock()
	return w.nearbyPlayersLocked(connID)
}

// BroadcastVisible 向指定玩家当前视野内的连接发送消息。
func (w *UnityWorld) BroadcastVisible(connID uint32, msgID uint32, data []byte, includeSelf bool) []error {
	if w == nil {
		return nil
	}
	w.lock.RLock()
	players := w.nearbyPlayersLocked(connID)
	if includeSelf {
		if player, ok := w.players[connID]; ok {
			players = append(players, *playerCopy(player))
		}
	}
	w.lock.RUnlock()

	errorsFound := make([]error, 0)
	for _, player := range players {
		if err := player.Connection.SendBuffMsg(msgID, data); err != nil {
			errorsFound = append(errorsFound, err)
		}
	}
	return errorsFound
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

func unityAOIPosition(position UnityPosition) gcore.Position {
	return gcore.Position{X: float64(position.X), Y: float64(position.Z)}
}

func (w *UnityWorld) nearbyPlayersLocked(connID uint32) []UnityWorldPlayer {
	nearbyIDs, err := w.aoi.GetNearbyEntityIDs(connID)
	if err != nil {
		return nil
	}
	return w.playersFromIDsLocked(nearbyIDs)
}

func (w *UnityWorld) playersFromIDsLocked(playerIDs []uint32) []UnityWorldPlayer {
	players := make([]UnityWorldPlayer, 0, len(playerIDs))
	for _, playerID := range playerIDs {
		if player, ok := w.players[playerID]; ok {
			players = append(players, *playerCopy(player))
		}
	}
	sort.Slice(players, func(i, j int) bool {
		return players[i].PID < players[j].PID
	})
	return players
}
