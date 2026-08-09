package test

import (
	"Ginx/gcore"
	"Ginx/gface"
	"testing"
)

type roomTestMessage struct {
	msgID uint32
	data  string
}

type roomTestConnection struct {
	testConnection
	messages chan roomTestMessage
}

func newRoomTestConnection(id uint32) *roomTestConnection {
	return &roomTestConnection{
		testConnection: testConnection{id: id},
		messages:       make(chan roomTestMessage, 8),
	}
}

func (c *roomTestConnection) SendBuffMsg(msgID uint32, data []byte) error {
	c.messages <- roomTestMessage{msgID: msgID, data: string(data)}
	return nil
}

func TestRoomJoinLeaveStateAndCapacity(t *testing.T) {
	room := gcore.NewRoom(100, 2)
	first := newRoomTestConnection(1)
	second := newRoomTestConnection(2)
	third := newRoomTestConnection(3)

	if err := room.Join(first); err != nil {
		t.Fatalf("Join() first error = %v", err)
	}
	if err := room.Join(second); err != nil {
		t.Fatalf("Join() second error = %v", err)
	}
	if err := room.Join(third); err == nil {
		t.Fatal("Join() accepted a full room")
	}
	if err := room.Join(first); err == nil {
		t.Fatal("Join() accepted a duplicate member")
	}
	if room.PlayerCount() != 2 {
		t.Fatalf("PlayerCount() = %d, want 2", room.PlayerCount())
	}

	if err := room.Start(); err != nil {
		t.Fatalf("Start() error = %v", err)
	}
	if room.State() != gcore.RoomStateRunning {
		t.Fatalf("State() = %v, want running", room.State())
	}
	if err := room.Leave(first.GetConnId()); err != nil {
		t.Fatalf("Leave() error = %v", err)
	}
	if err := room.Leave(first.GetConnId()); err == nil {
		t.Fatal("Leave() accepted a missing member")
	}

	room.Close()
	if room.State() != gcore.RoomStateClosed {
		t.Fatalf("State() after Close() = %v, want closed", room.State())
	}
	if err := room.Join(third); err == nil {
		t.Fatal("Join() accepted a closed room")
	}
}

func TestRoomBroadcastUsesMemberSnapshot(t *testing.T) {
	room := gcore.NewRoom(100, 0)
	first := newRoomTestConnection(1)
	second := newRoomTestConnection(2)
	if err := room.Join(first); err != nil {
		t.Fatalf("Join() first error = %v", err)
	}
	if err := room.Join(second); err != nil {
		t.Fatalf("Join() second error = %v", err)
	}

	if broadcastErrors := room.Broadcast(2004, []byte("room tick")); len(broadcastErrors) != 0 {
		t.Fatalf("Broadcast() errors = %v", broadcastErrors)
	}
	assertRoomMessage(t, first, 2004, "room tick")
	assertRoomMessage(t, second, 2004, "room tick")

	if broadcastErrors := room.BroadcastExcept(first.GetConnId(), 2005, []byte("except first")); len(broadcastErrors) != 0 {
		t.Fatalf("BroadcastExcept() errors = %v", broadcastErrors)
	}
	assertRoomMessage(t, second, 2005, "except first")
}

func TestRoomManagerCreatesGetsAndRemovesRooms(t *testing.T) {
	manager := gcore.NewRoomManager()
	room, err := manager.CreateRoom(10, 4)
	if err != nil {
		t.Fatalf("CreateRoom() error = %v", err)
	}
	if _, err := manager.CreateRoom(10, 4); err == nil {
		t.Fatal("CreateRoom() accepted a duplicate room")
	}
	got, err := manager.GetRoom(room.ID)
	if err != nil || got != room {
		t.Fatalf("GetRoom() room=%v error=%v, want created room", got, err)
	}
	if err := manager.RemoveRoom(room.ID); err != nil {
		t.Fatalf("RemoveRoom() error = %v", err)
	}
	if manager.Len() != 0 {
		t.Fatalf("Len() = %d, want 0", manager.Len())
	}
	if err := manager.RemoveRoom(room.ID); err == nil {
		t.Fatal("RemoveRoom() accepted a missing room")
	}
}

func assertRoomMessage(t *testing.T, connection *roomTestConnection, wantID uint32, wantData string) {
	t.Helper()
	message := <-connection.messages
	if message.msgID != wantID || message.data != wantData {
		t.Fatalf("room message = %+v, want id:%d data:%q", message, wantID, wantData)
	}
}

var _ gface.IConnection = (*roomTestConnection)(nil)
