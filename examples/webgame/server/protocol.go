// Package server 是 Web 游戏 Demo 的业务实现，不属于框架 API。
package server

const (
	MsgAuthenticate uint32 = 4101
	MsgJoin         uint32 = 4102
	MsgInput        uint32 = 4103
	MsgLeave        uint32 = 4104
	MsgHeartbeat    uint32 = 4105
	MsgSnapshot     uint32 = 4201
	MsgError        uint32 = 4202
	MsgEvent        uint32 = 4203
	WorldWidth             = 960
	WorldHeight            = 540
	TickRate               = 20
	MaxPacketSize          = 64 << 10
)

type PlayerView struct {
	ID     uint32  `json:"id"`
	Name   string  `json:"name"`
	X      float64 `json:"x"`
	Y      float64 `json:"y"`
	Score  int     `json:"score"`
	Seq    uint32  `json:"seq"`
	Nearby int     `json:"nearby"`
}
type Crystal struct {
	ID int     `json:"id"`
	X  float64 `json:"x"`
	Y  float64 `json:"y"`
}
type Snapshot struct {
	Tick     uint64       `json:"tick"`
	RoomID   uint32       `json:"room_id"`
	Players  []PlayerView `json:"players"`
	Crystals []Crystal    `json:"crystals"`
}
