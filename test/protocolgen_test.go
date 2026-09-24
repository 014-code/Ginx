package test

import (
	"Ginx/examples/gameapp"
	"Ginx/examples/gameapp/protocol"
	"Ginx/gnet"
	"bytes"
	"encoding/binary"
	"encoding/json"
	"go/format"
	"os"
	"testing"
)

// 固定既有公开消息编号，防止迁移 schema 时意外改变协议。
func TestGeneratedProtocolIDs(t *testing.T) {
	got := []uint32{gameapp.MsgAuthenticate, gameapp.MsgHeartbeat, gameapp.MsgJoinRoom,
		gameapp.MsgLeaveRoom, gameapp.MsgProgress, gameapp.MsgStarterReward, gameapp.MsgUseItem}
	want := []uint32{1001, 1002, 2001, 2002, 3001, 3002, 3003}
	for i := range want {
		if got[i] != want[i] {
			t.Fatalf("message %d: got %d, want %d", i, got[i], want[i])
		}
	}
	if protocol.ProtocolVersion != 1 || protocol.SchemaVersion != 1 {
		t.Fatal("unexpected protocol metadata version")
	}
}

func TestGeneratedProtocolJSONAndFraming(t *testing.T) {
	cases := []struct {
		id   uint32
		body any
		want string
	}{
		{protocol.MsgAuthenticate, protocol.AuthenticateRequest{Token: "example"}, `{"token":"example"}`},
		{protocol.MsgJoinRoom, protocol.JoinRoomRequest{RoomID: 7}, `{"room_id":7}`},
		{protocol.MsgUseItem, protocol.UseItemRequest{ItemID: "potion"}, `{"item_id":"potion"}`},
		{protocol.MsgHeartbeat, protocol.HeartbeatRequest{}, `{}`},
		{protocol.MsgLeaveRoom, protocol.LeaveRoomRequest{}, `{}`},
		{protocol.MsgProgress, protocol.ProgressRequest{}, `{}`},
		{protocol.MsgStarterReward, protocol.StarterRewardRequest{}, `{}`},
	}
	pack := gnet.NewDataPackWithLimit(4096)
	for _, item := range cases {
		data, err := json.Marshal(item.body)
		if err != nil || string(data) != item.want {
			t.Fatalf("id %d: JSON %s, error %v", item.id, data, err)
		}
		wire, err := pack.Pack(gnet.NewMsgPackage(item.id, data))
		if err != nil {
			t.Fatal(err)
		}
		if binary.LittleEndian.Uint32(wire[:4]) != uint32(len(data)) || binary.LittleEndian.Uint32(wire[4:8]) != item.id {
			t.Fatal("generated metadata changed transport header")
		}
		message, err := pack.ReadMessage(bytes.NewReader(wire))
		if err != nil || message.GetMsgID() != item.id || !bytes.Equal(message.GetData(), data) {
			t.Fatalf("framing round trip: %v", err)
		}
	}
}

func TestGeneratedProtocolResponseCompatibility(t *testing.T) {
	for _, data := range []any{nil, map[string]uint64{"player_id": 9007199254740993}} {
		wire, err := json.Marshal(gameapp.Reply{Code: "ok", Data: data})
		if err != nil {
			t.Fatal(err)
		}
		var reply protocol.AuthenticateResponse
		if err := json.Unmarshal(wire, &reply); err != nil || reply.Code != "ok" {
			t.Fatalf("generated response failed: %v", err)
		}
		roundTrip, err := json.Marshal(reply)
		if err != nil || !bytes.Equal(roundTrip, wire) {
			t.Fatalf("response JSON changed: %s -> %s (%v)", wire, roundTrip, err)
		}
	}
}

func TestGeneratedProtocolIsGofmtFormatted(t *testing.T) {
	data, err := os.ReadFile("../examples/gameapp/protocol/protocol.gen.go")
	if err != nil {
		t.Fatal(err)
	}
	data = bytes.ReplaceAll(data, []byte("\r\n"), []byte("\n"))
	formatted, err := format.Source(data)
	if err != nil || !bytes.Equal(formatted, data) {
		t.Fatalf("generator must emit gofmt-stable code: %v", err)
	}
}
