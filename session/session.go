// Package session 提供与具体网络协议无关的内存会话管理。
package session

import (
	"crypto/rand"
	"encoding/hex"
	"errors"
	"sync"
	"sync/atomic"
	"time"
)

// Session 保存一次登录会话的短期状态。
type Session struct {
	Token     string
	AccountID string
	PlayerID  uint64
	ConnID    uint32
	CreatedAt time.Time
	UpdatedAt time.Time
}

// Manager 管理账号、玩家、连接和 Token 之间的映射关系。
type Manager struct {
	nextPlayerID atomic.Uint64
	lock         sync.RWMutex
	byToken      map[string]*Session
	byAccountID  map[string]string
	byPlayerID   map[uint64]string
	byConnID     map[uint32]string
}

// NewManager 创建一个会话管理器，生成的 PlayerID 从 startPlayerID 之后开始递增。
func NewManager(startPlayerID uint64) *Manager {
	manager := &Manager{
		byToken:     make(map[string]*Session),
		byAccountID: make(map[string]string),
		byPlayerID:  make(map[uint64]string),
		byConnID:    make(map[uint32]string),
	}
	manager.nextPlayerID.Store(startPlayerID)
	return manager
}

// Login 创建一个新会话，并返回同账号被替换的旧会话。
func (m *Manager) Login(accountID string, connID uint32) (*Session, *Session, error) {
	if m == nil {
		return nil, nil, errors.New("session manager is nil")
	}
	if accountID == "" {
		return nil, nil, errors.New("account id is empty")
	}

	token, err := createToken()
	if err != nil {
		return nil, nil, err
	}
	now := time.Now()
	session := &Session{
		Token:     token,
		AccountID: accountID,
		PlayerID:  m.nextPlayerID.Add(1),
		ConnID:    connID,
		CreatedAt: now,
		UpdatedAt: now,
	}

	m.lock.Lock()
	defer m.lock.Unlock()
	var replaced *Session
	if oldToken, ok := m.byAccountID[accountID]; ok {
		replaced = sessionCopy(m.byToken[oldToken])
		m.removeLocked(oldToken)
	}
	if oldToken, ok := m.byConnID[connID]; ok {
		m.removeLocked(oldToken)
	}
	m.byToken[token] = session
	m.byAccountID[accountID] = token
	m.byPlayerID[session.PlayerID] = token
	m.byConnID[connID] = token
	return sessionCopy(session), replaced, nil
}

// GetByToken 根据 Token 获取会话快照。
func (m *Manager) GetByToken(token string) (*Session, error) {
	if m == nil {
		return nil, errors.New("session manager is nil")
	}
	m.lock.RLock()
	defer m.lock.RUnlock()
	session, ok := m.byToken[token]
	if !ok {
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
	return sessionCopy(m.byToken[token]), nil
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
	return sessionCopy(m.byToken[token]), nil
}

// Touch 刷新会话最后活跃时间。
func (m *Manager) Touch(token string) error {
	if m == nil {
		return errors.New("session manager is nil")
	}
	m.lock.Lock()
	defer m.lock.Unlock()
	session, ok := m.byToken[token]
	if !ok {
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

// Len 返回当前在线会话数量。
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
	delete(m.byConnID, session.ConnID)
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
