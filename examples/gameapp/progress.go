package gameapp

import (
	"Ginx/persist"
	"context"
	"encoding/json"
	"errors"
	"time"
)

var (
	ErrNotInRoom       = errors.New("not_in_room")
	ErrRewardClaimed   = errors.New("reward_already_claimed")
	ErrItemUnavailable = errors.New("item_unavailable")
)

// Progress 是本示例的业务存档，等级规则和背包不进入框架层。
type Progress struct {
	Version        int            `json:"version"`
	Level          int            `json:"level"`
	Experience     int            `json:"experience"`
	Inventory      map[string]int `json:"inventory"`
	StarterClaimed bool           `json:"starter_claimed"`
}

// 示例扩展接口：原子读取、修改、保存。框架的 PlayerStore 不需要变动。
type transactionalStore interface {
	Update(context.Context, uint64, func(*persist.Player) error) error
}

func decodeProgress(data []byte) (*Progress, error) {
	if len(data) == 0 {
		return &Progress{Version: 1, Level: 1, Inventory: map[string]int{}}, nil
	}
	p := new(Progress)
	if err := json.Unmarshal(data, p); err != nil {
		return nil, err
	}
	if p.Version != 1 || p.Experience < 0 || p.Experience > 100 || p.Level != 1+p.Experience/100 || p.Inventory == nil {
		return nil, errors.New("invalid example progress")
	}
	for item, count := range p.Inventory {
		if item != "potion" || count < 0 || count > 2 {
			return nil, errors.New("invalid example inventory")
		}
	}
	if (!p.StarterClaimed && (p.Experience != 0 || len(p.Inventory) != 0)) || (p.StarterClaimed && p.Experience != 100) {
		return nil, errors.New("invalid starter reward state")
	}
	return p, nil
}

func (s *Service) Progress(ctx context.Context, token string) (*Progress, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	value, err := s.sessionLocked(token)
	if err != nil {
		return nil, err
	}
	return s.loadProgress(ctx, value.PlayerID)
}

func (s *Service) loadProgress(ctx context.Context, playerID uint64) (*Progress, error) {
	player, err := s.store.Load(ctx, playerID)
	if err != nil {
		return nil, err
	}
	return decodeProgress(player.Data)
}

func (s *Service) ConnectionProgress(ctx context.Context, connID uint32) (*Progress, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	if _, err := s.authenticatedLocked(connID); err != nil {
		return nil, err
	}
	value, err := s.sessions.GetByConnID(connID)
	if err != nil {
		return nil, ErrUnauthorized
	}
	return s.loadProgress(ctx, value.PlayerID)
}

// ClaimStarter 演示一次性奖励：数额由服务端定义，不接受客户端传经验或数量。
func (s *Service) ClaimStarter(ctx context.Context, connID uint32) (*Progress, error) {
	return s.changeProgress(ctx, connID, func(p *Progress) error {
		if p.StarterClaimed {
			return ErrRewardClaimed
		}
		p.StarterClaimed = true
		p.Experience += 100
		p.Level = 1 + p.Experience/100
		p.Inventory["potion"] += 2
		return nil
	})
}

// UseItem 仅演示事务扣减；没有战斗系统，也不宣称实现回血效果。
func (s *Service) UseItem(ctx context.Context, connID uint32, itemID string) (*Progress, error) {
	return s.changeProgress(ctx, connID, func(p *Progress) error {
		if itemID != "potion" || p.Inventory[itemID] <= 0 {
			return ErrItemUnavailable
		}
		p.Inventory[itemID]--
		return nil
	})
}

func (s *Service) changeProgress(ctx context.Context, connID uint32, change func(*Progress) error) (*Progress, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	conn, err := s.authenticatedLocked(connID)
	if err != nil {
		return nil, err
	}
	if conn.roomID == 0 {
		return nil, ErrNotInRoom
	}
	value, err := s.sessions.GetByConnID(connID)
	if err != nil {
		return nil, ErrUnauthorized
	}
	store, ok := s.store.(transactionalStore)
	if !ok {
		return nil, errors.New("progress changes require a transactional example store")
	}
	ctx, cancel := context.WithTimeout(ctx, 3*time.Second)
	defer cancel()
	var result *Progress
	err = store.Update(ctx, value.PlayerID, func(player *persist.Player) error {
		if player.AccountID != value.AccountID {
			return ErrUnauthorized
		}
		p, err := decodeProgress(player.Data)
		if err != nil {
			return err
		}
		if err := change(p); err != nil {
			return err
		}
		player.Data, err = json.Marshal(p)
		result = p
		return err
	})
	if err != nil {
		return nil, err
	}
	return result, nil // 仅在事务提交成功后回包，不保留可能与数据库不一致的内存副本。
}
