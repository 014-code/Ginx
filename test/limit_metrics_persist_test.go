package test

import (
	"Ginx/limit"
	"Ginx/metrics"
	"Ginx/persist"
	"context"
	"errors"
	"os"
	"path/filepath"
	"testing"
	"time"
)

func TestTokenBucketLimitsBurstAndRefills(t *testing.T) {
	if _, err := limit.NewTokenBucket(0, 1); err == nil {
		t.Fatal("NewTokenBucket() accepted a zero rate")
	}
	if _, err := limit.NewTokenBucket(1, 0); err == nil {
		t.Fatal("NewTokenBucket() accepted a zero burst")
	}
	bucket, err := limit.NewTokenBucket(20, 2)
	if err != nil {
		t.Fatalf("NewTokenBucket() error = %v", err)
	}
	if !bucket.Allow() || !bucket.Allow() {
		t.Fatal("token bucket did not allow configured burst")
	}
	if bucket.Allow() {
		t.Fatal("token bucket allowed a request after burst was consumed")
	}
	if bucket.AllowN(3) {
		t.Fatal("token bucket allowed more tokens than its burst")
	}
	time.Sleep(60 * time.Millisecond)
	if !bucket.Allow() {
		t.Fatal("token bucket did not refill a token")
	}
}

func TestMetricsSnapshot(t *testing.T) {
	collector := &metrics.Metrics{}
	collector.AddConnections(2)
	collector.AddConnections(-1)
	collector.IncMessages()
	collector.AddBytesIn(10)
	collector.AddBytesOut(20)
	collector.IncRouterPanics()

	snapshot := collector.Snapshot()
	if snapshot.Connections != 1 || snapshot.Messages != 1 || snapshot.BytesIn != 10 || snapshot.BytesOut != 20 || snapshot.RouterPanics != 1 {
		t.Fatalf("metrics snapshot = %+v", snapshot)
	}
}

func TestMemoryPlayerStoreCopiesAndHonorsContext(t *testing.T) {
	store := persist.NewMemoryStore()
	player := &persist.Player{PlayerID: 1001, AccountID: "account-1", Data: []byte("state")}
	if err := store.Save(context.Background(), player); err != nil {
		t.Fatalf("Save() error = %v", err)
	}
	player.Data[0] = 'X'
	loaded, err := store.Load(context.Background(), player.PlayerID)
	if err != nil || string(loaded.Data) != "state" {
		t.Fatalf("Load() player=%+v error=%v", loaded, err)
	}
	canceled, cancel := context.WithCancel(context.Background())
	cancel()
	if _, err := store.Load(canceled, player.PlayerID); !errors.Is(err, context.Canceled) {
		t.Fatalf("Load() error = %v, want context.Canceled", err)
	}
	if err := store.Delete(context.Background(), player.PlayerID); err != nil {
		t.Fatalf("Delete() error = %v", err)
	}
	if _, err := store.Load(context.Background(), player.PlayerID); !errors.Is(err, persist.ErrPlayerNotFound) {
		t.Fatalf("Load() error = %v, want ErrPlayerNotFound", err)
	}
}

func TestJSONFilePlayerStorePersistsPlayer(t *testing.T) {
	path := filepath.Join(t.TempDir(), "players.json")
	store, err := persist.NewJSONFileStore(path)
	if err != nil {
		t.Fatalf("NewJSONFileStore() error = %v", err)
	}
	want := &persist.Player{PlayerID: 2001, AccountID: "account-2", Data: []byte("json-state"), UpdatedAt: time.Now()}
	if err := store.Save(context.Background(), want); err != nil {
		t.Fatalf("Save() error = %v", err)
	}
	loaded, err := store.Load(context.Background(), want.PlayerID)
	if err != nil {
		t.Fatalf("Load() error = %v", err)
	}
	if loaded.AccountID != want.AccountID || string(loaded.Data) != string(want.Data) || !loaded.UpdatedAt.Equal(want.UpdatedAt) {
		t.Fatalf("loaded player = %+v, want %+v", loaded, want)
	}
	if err := store.Delete(context.Background(), want.PlayerID); err != nil {
		t.Fatalf("Delete() error = %v", err)
	}
}

func TestJSONFilePlayerStoreRejectsBrokenFile(t *testing.T) {
	path := filepath.Join(t.TempDir(), "players.json")
	if err := os.WriteFile(path, []byte("not-json"), 0600); err != nil {
		t.Fatalf("WriteFile() error = %v", err)
	}
	store, err := persist.NewJSONFileStore(path)
	if err != nil {
		t.Fatalf("NewJSONFileStore() error = %v", err)
	}
	if _, err := store.Load(context.Background(), 1); err == nil {
		t.Fatal("Load() accepted a malformed JSON store")
	}
}
