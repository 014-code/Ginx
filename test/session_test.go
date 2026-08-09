package test

import (
	"Ginx/session"
	"sync"
	"testing"
	"time"
)

func TestSessionLoginLookupTouchAndLogout(t *testing.T) {
	manager := session.NewManager(1000)
	created, replaced, err := manager.Login("account-1", 7)
	if err != nil {
		t.Fatalf("Login() error = %v", err)
	}
	if replaced != nil || created.PlayerID != 1001 || created.Token == "" {
		t.Fatalf("created=%+v replaced=%+v", created, replaced)
	}

	byToken, err := manager.GetByToken(created.Token)
	if err != nil || byToken.PlayerID != created.PlayerID {
		t.Fatalf("GetByToken() session=%+v error=%v", byToken, err)
	}
	before := byToken.UpdatedAt
	time.Sleep(time.Millisecond)
	if err := manager.Touch(created.Token); err != nil {
		t.Fatalf("Touch() error = %v", err)
	}
	byConn, err := manager.GetByConnID(7)
	if err != nil || !byConn.UpdatedAt.After(before) {
		t.Fatalf("GetByConnID() session=%+v error=%v", byConn, err)
	}

	removed, err := manager.LogoutByConnID(7)
	if err != nil || removed.Token != created.Token || manager.Len() != 0 {
		t.Fatalf("LogoutByConnID() session=%+v error=%v len=%d", removed, err, manager.Len())
	}
}

func TestSessionLoginReplacesSameAccountAndConnection(t *testing.T) {
	manager := session.NewManager(0)
	first, replaced, err := manager.Login("account-1", 1)
	if err != nil || replaced != nil {
		t.Fatalf("first Login() session=%+v replaced=%+v error=%v", first, replaced, err)
	}
	second, replaced, err := manager.Login("account-1", 2)
	if err != nil || replaced == nil || replaced.Token != first.Token {
		t.Fatalf("second Login() session=%+v replaced=%+v error=%v", second, replaced, err)
	}
	if manager.Len() != 1 {
		t.Fatalf("Len() = %d, want 1", manager.Len())
	}
	if _, err := manager.GetByConnID(1); err == nil {
		t.Fatal("old connection still has a session")
	}
	if got, err := manager.GetByPlayerID(second.PlayerID); err != nil || got.Token != second.Token {
		t.Fatalf("GetByPlayerID() session=%+v error=%v", got, err)
	}
}

func TestSessionManagerRejectsInvalidAndMissingOperations(t *testing.T) {
	manager := session.NewManager(1)
	if _, _, err := manager.Login("", 1); err == nil {
		t.Fatal("Login() accepted an empty account")
	}
	if _, err := manager.GetByToken("missing"); err == nil {
		t.Fatal("GetByToken() found a missing session")
	}
	if err := manager.Touch("missing"); err == nil {
		t.Fatal("Touch() accepted a missing session")
	}
	if _, err := manager.LogoutByConnID(1); err == nil {
		t.Fatal("LogoutByConnID() accepted a missing session")
	}
	if _, err := manager.LogoutByToken("missing"); err == nil {
		t.Fatal("LogoutByToken() accepted a missing session")
	}
}

func TestSessionManagerGeneratesUniquePlayersConcurrently(t *testing.T) {
	manager := session.NewManager(100)
	const count = 32
	players := make(chan uint64, count)
	var waitGroup sync.WaitGroup
	for index := 0; index < count; index++ {
		waitGroup.Add(1)
		go func(index int) {
			defer waitGroup.Done()
			created, _, err := manager.Login("account-"+string(rune('a'+index)), uint32(index+1))
			if err == nil {
				players <- created.PlayerID
			}
		}(index)
	}
	waitGroup.Wait()
	close(players)
	seen := make(map[uint64]struct{}, count)
	for playerID := range players {
		seen[playerID] = struct{}{}
	}
	if len(seen) != count || manager.Len() != count {
		t.Fatalf("concurrent sessions = %d unique players, manager len = %d, want %d", len(seen), manager.Len(), count)
	}
}
