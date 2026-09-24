package server

import (
	"Ginx/gcore"
	"Ginx/gface"
	"Ginx/gnet"
	"Ginx/session"
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"math"
	"sort"
	"strings"
	"sync"
	"time"
	"unicode"
)

type Guest struct {
	Token     string    `json:"token"`
	Name      string    `json:"name"`
	ExpiresAt time.Time `json:"expires_at"`
}
type player struct {
	PlayerView
	conn        gface.IConnection
	roomID      uint32
	dx, dy      float64
	inputAt     time.Time
	connectedAt time.Time
}
type arena struct {
	room     *gcore.Room
	aoi      *gcore.AOIManager
	crystals []Crystal
	respawn  map[int]time.Time
}

type World struct {
	mu        sync.Mutex
	sessions  *session.Manager
	names     map[uint64]string
	nextGuest uint64
	players   map[uint32]*player
	arenas    map[uint32]*arena
	tick      uint64
	closed    bool
	running   bool
}

func NewWorld() *World {
	sessions, _ := session.New(session.Options{})
	w := &World{sessions: sessions, names: map[uint64]string{}, players: map[uint32]*player{}, arenas: map[uint32]*arena{}}
	rooms := gcore.NewRoomManager()
	for id := uint32(1); id <= 2; id++ {
		room, _ := rooms.CreateRoom(id, 8)
		_ = room.Start()
		aoi, _ := gcore.NewAOIManager(WorldWidth, WorldHeight, 160, 135)
		w.arenas[id] = &arena{room: room, aoi: aoi, respawn: map[int]time.Time{}, crystals: []Crystal{
			{1, 250, 270}, {2, 410, 150}, {3, 480, 390}, {4, 640, 270}, {5, 780, 130}, {6, 810, 420}, {7, 150, 100}, {8, 140, 430},
		}}
	}
	return w
}

// Guest 只签发临时演示身份；无账号口令、无持久化，不用于生产认证。
func (w *World) Guest(name string) (*Guest, error) {
	name = strings.TrimSpace(name)
	if len([]rune(name)) < 1 || len([]rune(name)) > 16 {
		return nil, errors.New("name must contain 1..16 characters")
	}
	for _, r := range name {
		if unicode.IsControl(r) || unicode.Is(unicode.Cf, r) {
			return nil, errors.New("invalid name")
		}
	}
	w.mu.Lock()
	defer w.mu.Unlock()
	if w.closed || w.sessions.Len() >= 256 {
		return nil, errors.New("guest capacity reached")
	}
	w.nextGuest++
	value, _, err := w.sessions.Issue(fmt.Sprintf("webguest-%d", w.nextGuest), w.nextGuest, 30*time.Minute)
	if err != nil {
		return nil, err
	}
	w.names[value.PlayerID] = name
	return &Guest{value.Token, name, value.ExpiresAt}, nil
}

func (w *World) Register(tcp *gnet.Server) {
	tcp.SetOnConnStart(func(c gface.IConnection) {
		w.mu.Lock()
		defer w.mu.Unlock()
		if w.closed {
			c.Stop()
			return
		}
		w.players[c.GetConnId()] = &player{PlayerView: PlayerView{ID: c.GetConnId()}, conn: c, connectedAt: time.Now()}
	})
	tcp.SetOnConnStop(func(c gface.IConnection) {
		w.mu.Lock()
		defer w.mu.Unlock()
		if p := w.players[c.GetConnId()]; p != nil {
			w.leave(p)
			delete(w.players, p.ID)
		}
		if s, err := w.sessions.LogoutByConnID(c.GetConnId()); err == nil {
			delete(w.names, s.PlayerID)
		}
	})
	router := &worldRouter{world: w}
	for _, id := range []uint32{MsgAuthenticate, MsgJoin, MsgInput, MsgLeave, MsgHeartbeat} {
		tcp.AddRouter(id, router)
	}
}

type worldRouter struct {
	gnet.BaseRouter
	world *World
}

func (r *worldRouter) Handle(request gface.IRequest) { r.world.handle(request) }
func decode(data []byte, target any) bool {
	data = bytes.TrimSpace(data)
	if len(data) == 0 || data[0] != '{' {
		return false
	}
	d := json.NewDecoder(bytes.NewReader(data))
	d.DisallowUnknownFields()
	return d.Decode(target) == nil && d.Decode(new(any)) == io.EOF
}
func send(c gface.IConnection, id uint32, value any) {
	data, err := json.Marshal(value)
	if err != nil || c.SendBuffMsg(id, data) != nil {
		c.Stop()
	}
}
func (w *World) fail(p *player, request uint32, code string) {
	send(p.conn, MsgError, map[string]any{"request_id": request, "code": code})
}

func (w *World) handle(request gface.IRequest) {
	w.mu.Lock()
	defer w.mu.Unlock()
	p := w.players[request.GetConnection().GetConnId()]
	if p == nil || w.closed {
		return
	}
	id := request.GetMsgID()
	if id == MsgAuthenticate {
		var body struct {
			Token string `json:"token"`
		}
		if !decode(request.GetData(), &body) || len(body.Token) != 48 {
			w.fail(p, id, "invalid_request")
			return
		}
		s, err := w.sessions.Bind(body.Token, p.ID)
		if err != nil {
			w.fail(p, id, "unauthorized")
			return
		}
		p.Name = w.names[s.PlayerID]
		send(p.conn, id, map[string]any{"id": p.ID, "name": p.Name})
		return
	}
	if _, err := w.sessions.GetByConnID(p.ID); err != nil {
		w.fail(p, id, "unauthorized")
		return
	}
	switch id {
	case MsgJoin:
		var body struct {
			RoomID uint32 `json:"room_id"`
		}
		if !decode(request.GetData(), &body) {
			w.fail(p, id, "invalid_request")
			return
		}
		a := w.arenas[body.RoomID]
		if a == nil {
			w.fail(p, id, "room_not_found")
			return
		}
		if p.roomID != body.RoomID {
			if err := a.room.Join(p.conn); err != nil {
				w.fail(p, id, "room_full")
				return
			}
			w.leave(p)
			p.roomID = body.RoomID
			p.X = 130
			p.Y = 270
			p.Score = 0
			p.dx = 0
			p.dy = 0
			_ = a.aoi.AddEntity(p.ID, gcore.Position{X: p.X, Y: p.Y})
			w.event(a, "join", p.Name+" 加入竞技场")
		}
		send(p.conn, id, map[string]any{"room_id": p.roomID})
		w.broadcast(a)
	case MsgInput:
		var body struct {
			DX  float64 `json:"dx"`
			DY  float64 `json:"dy"`
			Seq uint32  `json:"seq"`
		}
		if !decode(request.GetData(), &body) || math.IsNaN(body.DX) || math.IsNaN(body.DY) || math.Abs(body.DX) > 1 || math.Abs(body.DY) > 1 || body.Seq == 0 {
			w.fail(p, id, "invalid_input")
			return
		}
		if p.roomID == 0 {
			w.fail(p, id, "not_in_room")
			return
		}
		if body.Seq <= p.Seq {
			return
		}
		length := math.Hypot(body.DX, body.DY)
		if length > 1 {
			body.DX /= length
			body.DY /= length
		}
		p.dx = body.DX
		p.dy = body.DY
		p.Seq = body.Seq
		p.inputAt = time.Now()
	case MsgLeave:
		if !decode(request.GetData(), &struct{}{}) {
			w.fail(p, id, "invalid_request")
			return
		}
		w.leave(p)
		send(p.conn, id, map[string]bool{"ok": true})
	case MsgHeartbeat:
		var body struct {
			SentAt float64 `json:"sent_at"`
		}
		if !decode(request.GetData(), &body) {
			w.fail(p, id, "invalid_request")
			return
		}
		send(p.conn, id, body)
	}
}

func (w *World) leave(p *player) {
	if a := w.arenas[p.roomID]; a != nil {
		_ = a.room.Leave(p.ID)
		_ = a.aoi.RemoveEntity(p.ID)
		w.event(a, "leave", p.Name+" 离开竞技场")
	}
	p.roomID = 0
	p.dx = 0
	p.dy = 0
}
func (w *World) event(a *arena, kind, message string) {
	w.broadcastData(a, MsgEvent, map[string]string{"kind": kind, "message": message})
}
func (w *World) broadcastData(a *arena, id uint32, value any) {
	data, err := json.Marshal(value)
	if err != nil {
		return
	}
	for _, e := range a.room.Broadcast(id, data) {
		if p := w.players[e.ConnID]; p != nil {
			p.conn.Stop()
		}
	}
}
func (w *World) broadcast(a *arena) {
	s := Snapshot{Tick: w.tick, RoomID: a.room.ID, Players: []PlayerView{}, Crystals: []Crystal{}}
	for _, c := range a.crystals {
		if _, hidden := a.respawn[c.ID]; !hidden {
			s.Crystals = append(s.Crystals, c)
		}
	}
	for _, c := range a.room.Members() {
		if p := w.players[c.GetConnId()]; p != nil {
			v := p.PlayerView
			nearby, _ := a.aoi.GetNearbyEntityIDs(p.ID)
			v.Nearby = len(nearby)
			s.Players = append(s.Players, v)
		}
	}
	w.broadcastData(a, MsgSnapshot, s)
}

// Run 由应用显式启动；固定步进限制移速，输入停止 250ms 后角色自动停下。
func (w *World) Run(ctx context.Context) {
	w.mu.Lock()
	if w.running || w.closed {
		w.mu.Unlock()
		return
	}
	w.running = true
	w.mu.Unlock()
	defer func() { w.mu.Lock(); w.running = false; w.mu.Unlock() }()
	ticker := time.NewTicker(time.Second / TickRate)
	defer ticker.Stop()
	for {
		select {
		case <-ctx.Done():
			return
		case now := <-ticker.C:
			w.step(now)
		}
	}
}
func (w *World) step(now time.Time) {
	w.mu.Lock()
	defer w.mu.Unlock()
	if w.closed {
		return
	}
	w.tick++
	for _, s := range w.sessions.RemoveExpired(now) {
		delete(w.names, s.PlayerID)
		if s.Bound {
			if p := w.players[s.ConnID]; p != nil {
				w.leave(p)
				p.conn.Stop()
			}
		}
	}
	ids := make([]uint32, 0, len(w.players))
	for id := range w.players {
		ids = append(ids, id)
	}
	sort.Slice(ids, func(i, j int) bool { return ids[i] < ids[j] })
	// 轮换同 tick 的采集顺序，避免固定连接 ID 总是优先。
	for offset := 0; offset < len(ids); offset++ {
		p := w.players[ids[(offset+int(w.tick%uint64(len(ids))))%len(ids)]]
		if p.Name == "" && now.Sub(p.connectedAt) > 10*time.Second {
			p.conn.Stop()
			continue
		}
		a := w.arenas[p.roomID]
		if a == nil {
			continue
		}
		if now.Sub(p.inputAt) <= 250*time.Millisecond {
			p.X = math.Max(24, math.Min(WorldWidth-24, p.X+p.dx*180/TickRate))
			p.Y = math.Max(24, math.Min(WorldHeight-24, p.Y+p.dy*180/TickRate))
			_, _ = a.aoi.MoveEntity(p.ID, gcore.Position{X: p.X, Y: p.Y})
		}
		for _, c := range a.crystals {
			if _, hidden := a.respawn[c.ID]; hidden {
				continue
			}
			if math.Hypot(p.X-c.X, p.Y-c.Y) <= 25 {
				p.Score++
				a.respawn[c.ID] = now.Add(5 * time.Second)
				w.event(a, "collect", p.Name+" 采集水晶 +1")
			}
		}
	}
	for _, a := range w.arenas {
		for id, at := range a.respawn {
			if !now.Before(at) {
				delete(a.respawn, id)
			}
		}
		if w.tick%2 == 0 && a.room.PlayerCount() > 0 {
			w.broadcast(a)
		}
	}
}
func (w *World) Close() {
	w.mu.Lock()
	defer w.mu.Unlock()
	w.closed = true
	for _, p := range w.players {
		p.conn.Stop()
	}
}
