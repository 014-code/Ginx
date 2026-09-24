package session

import (
	"errors"
	"sync/atomic"
	"time"
)

type legacyState struct{ nextPlayerID atomic.Uint64 }

// NewManager 保留旧自动编号入口。
//
// Deprecated: 使用 New(Options)，由应用提供 PlayerID。
func NewManager(startPlayerID uint64) *Manager {
	m, _ := New(Options{AccountPolicy: ReplaceExisting})
	m.legacy = &legacyState{}
	m.legacy.nextPlayerID.Store(startPlayerID)
	return m
}

// Login 保留旧自动编号登录行为。
//
// Deprecated: 应用分配 PlayerID 后调用 Issue 和 Bind。
func (m *Manager) Login(accountID string, connID uint32) (*Session, *Session, error) {
	if m == nil {
		return nil, nil, errors.New("session manager is nil")
	}
	if m.legacy == nil {
		return nil, nil, errors.New("automatic player IDs require the deprecated NewManager constructor")
	}
	if accountID == "" {
		return nil, nil, errors.New("account id is empty")
	}

	token, err := createToken()
	if err != nil {
		return nil, nil, err
	}
	m.lock.Lock()
	defer m.lock.Unlock()
	now := time.Now()
	session := &Session{
		Token:     token,
		AccountID: accountID,
		PlayerID:  m.legacy.nextPlayerID.Add(1),
		ConnID:    connID,
		Bound:     true,
		CreatedAt: now,
		UpdatedAt: now,
	}

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

func (m *Manager) observeLegacyID(id uint64) {
	if m.legacy == nil {
		return
	}
	for current := m.legacy.nextPlayerID.Load(); current < id; current = m.legacy.nextPlayerID.Load() {
		if m.legacy.nextPlayerID.CompareAndSwap(current, id) {
			break
		}
	}
}
