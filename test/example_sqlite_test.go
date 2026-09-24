package test

import (
	"Ginx/examples/gameapp"
	"Ginx/examples/sqlitestore"
	"Ginx/persist"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"path/filepath"
	"sync"
	"testing"
	"time"
)

func exampleStore(t *testing.T, path string) *sqlitestore.Store {
	t.Helper()
	s, err := sqlitestore.Open(path)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { s.Close() })
	return s
}

func TestExampleSQLiteTransactions(t *testing.T) {
	ctx := context.Background()
	path := filepath.Join(t.TempDir(), "space # ?", "players.db")
	s := exampleStore(t, path)
	if _, err := s.Load(ctx, 1001); !errors.Is(err, persist.ErrPlayerNotFound) {
		t.Fatal(err)
	}
	if err := s.Save(ctx, &persist.Player{PlayerID: 1001, AccountID: "alice", Data: []byte(`0`)}); err != nil {
		t.Fatal(err)
	}
	if err := s.Save(ctx, &persist.Player{PlayerID: 1001, AccountID: "bob"}); !errors.Is(err, sqlitestore.ErrIdentityConflict) {
		t.Fatal(err)
	}
	failure := errors.New("business rule rejected")
	if err := s.Update(ctx, 1001, func(p *persist.Player) error { p.Data = []byte(`999`); return failure }); !errors.Is(err, failure) {
		t.Fatal(err)
	}
	if err := s.Update(ctx, 1001, func(p *persist.Player) error { p.AccountID = "bob"; return nil }); !errors.Is(err, sqlitestore.ErrIdentityConflict) {
		t.Fatal(err)
	}
	cancelled, cancel := context.WithCancel(ctx)
	if err := s.Update(cancelled, 1001, func(p *persist.Player) error { p.Data = []byte(`999`); cancel(); return nil }); err == nil {
		t.Fatal("cancelled update committed")
	}
	p, err := s.Load(ctx, 1001)
	if err != nil || string(p.Data) != "0" {
		t.Fatalf("rollback: %+v %v", p, err)
	}
	// 两个独立 Store 实例争用同一数据库，不能丢失读改写。
	other := exampleStore(t, path)
	var wg sync.WaitGroup
	for i := 0; i < 20; i++ {
		wg.Add(1)
		go func(i int) {
			defer wg.Done()
			store := s
			if i%2 != 0 {
				store = other
			}
			err := store.Update(ctx, 1001, func(p *persist.Player) error {
				var count int
				if err := json.Unmarshal(p.Data, &count); err != nil {
					return err
				}
				p.Data = []byte(fmt.Sprint(count + 1))
				return nil
			})
			if err != nil {
				t.Error(err)
			}
		}(i)
	}
	wg.Wait()
	p, err = s.Load(ctx, 1001)
	if err != nil || string(p.Data) != "20" || p.UpdatedAt.IsZero() {
		t.Fatalf("lost update: %+v %v", p, err)
	}
	if err := s.Delete(ctx, 1001); err != nil {
		t.Fatal(err)
	}
	if err := s.Delete(ctx, 1001); !errors.Is(err, persist.ErrPlayerNotFound) {
		t.Fatal(err)
	}
	// 不把 Go 的 uint64 截断为 SQLite 的有符号整数。
	maxID := ^uint64(0)
	if err := s.Save(ctx, &persist.Player{PlayerID: maxID, AccountID: "max"}); err != nil {
		t.Fatal(err)
	}
	if p, err := s.Load(ctx, maxID); err != nil || p.PlayerID != maxID {
		t.Fatal(p, err)
	}
}

func TestExampleProgressPersistenceAndRules(t *testing.T) {
	ctx := context.Background()
	path := filepath.Join(t.TempDir(), "players.db")
	store := exampleStore(t, path)
	accounts := gameAccounts(t)
	s, err := gameapp.New(accounts, store, time.Hour, []gameapp.RoomConfig{{ID: 1, MaxPlayers: 2}})
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(s.Close)
	if _, err := s.ClaimStarter(ctx, 1); !errors.Is(err, gameapp.ErrUnauthorized) {
		t.Fatal(err)
	}
	login, err := s.Login(ctx, "alice", "test-password")
	if err != nil {
		t.Fatal(err)
	}
	c := newManagedTestConnection(1)
	s.Connected(c)
	if _, err := s.Authenticate(1, login.Token); err != nil {
		t.Fatal(err)
	}
	if _, err := s.ClaimStarter(ctx, 1); !errors.Is(err, gameapp.ErrNotInRoom) {
		t.Fatal(err)
	}
	if _, err := s.Join(1, 1); err != nil {
		t.Fatal(err)
	}
	var wg sync.WaitGroup
	results := make(chan error, 8)
	for i := 0; i < 8; i++ {
		wg.Add(1)
		go func() { defer wg.Done(); _, err := s.ClaimStarter(ctx, 1); results <- err }()
	}
	wg.Wait()
	close(results)
	winners := 0
	for err := range results {
		if err == nil {
			winners++
		} else if !errors.Is(err, gameapp.ErrRewardClaimed) {
			t.Fatal(err)
		}
	}
	if winners != 1 {
		t.Fatalf("reward winners: %d", winners)
	}
	p, err := s.UseItem(ctx, 1, "potion")
	if err != nil || p.Level != 2 || p.Experience != 100 || p.Inventory["potion"] != 1 {
		t.Fatal(p, err)
	}
	if _, err := s.UseItem(ctx, 1, "arbitrary"); !errors.Is(err, gameapp.ErrItemUnavailable) {
		t.Fatal(err)
	}
	s.Close()
	if err := store.Close(); err != nil {
		t.Fatal(err)
	}
	// 重开数据库与业务服务，Token 不恢复，成长与背包恢复。
	reopened := exampleStore(t, path)
	restarted, err := gameapp.New(accounts, reopened, time.Hour, []gameapp.RoomConfig{{ID: 1, MaxPlayers: 2}})
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(restarted.Close)
	if _, err := restarted.Progress(ctx, login.Token); !errors.Is(err, gameapp.ErrUnauthorized) {
		t.Fatal(err)
	}
	login, err = restarted.Login(ctx, "alice", "test-password")
	if err != nil {
		t.Fatal(err)
	}
	p, err = restarted.Progress(ctx, login.Token)
	if err != nil || p.Level != 2 || p.Inventory["potion"] != 1 || !p.StarterClaimed {
		t.Fatal(p, err)
	}
	restarted.Connected(newManagedTestConnection(2))
	if _, err := restarted.Authenticate(2, login.Token); err != nil {
		t.Fatal(err)
	}
	if _, err := restarted.Join(2, 1); err != nil {
		t.Fatal(err)
	}
	if _, err := restarted.ClaimStarter(ctx, 2); !errors.Is(err, gameapp.ErrRewardClaimed) {
		t.Fatal(err)
	}
	if _, err := restarted.UseItem(ctx, 2, "potion"); err != nil {
		t.Fatal(err)
	}
	if _, err := restarted.UseItem(ctx, 2, "potion"); !errors.Is(err, gameapp.ErrItemUnavailable) {
		t.Fatal(err)
	}
	router := gameapp.NewHTTPHandler(restarted)
	if res := gameHTTP(t, router, "GET", "/api/v1/progress", login.Token, ""); res.Code != 200 {
		t.Fatal(res.Body.String())
	}
	if res := gameHTTP(t, router, "GET", "/api/v1/progress", "bad", ""); res.Code != 401 {
		t.Fatal(res.Code)
	}
	if err := reopened.Close(); err != nil {
		t.Fatal(err)
	}
	if _, err := restarted.UseItem(ctx, 2, "potion"); err == nil {
		t.Fatal("closed database reported success")
	}
}

func TestExampleRejectsInvalidProgressWithoutReset(t *testing.T) {
	ctx := context.Background()
	store := exampleStore(t, filepath.Join(t.TempDir(), "players.db"))
	s, err := gameapp.New(gameAccounts(t), store, time.Hour, []gameapp.RoomConfig{{ID: 1, MaxPlayers: 2}})
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(s.Close)
	login, err := s.Login(ctx, "alice", "test-password")
	if err != nil {
		t.Fatal(err)
	}
	s.Connected(newManagedTestConnection(1))
	if _, err := s.Authenticate(1, login.Token); err != nil {
		t.Fatal(err)
	}
	if _, err := s.Join(1, 1); err != nil {
		t.Fatal(err)
	}
	for _, data := range []string{
		`{`, `null`, `{}`, `{"version":2,"level":1,"experience":0,"inventory":{}}`,
		`{"version":1,"level":2,"experience":100,"inventory":{"potion":2},"starter_claimed":false}`,
		`{"version":1,"level":2,"experience":100,"inventory":{"potion":-1},"starter_claimed":true}`,
	} {
		if err := store.Save(ctx, &persist.Player{PlayerID: 1001, AccountID: "alice", Data: []byte(data)}); err != nil {
			t.Fatal(err)
		}
		if _, err := s.Progress(ctx, login.Token); err == nil {
			t.Fatalf("accepted corrupt progress: %s", data)
		}
		if _, err := s.ClaimStarter(ctx, 1); err == nil {
			t.Fatalf("reward reset corrupt progress: %s", data)
		}
		player, err := store.Load(ctx, 1001)
		if err != nil || string(player.Data) != data {
			t.Fatal("corrupt record was silently overwritten", err)
		}
	}
}
