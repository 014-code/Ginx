// Package persist 是可选的玩家存档组件，不属于网络核心。
// 其 Player 模型仅适用于采用此存档约定的应用；gnet 不依赖该接口。
package persist

import (
	"context"
	"errors"
	"time"
)

// ErrPlayerNotFound 表示存储中没有指定玩家。
var ErrPlayerNotFound = errors.New("player not found")

// Player 是框架提供的最小玩家存档结构。
// 业务层可以把扩展字段编码到 Data 中，避免持久化包依赖具体游戏模型。
type Player struct {
	PlayerID  uint64    `json:"player_id"`
	AccountID string    `json:"account_id"`
	Data      []byte    `json:"data"`
	UpdatedAt time.Time `json:"updated_at"`
}

// PlayerStore 是玩家存档的最小接口。
type PlayerStore interface {
	Load(ctx context.Context, playerID uint64) (*Player, error)
	Save(ctx context.Context, player *Player) error
	Delete(ctx context.Context, playerID uint64) error
}
