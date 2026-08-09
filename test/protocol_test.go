package test

import (
	"Ginx/gcore"
	"encoding/binary"
	"testing"
)

func TestGameMessageEncodeAndDecode(t *testing.T) {
	message := gcore.NewGameMessage(gcore.GameMsgPlayerMove, 8, 1001, 2001, []byte("move payload"))
	message.Flags = gcore.GameMessageFlagReliable | gcore.GameMessageFlagResponse

	data, err := message.Encode()
	if err != nil {
		t.Fatalf("Encode() error = %v", err)
	}
	decoded, err := gcore.DecodeGameMessage(data)
	if err != nil {
		t.Fatalf("DecodeGameMessage() error = %v", err)
	}
	if decoded.MessageID != message.MessageID || decoded.Sequence != message.Sequence || decoded.PlayerID != message.PlayerID || decoded.RoomID != message.RoomID || string(decoded.Payload) != string(message.Payload) {
		t.Fatalf("decoded message = %+v, want %+v", decoded, message)
	}
	if decoded.Flags != message.Flags || decoded.Version != gcore.GameProtocolVersion {
		t.Fatalf("decoded flags/version = %d/%d, want %d/%d", decoded.Flags, decoded.Version, message.Flags, gcore.GameProtocolVersion)
	}
}

func TestGameMessageRejectsInvalidData(t *testing.T) {
	if _, err := gcore.DecodeGameMessage([]byte{1, 2, 3}); err == nil {
		t.Fatal("DecodeGameMessage() accepted an incomplete header")
	}

	message := gcore.NewGameMessage(gcore.GameMsgLoginRequest, 1, 2, 3, []byte("login"))
	data, err := message.Encode()
	if err != nil {
		t.Fatalf("Encode() error = %v", err)
	}
	data[0] = 0
	if _, err := gcore.DecodeGameMessage(data); err == nil {
		t.Fatal("DecodeGameMessage() accepted an invalid magic")
	}

	data, err = message.Encode()
	if err != nil {
		t.Fatalf("Encode() second error = %v", err)
	}
	binary.LittleEndian.PutUint32(data[20:24], gcore.MaxGamePayloadSize+1)
	if _, err := gcore.DecodeGameMessage(data); err == nil {
		t.Fatal("DecodeGameMessage() accepted an oversized payload")
	}
}
