package test

import (
	"Ginx/gcore"
	"math"
	"testing"
)

func TestAOIManagerTracksNearbyEntitiesAndChanges(t *testing.T) {
	manager, err := gcore.NewAOIManager(100, 100, 10, 10)
	if err != nil {
		t.Fatalf("NewAOIManager() error = %v", err)
	}

	for id, position := range map[uint32]gcore.Position{
		1: {X: 5, Y: 5},
		2: {X: 15, Y: 5},
		3: {X: 35, Y: 5},
	} {
		if err := manager.AddEntity(id, position); err != nil {
			t.Fatalf("AddEntity(%d) error = %v", id, err)
		}
	}

	nearby, err := manager.GetNearbyEntityIDs(1)
	if err != nil {
		t.Fatalf("GetNearbyEntityIDs() error = %v", err)
	}
	if len(nearby) != 1 || nearby[0] != 2 {
		t.Fatalf("nearby entities = %v, want [2]", nearby)
	}

	change, err := manager.MoveEntity(1, gcore.Position{X: 35, Y: 5})
	if err != nil {
		t.Fatalf("MoveEntity() error = %v", err)
	}
	if len(change.Entered) != 1 || change.Entered[0] != 3 {
		t.Fatalf("entered entities = %v, want [3]", change.Entered)
	}
	if len(change.Left) != 1 || change.Left[0] != 2 {
		t.Fatalf("left entities = %v, want [2]", change.Left)
	}
}

func TestAOIManagerRejectsInvalidEntitiesAndPositions(t *testing.T) {
	if _, err := gcore.NewAOIManager(0, 100, 10, 10); err == nil {
		t.Fatal("NewAOIManager() accepted an invalid world size")
	}

	manager, err := gcore.NewAOIManager(100, 100, 10, 10)
	if err != nil {
		t.Fatalf("NewAOIManager() error = %v", err)
	}
	if err := manager.AddEntity(1, gcore.Position{X: 100, Y: 5}); err == nil {
		t.Fatal("AddEntity() accepted an out-of-world position")
	}
	if err := manager.AddEntity(1, gcore.Position{X: 5, Y: 5}); err != nil {
		t.Fatalf("AddEntity() error = %v", err)
	}
	if err := manager.AddEntity(1, gcore.Position{X: 6, Y: 6}); err == nil {
		t.Fatal("AddEntity() accepted a duplicate entity")
	}
	if err := manager.RemoveEntity(2); err == nil {
		t.Fatal("RemoveEntity() found a missing entity")
	}
	for _, position := range []gcore.Position{{X: math.NaN(), Y: 1}, {X: 1, Y: math.Inf(1)}} {
		if err := manager.AddEntity(2, position); err == nil {
			t.Fatalf("AddEntity() accepted non-finite position %+v", position)
		}
	}
}
