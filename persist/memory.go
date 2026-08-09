package persist

import (
	"context"
	"errors"
	"sync"
)

// MemoryStore 是用于测试和单进程原型的内存玩家存储。
type MemoryStore struct {
	lock    sync.RWMutex
	players map[uint64]*Player
}

// NewMemoryStore 创建内存玩家存储。
func NewMemoryStore() *MemoryStore {
	return &MemoryStore{players: make(map[uint64]*Player)}
}

// Load 读取玩家存档快照。
func (s *MemoryStore) Load(ctx context.Context, playerID uint64) (*Player, error) {
	if err := contextError(ctx); err != nil {
		return nil, err
	}
	s.lock.RLock()
	defer s.lock.RUnlock()
	player, ok := s.players[playerID]
	if !ok {
		return nil, ErrPlayerNotFound
	}
	return playerCopy(player), nil
}

// Save 保存玩家存档快照。
func (s *MemoryStore) Save(ctx context.Context, player *Player) error {
	if err := contextError(ctx); err != nil {
		return err
	}
	if player == nil || player.PlayerID == 0 {
		return errors.New("player is invalid")
	}
	s.lock.Lock()
	defer s.lock.Unlock()
	s.players[player.PlayerID] = playerCopy(player)
	return nil
}

// Delete 删除玩家存档。
func (s *MemoryStore) Delete(ctx context.Context, playerID uint64) error {
	if err := contextError(ctx); err != nil {
		return err
	}
	s.lock.Lock()
	defer s.lock.Unlock()
	if _, ok := s.players[playerID]; !ok {
		return ErrPlayerNotFound
	}
	delete(s.players, playerID)
	return nil
}

func contextError(ctx context.Context) error {
	if ctx == nil {
		return nil
	}
	select {
	case <-ctx.Done():
		return ctx.Err()
	default:
		return nil
	}
}

func playerCopy(player *Player) *Player {
	if player == nil {
		return nil
	}
	copyPlayer := *player
	copyPlayer.Data = append([]byte(nil), player.Data...)
	return &copyPlayer
}
