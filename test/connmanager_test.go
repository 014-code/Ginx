package test

import (
	"Ginx/gnet"
	"testing"
	"time"
)

func TestConnManagerAddGetRemoveAndClear(t *testing.T) {
	setServerConfig(t, 2)
	manager := gnet.NewConnManager()
	first := newManagedTestConnection(1)
	second := newManagedTestConnection(2)

	if err := manager.Add(first); err != nil {
		t.Fatalf("Add() first connection error = %v", err)
	}
	if err := manager.Add(second); err != nil {
		t.Fatalf("Add() second connection error = %v", err)
	}
	if got := manager.Len(); got != 2 {
		t.Fatalf("Len() = %d, want 2", got)
	}

	connection, err := manager.Get(first.GetConnId())
	if err != nil {
		t.Fatalf("Get() error = %v", err)
	}
	if connection != first {
		t.Fatal("Get() returned an unexpected connection")
	}

	manager.Remove(first.GetConnId())
	if got := manager.Len(); got != 1 {
		t.Fatalf("Len() after Remove() = %d, want 1", got)
	}
	if _, err := manager.Get(first.GetConnId()); err == nil {
		t.Fatal("Get() found a removed connection")
	}

	manager.ClearConn()
	if got := manager.Len(); got != 0 {
		t.Fatalf("Len() after ClearConn() = %d, want 0", got)
	}
	select {
	case <-second.stopped:
	case <-time.After(time.Second):
		t.Fatal("ClearConn() did not stop the managed connection")
	}
}

func TestConnManagerRejectsDuplicateAndOverLimitConnections(t *testing.T) {
	setServerConfig(t, 1)
	manager := gnet.NewConnManager()
	first := newManagedTestConnection(1)

	if err := manager.Add(first); err != nil {
		t.Fatalf("Add() first connection error = %v", err)
	}
	if err := manager.Add(newManagedTestConnection(1)); err == nil {
		t.Fatal("Add() accepted a duplicate connection ID")
	}
	if err := manager.Add(newManagedTestConnection(2)); err == nil {
		t.Fatal("Add() accepted a connection beyond MaxConn")
	}
}

func TestConnManagerAllowsUnlimitedConnectionsWhenMaxConnIsNonPositive(t *testing.T) {
	setServerConfig(t, 0)
	manager := gnet.NewConnManager()

	for id := uint32(0); id < 3; id++ {
		if err := manager.Add(newManagedTestConnection(id)); err != nil {
			t.Fatalf("Add() connection %d error = %v", id, err)
		}
	}
	if got := manager.Len(); got != 3 {
		t.Fatalf("Len() = %d, want 3", got)
	}
}
