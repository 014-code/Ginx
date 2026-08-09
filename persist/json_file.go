package persist

import (
	"context"
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
	"sync"
)

// JSONFileStore 是适合开发环境和小规模服务的 JSON 文件存储。
// 正式生产环境建议实现 PlayerStore 接入数据库或专用存储服务。
type JSONFileStore struct {
	lock sync.Mutex
	path string
}

// NewJSONFileStore 创建 JSON 文件存储。
func NewJSONFileStore(path string) (*JSONFileStore, error) {
	if path == "" {
		return nil, errors.New("player store path is empty")
	}
	return &JSONFileStore{path: filepath.Clean(path)}, nil
}

// Load 读取玩家存档。
func (s *JSONFileStore) Load(ctx context.Context, playerID uint64) (*Player, error) {
	if err := contextError(ctx); err != nil {
		return nil, err
	}
	s.lock.Lock()
	defer s.lock.Unlock()
	players, err := s.readLocked()
	if err != nil {
		return nil, err
	}
	player, ok := players[playerID]
	if !ok {
		return nil, ErrPlayerNotFound
	}
	return playerCopy(player), nil
}

// Save 写入玩家存档。
func (s *JSONFileStore) Save(ctx context.Context, player *Player) error {
	if err := contextError(ctx); err != nil {
		return err
	}
	if player == nil || player.PlayerID == 0 {
		return errors.New("player is invalid")
	}
	s.lock.Lock()
	defer s.lock.Unlock()
	players, err := s.readLocked()
	if err != nil {
		return err
	}
	players[player.PlayerID] = playerCopy(player)
	return s.writeLocked(players)
}

// Delete 删除玩家存档。
func (s *JSONFileStore) Delete(ctx context.Context, playerID uint64) error {
	if err := contextError(ctx); err != nil {
		return err
	}
	s.lock.Lock()
	defer s.lock.Unlock()
	players, err := s.readLocked()
	if err != nil {
		return err
	}
	if _, ok := players[playerID]; !ok {
		return ErrPlayerNotFound
	}
	delete(players, playerID)
	return s.writeLocked(players)
}

func (s *JSONFileStore) readLocked() (map[uint64]*Player, error) {
	data, err := os.ReadFile(s.path)
	if errors.Is(err, os.ErrNotExist) {
		return make(map[uint64]*Player), nil
	}
	if err != nil {
		return nil, err
	}
	players := make(map[uint64]*Player)
	if len(data) == 0 {
		return players, nil
	}
	if err := json.Unmarshal(data, &players); err != nil {
		return nil, err
	}
	return players, nil
}

func (s *JSONFileStore) writeLocked(players map[uint64]*Player) error {
	data, err := json.MarshalIndent(players, "", "  ")
	if err != nil {
		return err
	}
	if err := os.MkdirAll(filepath.Dir(s.path), 0755); err != nil {
		return err
	}
	temporary, err := os.CreateTemp(filepath.Dir(s.path), ".ginx-player-*.tmp")
	if err != nil {
		return err
	}
	temporaryName := temporary.Name()
	defer os.Remove(temporaryName)
	if _, err := temporary.Write(data); err != nil {
		temporary.Close()
		return err
	}
	if err := temporary.Close(); err != nil {
		return err
	}
	if err := os.Remove(s.path); err != nil && !errors.Is(err, os.ErrNotExist) {
		return err
	}
	return os.Rename(temporaryName, s.path)
}
