// Package gameapp 是可修改的游戏业务示例，不属于 Ginx 框架 API。
package gameapp

import (
	"Ginx/gcore"
	"Ginx/gface"
	"Ginx/persist"
	"Ginx/session"
	"context"
	"errors"
	"fmt"
	"sort"
	"sync"
	"time"

	"golang.org/x/crypto/bcrypt"
)

var (
	ErrUnauthorized    = errors.New("unauthorized")
	ErrRoomNotFound    = errors.New("room_not_found")
	ErrRoomUnavailable = errors.New("room_unavailable")
)

// Account 由服务端配置，客户端不能指定 PlayerID 或密码哈希。
type Account struct {
	AccountID    string `json:"account_id"`
	PlayerID     uint64 `json:"player_id"`
	PasswordHash string `json:"password_hash"`
}

type RoomConfig struct {
	ID         uint32 `json:"id"`
	MaxPlayers int    `json:"max_players"`
}

type LoginResult struct {
	Token     string    `json:"token"`
	PlayerID  uint64    `json:"player_id"`
	ExpiresAt time.Time `json:"expires_at"`
}

type PlayerView struct {
	Player *persist.Player `json:"player"`
	Online bool            `json:"online"`
	RoomID uint32          `json:"room_id"`
}

type RoomView struct {
	ID         uint32 `json:"id"`
	MaxPlayers int    `json:"max_players"`
	Players    int    `json:"players"`
	State      string `json:"state"`
}

type connectedPlayer struct {
	connection   gface.IConnection
	roomID       uint32
	authDeadline time.Time
}

// Service 的锁保证跨 HTTP 请求、不同 Worker 和断线 Hook 的复合操作一致。
// 构造函数只组装依赖，不监听端口、不启动后台协程。
type Service struct {
	mu          sync.Mutex
	accounts    map[string]Account
	dummyHash   string
	sessions    *session.Manager
	store       persist.PlayerStore
	rooms       *gcore.RoomManager
	roomIDs     []uint32
	connections map[uint32]*connectedPlayer
	ttl         time.Duration
	closed      bool
}

func New(accounts []Account, store persist.PlayerStore, ttl time.Duration, rooms []RoomConfig) (*Service, error) {
	if len(accounts) == 0 || store == nil || ttl <= 0 {
		return nil, errors.New("accounts, player store and positive token TTL are required")
	}
	sessions, err := session.New(session.Options{AccountPolicy: session.ReplaceExisting})
	if err != nil {
		return nil, err
	}
	s := &Service{accounts: make(map[string]Account), sessions: sessions, store: store,
		rooms: gcore.NewRoomManager(), connections: make(map[uint32]*connectedPlayer), ttl: ttl}
	players := make(map[uint64]bool)
	for _, account := range accounts {
		if account.AccountID == "" || len(account.AccountID) > 128 || account.PlayerID == 0 || players[account.PlayerID] {
			return nil, errors.New("invalid or duplicate account identity")
		}
		if _, ok := s.accounts[account.AccountID]; ok {
			return nil, errors.New("duplicate account")
		}
		cost, err := bcrypt.Cost([]byte(account.PasswordHash))
		if err != nil || cost < bcrypt.MinCost || cost > 14 {
			return nil, errors.New("invalid bcrypt hash or unsupported cost")
		}
		s.accounts[account.AccountID] = account
		players[account.PlayerID] = true
		s.dummyHash = account.PasswordHash
	}
	for _, room := range rooms {
		if room.ID == 0 || room.MaxPlayers <= 0 {
			return nil, errors.New("room ID and capacity must be positive")
		}
		if _, err := s.rooms.CreateRoom(room.ID, room.MaxPlayers); err != nil {
			return nil, err
		}
		s.roomIDs = append(s.roomIDs, room.ID)
	}
	sort.Slice(s.roomIDs, func(i, j int) bool { return s.roomIDs[i] < s.roomIDs[j] })
	return s, nil
}

func (s *Service) Login(ctx context.Context, accountID, password string) (*LoginResult, error) {
	account, found := s.accounts[accountID]
	hash := account.PasswordHash
	if !found {
		hash = s.dummyHash
	}
	// 密码校验不占用业务锁；不存在的账号也执行相同类型的校验。
	err := bcrypt.CompareHashAndPassword([]byte(hash), []byte(password))
	if err != nil || !found || len(password) == 0 || len(password) > 72 {
		return nil, ErrUnauthorized
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.closed {
		return nil, ErrUnauthorized
	}
	player, err := s.store.Load(ctx, account.PlayerID)
	if errors.Is(err, persist.ErrPlayerNotFound) {
		player = &persist.Player{PlayerID: account.PlayerID, AccountID: accountID, UpdatedAt: time.Now()}
		err = s.store.Save(ctx, player)
	}
	if err != nil {
		return nil, fmt.Errorf("load or create player: %w", err)
	}
	if player.AccountID != accountID {
		return nil, errors.New("stored player identity mismatch")
	}
	value, old, err := s.sessions.Issue(accountID, account.PlayerID, s.ttl)
	if err != nil {
		return nil, err
	}
	if old != nil && old.Bound {
		s.disconnectLocked(old.ConnID, true)
	}
	return &LoginResult{Token: value.Token, PlayerID: value.PlayerID, ExpiresAt: value.ExpiresAt}, nil
}

func (s *Service) Me(ctx context.Context, token string) (*PlayerView, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	value, err := s.sessionLocked(token)
	if err != nil {
		return nil, err
	}
	player, err := s.store.Load(ctx, value.PlayerID)
	if err != nil {
		return nil, err
	}
	view := &PlayerView{Player: player}
	if connection, ok := s.connections[value.ConnID]; value.Bound && ok {
		view.Online, view.RoomID = true, connection.roomID
	}
	return view, nil
}

func (s *Service) Rooms(token string) ([]RoomView, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	if _, err := s.sessionLocked(token); err != nil {
		return nil, err
	}
	views := make([]RoomView, 0, len(s.roomIDs))
	for _, id := range s.roomIDs {
		room, _ := s.rooms.GetRoom(id)
		views = append(views, roomView(room))
	}
	return views, nil
}

func (s *Service) sessionLocked(token string) (*session.Session, error) {
	if s.closed {
		return nil, ErrUnauthorized
	}
	value, err := s.sessions.GetByToken(token)
	if err != nil {
		return nil, ErrUnauthorized
	}
	return value, nil
}

// Connected 必须由初始化 Hook 在读取第一条 TCP 消息前调用。
func (s *Service) Connected(connection gface.IConnection) {
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.closed {
		connection.Stop()
		return
	}
	s.connections[connection.GetConnId()] = &connectedPlayer{connection: connection, authDeadline: time.Now().Add(30 * time.Second)}
}

func (s *Service) Disconnected(connection gface.IConnection) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.disconnectLocked(connection.GetConnId(), false)
}

func (s *Service) disconnectLocked(connID uint32, stop bool) {
	connection, ok := s.connections[connID]
	if !ok {
		return
	}
	if connection.roomID != 0 {
		if room, err := s.rooms.GetRoom(connection.roomID); err == nil {
			_ = room.Leave(connID)
		}
	}
	_, _ = s.sessions.LogoutByConnID(connID)
	delete(s.connections, connID)
	if stop {
		connection.connection.Stop()
	}
}

func (s *Service) Authenticate(connID uint32, token string) (uint64, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	connection, ok := s.connections[connID]
	if s.closed || !ok || (!connection.authDeadline.IsZero() && !time.Now().Before(connection.authDeadline)) {
		return 0, ErrUnauthorized
	}
	value, err := s.sessions.Bind(token, connID)
	if err != nil {
		return 0, ErrUnauthorized
	}
	connection.authDeadline = time.Time{}
	return value.PlayerID, nil
}

func (s *Service) authenticatedLocked(connID uint32) (*connectedPlayer, error) {
	connection, ok := s.connections[connID]
	if s.closed || !ok {
		return nil, ErrUnauthorized
	}
	if _, err := s.sessions.GetByConnID(connID); err != nil {
		return nil, ErrUnauthorized
	}
	return connection, nil
}

func (s *Service) Join(connID, roomID uint32) (*RoomView, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	connection, err := s.authenticatedLocked(connID)
	if err != nil {
		return nil, err
	}
	room, err := s.rooms.GetRoom(roomID)
	if err != nil {
		return nil, ErrRoomNotFound
	}
	if connection.roomID != roomID {
		if err := room.Join(connection.connection); err != nil {
			return nil, ErrRoomUnavailable
		}
		if old, err := s.rooms.GetRoom(connection.roomID); err == nil {
			_ = old.Leave(connID)
		}
		connection.roomID = roomID
	}
	view := roomView(room)
	return &view, nil
}

func (s *Service) Leave(connID uint32) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	connection, err := s.authenticatedLocked(connID)
	if err != nil {
		return err
	}
	if connection.roomID != 0 {
		room, _ := s.rooms.GetRoom(connection.roomID)
		_ = room.Leave(connID)
		connection.roomID = 0
	}
	return nil
}

func (s *Service) Heartbeat(connID uint32) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	_, err := s.authenticatedLocked(connID)
	return err
}

func roomView(room *gcore.Room) RoomView {
	return RoomView{ID: room.ID, MaxPlayers: room.MaxPlayers, Players: room.PlayerCount(), State: room.State().String()}
}

// Sweep 回收过期 Token 和未完成鉴权的连接，所有请求仍会独立校验有效期。
func (s *Service) Sweep(now time.Time) {
	s.mu.Lock()
	defer s.mu.Unlock()
	for _, value := range s.sessions.RemoveExpired(now) {
		if value.Bound {
			s.disconnectLocked(value.ConnID, true)
		}
	}
	for id, connection := range s.connections {
		if !connection.authDeadline.IsZero() && !now.Before(connection.authDeadline) {
			s.disconnectLocked(id, true)
		}
	}
}

func (s *Service) Close() {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.closed = true
	for id := range s.connections {
		s.disconnectLocked(id, true)
	}
}
