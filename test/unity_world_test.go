package test

import (
	"Ginx/gcore"
	"Ginx/gface"
	"testing"
)

type unityWorldTestConnection struct {
	testConnection
	messages chan unityWorldMessage
}

type unityWorldMessage struct {
	msgID uint32
	data  []byte
}

func newUnityWorldTestConnection(id uint32) *unityWorldTestConnection {
	return &unityWorldTestConnection{
		testConnection: testConnection{id: id},
		messages:       make(chan unityWorldMessage, 8),
	}
}

func (c *unityWorldTestConnection) SendBuffMsg(msgID uint32, data []byte) error {
	c.messages <- unityWorldMessage{msgID: msgID, data: append([]byte(nil), data...)}
	return nil
}

func TestUnityWorldPlayerLifecycle(t *testing.T) {
	world := gcore.NewUnityWorld()
	first := newUnityWorldTestConnection(10)
	second := newUnityWorldTestConnection(20)

	firstPlayer, oldPlayers, err := world.AddPlayer(first)
	if err != nil {
		t.Fatalf("AddPlayer() first error = %v", err)
	}
	if firstPlayer.PID != 10 || len(oldPlayers) != 0 {
		t.Fatalf("first player = %+v, old players = %+v", firstPlayer, oldPlayers)
	}

	secondPlayer, oldPlayers, err := world.AddPlayer(second)
	if err != nil {
		t.Fatalf("AddPlayer() second error = %v", err)
	}
	if secondPlayer.PID != 20 || len(oldPlayers) != 1 || oldPlayers[0].PID != 10 {
		t.Fatalf("second player = %+v, old players = %+v", secondPlayer, oldPlayers)
	}

	position := gcore.UnityPosition{X: 5, Y: 6, Z: 7, V: 90}
	updated, err := world.UpdatePlayer(20, position, 3)
	if err != nil {
		t.Fatalf("UpdatePlayer() error = %v", err)
	}
	if updated.Position != position || updated.ActionData != 3 {
		t.Fatalf("updated player = %+v", updated)
	}

	removed, err := world.RemovePlayer(10)
	if err != nil {
		t.Fatalf("RemovePlayer() error = %v", err)
	}
	if removed.PID != 10 || len(world.Players()) != 1 {
		t.Fatalf("removed player = %+v, players = %+v", removed, world.Players())
	}
}

func TestUnityWorldBroadcastCanExcludeConnection(t *testing.T) {
	world := gcore.NewUnityWorld()
	first := newUnityWorldTestConnection(0)
	second := newUnityWorldTestConnection(1)
	if _, _, err := world.AddPlayer(first); err != nil {
		t.Fatalf("AddPlayer() first error = %v", err)
	}
	if _, _, err := world.AddPlayer(second); err != nil {
		t.Fatalf("AddPlayer() second error = %v", err)
	}

	if errorsFound := world.BroadcastExcept(first.GetConnId(), gcore.UnityMsgBroadCast, []byte("broadcast")); len(errorsFound) != 0 {
		t.Fatalf("Broadcast() errors = %v", errorsFound)
	}
	select {
	case <-first.messages:
		t.Fatal("Broadcast() sent a message to the excluded connection")
	default:
	}
	select {
	case message := <-second.messages:
		if message.msgID != gcore.UnityMsgBroadCast || string(message.data) != "broadcast" {
			t.Fatalf("message = %+v", message)
		}
	default:
		t.Fatal("Broadcast() did not send a message to the active connection")
	}
}

var _ gface.IConnection = (*unityWorldTestConnection)(nil)
