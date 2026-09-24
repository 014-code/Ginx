// Package session 提供与具体网络协议无关的内存会话管理。
package session

import (
	"crypto/rand"
	"encoding/hex"
	"errors"
	"sync"
	"time"
)

// Session 保存一次登录会话的短期状态。
type Session struct {
	Token     string
	AccountID string
	PlayerID  uint64
	ConnID    uint32
	Bound     bool
	ExpiresAt time.Time
	CreatedAt time.Time
	UpdatedAt time.Time
}

// Manager 管理账号、玩家、连接和 Token 之间的映射关系。
type Manager struct {
	legacy        *legacyState
	accountPolicy AccountPolicy
	lock          sync.RWMutex
	byToken       map[string]*Session
	byAccountID   map[string]string
	byPlayerID    map[uint64]string
	byConnID      map[uint32]string
}

// AccountPolicy 明确选择同账号已有会话时的处理方式。
type AccountPolicy uint8

const (
	RejectExisting AccountPolicy = iota
	ReplaceExisting
)

type Options struct{ AccountPolicy AccountPolicy }

var ErrAccountActive = errors.New("account already has an active session")

// New 创建可选的单账号单会话组件，不分配玩家编号、不读取配置。
// 多设备会话应由应用使用不同的会话模型，本组件不隐式开启多登录。
func New(options Options) (*Manager, error) {
	if options.AccountPolicy != RejectExisting && options.AccountPolicy != ReplaceExisting {
		return nil, errors.New("invalid account policy")
	}
	return &Manager{accountPolicy: options.AccountPolicy, byToken: make(map[string]*Session),
		byAccountID: make(map[string]string), byPlayerID: make(map[uint64]string), byConnID: make(map[uint32]string)}, nil
}

// GetByToken 根据 Token 获取会话快照。
func (m *Manager) GetByToken(token string) (*Session, error) {
	if m == nil {
		return nil, errors.New("session manager is nil")
	}
	m.lock.RLock()
	defer m.lock.RUnlock()
	session, ok := m.byToken[token]
	if !ok || expired(session, time.Now()) {
		return nil, errors.New("session not found")
	}
	return sessionCopy(session), nil
}

// GetByConnID 根据连接 ID 获取会话快照。
func (m *Manager) GetByConnID(connID uint32) (*Session, error) {
	if m == nil {
		return nil, errors.New("session manager is nil")
	}
	m.lock.RLock()
	defer m.lock.RUnlock()
	token, ok := m.byConnID[connID]
	if !ok {
		return nil, errors.New("session not found")
	}
	value := m.byToken[token]
	if expired(value, time.Now()) {
		return nil, errors.New("session expired")
	}
	return sessionCopy(value), nil
}

// GetByPlayerID 根据玩家 ID 获取会话快照。
func (m *Manager) GetByPlayerID(playerID uint64) (*Session, error) {
	if m == nil {
		return nil, errors.New("session manager is nil")
	}
	m.lock.RLock()
	defer m.lock.RUnlock()
	token, ok := m.byPlayerID[playerID]
	if !ok {
		return nil, errors.New("session not found")
	}
	value := m.byToken[token]
	if expired(value, time.Now()) {
		return nil, errors.New("session expired")
	}
	return sessionCopy(value), nil
}

// Touch 刷新会话最后活跃时间。
func (m *Manager) Touch(token string) error {
	if m == nil {
		return errors.New("session manager is nil")
	}
	m.lock.Lock()
	defer m.lock.Unlock()
	session, ok := m.byToken[token]
	if !ok || expired(session, time.Now()) {
		return errors.New("session not found")
	}
	session.UpdatedAt = time.Now()
	return nil
}

// LogoutByToken 删除指定 Token 的会话。
func (m *Manager) LogoutByToken(token string) (*Session, error) {
	if m == nil {
		return nil, errors.New("session manager is nil")
	}
	m.lock.Lock()
	defer m.lock.Unlock()
	session, ok := m.byToken[token]
	if !ok {
		return nil, errors.New("session not found")
	}
	result := sessionCopy(session)
	m.removeLocked(token)
	return result, nil
}

// LogoutByConnID 删除连接对应的会话。
func (m *Manager) LogoutByConnID(connID uint32) (*Session, error) {
	if m == nil {
		return nil, errors.New("session manager is nil")
	}
	m.lock.Lock()
	defer m.lock.Unlock()
	token, ok := m.byConnID[connID]
	if !ok {
		return nil, errors.New("session not found")
	}
	session := sessionCopy(m.byToken[token])
	m.removeLocked(token)
	return session, nil
}

// Len 返回当前保存的会话数量，包括尚未绑定及等待回收的会话。
func (m *Manager) Len() int {
	if m == nil {
		return 0
	}
	m.lock.RLock()
	defer m.lock.RUnlock()
	return len(m.byToken)
}

func (m *Manager) removeLocked(token string) {
	session, ok := m.byToken[token]
	if !ok {
		return
	}
	delete(m.byToken, token)
	delete(m.byAccountID, session.AccountID)
	delete(m.byPlayerID, session.PlayerID)
	if session.Bound && m.byConnID[session.ConnID] == token {
		delete(m.byConnID, session.ConnID)
	}
}

// Issue 为已验证的稳定玩家身份签发尚未绑定 TCP 连接的会话。
// 返回被替换的会话，由应用层清理旧连接及房间。
func (m *Manager) Issue(accountID string, playerID uint64, ttl time.Duration) (*Session, *Session, error) {
	if m == nil || accountID == "" || playerID == 0 || ttl <= 0 {
		return nil, nil, errors.New("invalid session identity or lifetime")
	}
	token, err := createToken()
	if err != nil {
		return nil, nil, err
	}
	now := time.Now()
	value := &Session{Token: token, AccountID: accountID, PlayerID: playerID, CreatedAt: now, UpdatedAt: now, ExpiresAt: now.Add(ttl)}
	m.lock.Lock()
	defer m.lock.Unlock()
	if oldToken, ok := m.byPlayerID[playerID]; ok && m.byToken[oldToken].AccountID != accountID {
		return nil, nil, errors.New("player belongs to another account")
	}
	var replaced *Session
	if oldToken, ok := m.byAccountID[accountID]; ok {
		if m.accountPolicy == RejectExisting && !expired(m.byToken[oldToken], now) {
			return nil, nil, ErrAccountActive
		}
		replaced = sessionCopy(m.byToken[oldToken])
		m.removeLocked(oldToken)
	}
	m.byToken[token] = value
	m.byAccountID[accountID] = token
	m.byPlayerID[playerID] = token
	m.observeLegacyID(playerID)
	return sessionCopy(value), replaced, nil
}

// Bind 将有效 Token 绑定到连接；连接 ID 0 也是合法值。
// 重复绑定同一连接幂等，不允许跨连接抢占或在连接上切换身份。
func (m *Manager) Bind(token string, connID uint32) (*Session, error) {
	if m == nil {
		return nil, errors.New("session manager is nil")
	}
	m.lock.Lock()
	defer m.lock.Unlock()
	value, ok := m.byToken[token]
	if !ok || expired(value, time.Now()) {
		return nil, errors.New("invalid or expired token")
	}
	if value.Bound && value.ConnID != connID {
		return nil, errors.New("token already bound")
	}
	if other, ok := m.byConnID[connID]; ok && other != token {
		return nil, errors.New("connection already authenticated")
	}
	value.ConnID, value.Bound, value.UpdatedAt = connID, true, time.Now()
	m.byConnID[connID] = token
	return sessionCopy(value), nil
}

// RemoveExpired 移除到期会话，返回快照供应用层关闭连接、清理房间。
func (m *Manager) RemoveExpired(now time.Time) []Session {
	if m == nil {
		return nil
	}
	m.lock.Lock()
	defer m.lock.Unlock()
	removed := make([]Session, 0)
	for token, value := range m.byToken {
		if expired(value, now) {
			removed = append(removed, *value)
			m.removeLocked(token)
		}
	}
	return removed
}

func expired(value *Session, now time.Time) bool {
	return !value.ExpiresAt.IsZero() && !now.Before(value.ExpiresAt)
}

func createToken() (string, error) {
	data := make([]byte, 24)
	if _, err := rand.Read(data); err != nil {
		return "", err
	}
	return hex.EncodeToString(data), nil
}

func sessionCopy(value *Session) *Session {
	if value == nil {
		return nil
	}
	result := *value
	return &result
}
