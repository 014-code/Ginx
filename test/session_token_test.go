package test

import (
	"Ginx/session"
	"sync"
	"sync/atomic"
	"testing"
	"time"
)

func TestSessionIssueBindAndReplacement(t *testing.T) {
	m := session.NewManager(0)
	a, _, err := m.Issue("alice", 1001, time.Hour)
	if err != nil || a.Bound {
		t.Fatalf("issue: %v", err)
	}
	b, _, err := m.Issue("bob", 1002, time.Hour)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := m.GetByConnID(0); err == nil {
		t.Fatal("pending token claimed connection zero")
	}
	if value, err := m.Bind(a.Token, 0); err != nil || !value.Bound || value.PlayerID != 1001 {
		t.Fatalf("bind: %v", err)
	}
	if _, err := m.Bind(a.Token, 0); err != nil {
		t.Fatal("same binding must be idempotent")
	}
	if _, err := m.Bind(a.Token, 2); err == nil {
		t.Fatal("token stolen by another connection")
	}
	if _, err := m.Bind(b.Token, 0); err == nil {
		t.Fatal("connection switched identity")
	}
	_, _ = m.LogoutByToken(b.Token)
	if _, err := m.GetByConnID(0); err != nil {
		t.Fatal("unbound logout removed real connection zero")
	}
	if _, _, err := m.Issue("mallory", 1001, time.Hour); err == nil {
		t.Fatal("player identity reused by another account")
	}
	newToken, old, err := m.Issue("alice", 1001, time.Hour)
	if err != nil || old == nil || !old.Bound || old.ConnID != 0 || newToken.PlayerID != a.PlayerID {
		t.Fatalf("replacement: %v", err)
	}
	if _, err := m.GetByToken(a.Token); err == nil {
		t.Fatal("old token still valid")
	}
	if _, err := m.GetByConnID(0); err == nil {
		t.Fatal("old connection still authenticated")
	}
}

func TestSessionExpiryAndConcurrentBind(t *testing.T) {
	m := session.NewManager(0)
	value, _, _ := m.Issue("alice", 1, time.Hour)
	var winners atomic.Int32
	var wg sync.WaitGroup
	for i := 0; i < 16; i++ {
		wg.Add(1)
		go func(id uint32) {
			defer wg.Done()
			if _, err := m.Bind(value.Token, id); err == nil {
				winners.Add(1)
			}
		}(uint32(i))
	}
	wg.Wait()
	if winners.Load() != 1 {
		t.Fatalf("binding winners = %d", winners.Load())
	}
	removed := m.RemoveExpired(value.ExpiresAt)
	if len(removed) != 1 || !removed[0].Bound || m.Len() != 0 {
		t.Fatal("expired indexes not removed")
	}
	if _, err := m.GetByConnID(removed[0].ConnID); err == nil {
		t.Fatal("expired connection index remains")
	}
	expired, _, _ := m.Issue("alice", 1, time.Nanosecond)
	deadline := time.Now().Add(time.Second)
	for time.Now().Before(expired.ExpiresAt) && time.Now().Before(deadline) {
		time.Sleep(time.Microsecond)
	}
	if _, err := m.GetByToken(expired.Token); err == nil {
		t.Fatal("expired token accepted")
	}
	if _, err := m.Bind(expired.Token, 1); err == nil {
		t.Fatal("expired token bound")
	}
	if err := m.Touch(expired.Token); err == nil {
		t.Fatal("touch revived token")
	}
}
