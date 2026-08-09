package test

import (
	"Ginx/unity"
	"testing"
)

func TestUnityPositionMarshalAndUnmarshal(t *testing.T) {
	want := unity.UnityPosition{X: 1.5, Y: -2.25, Z: 3.75, V: 90}
	data, err := want.Marshal()
	if err != nil {
		t.Fatalf("Marshal() error = %v", err)
	}

	got := unity.UnityPosition{}
	if err := got.Unmarshal(data); err != nil {
		t.Fatalf("Unmarshal() error = %v", err)
	}
	if got != want {
		t.Fatalf("position = %+v, want %+v", got, want)
	}
}

func TestUnityBroadcastMarshalAndUnmarshal(t *testing.T) {
	want := unity.UnityBroadCast{
		PID:        7,
		TP:         3,
		Content:    "move",
		P:          &unity.UnityPosition{X: 10, Y: 20, Z: 30, V: 45},
		ActionData: 5,
	}
	data, err := want.Marshal()
	if err != nil {
		t.Fatalf("Marshal() error = %v", err)
	}

	got := unity.UnityBroadCast{}
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
	want := unity.UnitySyncPlayers{
		Players: []unity.UnityPlayer{
			{PID: 1, P: unity.UnityPosition{X: 1}},
			{PID: 2, P: unity.UnityPosition{Y: 2}},
		},
	}
	data, err := want.Marshal()
	if err != nil {
		t.Fatalf("Marshal() error = %v", err)
	}

	got := unity.UnitySyncPlayers{}
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
	want := unity.UnityMovePackage{
		P:          unity.UnityPosition{X: 4, Y: 5, Z: 6, V: 7},
		ActionData: 3,
	}
	data, err := want.Marshal()
	if err != nil {
		t.Fatalf("Marshal() error = %v", err)
	}

	got := unity.UnityMovePackage{}
	if err := got.Unmarshal(data); err != nil {
		t.Fatalf("Unmarshal() error = %v", err)
	}
	if got != want {
		t.Fatalf("move package = %+v, want %+v", got, want)
	}
}

func TestUnityRoomProtocolMarshalAndUnmarshal(t *testing.T) {
	request := unity.UnityRoomRequest{RoomID: 99}
	requestData, err := request.Marshal()
	if err != nil {
		t.Fatalf("room request Marshal() error = %v", err)
	}
	decodedRequest := unity.UnityRoomRequest{}
	if err := decodedRequest.Unmarshal(requestData); err != nil {
		t.Fatalf("room request Unmarshal() error = %v", err)
	}
	if decodedRequest != request {
		t.Fatalf("room request = %+v, want %+v", decodedRequest, request)
	}

	response := unity.UnityRoomResponse{RoomID: 99, Code: -1, Message: "room is full", PlayerCount: 8}
	responseData, err := response.Marshal()
	if err != nil {
		t.Fatalf("room response Marshal() error = %v", err)
	}
	decodedResponse := unity.UnityRoomResponse{}
	if err := decodedResponse.Unmarshal(responseData); err != nil {
		t.Fatalf("room response Unmarshal() error = %v", err)
	}
	if decodedResponse != response {
		t.Fatalf("room response = %+v, want %+v", decodedResponse, response)
	}

	event := unity.UnityRoomEvent{RoomID: 99, PlayerID: 1001, Content: "ready"}
	eventData, err := event.Marshal()
	if err != nil {
		t.Fatalf("room event Marshal() error = %v", err)
	}
	decodedEvent := unity.UnityRoomEvent{}
	if err := decodedEvent.Unmarshal(eventData); err != nil {
		t.Fatalf("room event Unmarshal() error = %v", err)
	}
	if decodedEvent != event {
		t.Fatalf("room event = %+v, want %+v", decodedEvent, event)
	}
}

func TestUnityProtocolRejectsBrokenPayload(t *testing.T) {
	if err := (&unity.UnityTalk{}).Unmarshal([]byte{0x0a, 0x05, 'h'}); err == nil {
		t.Fatal("UnityTalk.Unmarshal() accepted an incomplete payload")
	}
	if err := (&unity.UnityPosition{}).Unmarshal([]byte{0x0d, 0x01}); err == nil {
		t.Fatal("UnityPosition.Unmarshal() accepted an incomplete fixed32 field")
	}
	if err := (&unity.UnityRoomRequest{}).Unmarshal([]byte{0x08, 0x80, 0x80, 0x80, 0x80, 0x80, 0x80, 0x80, 0x80, 0x80, 0x02}); err == nil {
		t.Fatal("UnityRoomRequest.Unmarshal() accepted an overflowing varint")
	}
}
