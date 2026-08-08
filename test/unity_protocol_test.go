package test

import (
	"Ginx/gcore"
	"testing"
)

func TestUnityPositionMarshalAndUnmarshal(t *testing.T) {
	want := gcore.UnityPosition{X: 1.5, Y: -2.25, Z: 3.75, V: 90}
	data, err := want.Marshal()
	if err != nil {
		t.Fatalf("Marshal() error = %v", err)
	}

	got := gcore.UnityPosition{}
	if err := got.Unmarshal(data); err != nil {
		t.Fatalf("Unmarshal() error = %v", err)
	}
	if got != want {
		t.Fatalf("position = %+v, want %+v", got, want)
	}
}

func TestUnityBroadcastMarshalAndUnmarshal(t *testing.T) {
	want := gcore.UnityBroadCast{
		PID:        7,
		TP:         3,
		Content:    "move",
		P:          &gcore.UnityPosition{X: 10, Y: 20, Z: 30, V: 45},
		ActionData: 5,
	}
	data, err := want.Marshal()
	if err != nil {
		t.Fatalf("Marshal() error = %v", err)
	}

	got := gcore.UnityBroadCast{}
	if err := got.Unmarshal(data); err != nil {
		t.Fatalf("Unmarshal() error = %v", err)
	}
	if got.P == nil || want.P == nil || *got.P != *want.P {
		t.Fatalf("broadcast position = %+v, want %+v", got.P, want.P)
	}
	got.P = nil
	want.P = nil
	if got != want {
		t.Fatalf("broadcast = %+v, want %+v", got, want)
	}
}

func TestUnitySyncPlayersMarshalAndUnmarshal(t *testing.T) {
	want := gcore.UnitySyncPlayers{
		Players: []gcore.UnityPlayer{
			{PID: 1, P: gcore.UnityPosition{X: 1}},
			{PID: 2, P: gcore.UnityPosition{Y: 2}},
		},
	}
	data, err := want.Marshal()
	if err != nil {
		t.Fatalf("Marshal() error = %v", err)
	}

	got := gcore.UnitySyncPlayers{}
	if err := got.Unmarshal(data); err != nil {
		t.Fatalf("Unmarshal() error = %v", err)
	}
	if len(got.Players) != len(want.Players) {
		t.Fatalf("players length = %d, want %d", len(got.Players), len(want.Players))
	}
	for index := range want.Players {
		if got.Players[index] != want.Players[index] {
			t.Fatalf("player[%d] = %+v, want %+v", index, got.Players[index], want.Players[index])
		}
	}
}

func TestUnityMovePackageMarshalAndUnmarshal(t *testing.T) {
	want := gcore.UnityMovePackage{
		P:          gcore.UnityPosition{X: 4, Y: 5, Z: 6, V: 7},
		ActionData: 3,
	}
	data, err := want.Marshal()
	if err != nil {
		t.Fatalf("Marshal() error = %v", err)
	}

	got := gcore.UnityMovePackage{}
	if err := got.Unmarshal(data); err != nil {
		t.Fatalf("Unmarshal() error = %v", err)
	}
	if got != want {
		t.Fatalf("move package = %+v, want %+v", got, want)
	}
}

func TestUnityProtocolRejectsBrokenPayload(t *testing.T) {
	if err := (&gcore.UnityTalk{}).Unmarshal([]byte{0x0a, 0x05, 'h'}); err == nil {
		t.Fatal("UnityTalk.Unmarshal() accepted an incomplete payload")
	}
	if err := (&gcore.UnityPosition{}).Unmarshal([]byte{0x0d, 0x01}); err == nil {
		t.Fatal("UnityPosition.Unmarshal() accepted an incomplete fixed32 field")
	}
}
