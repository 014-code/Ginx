package gnet

import (
	"bytes"
	"encoding/binary"
	"testing"

	"Ginx/utils"
)

func TestDataPackPackAndUnpack(t *testing.T) {
	pack := NewDataPack()
	want := NewMsgPackage(1001, []byte("login request"))

	data, err := pack.Pack(want)
	if err != nil {
		t.Fatalf("Pack() error = %v", err)
	}

	if got, wantLen := uint32(len(data)), pack.GetHeadLen()+want.GetDataLen(); got != wantLen {
		t.Fatalf("packet length = %d, want %d", got, wantLen)
	}
	if got := binary.LittleEndian.Uint32(data[0:4]); got != want.GetDataLen() {
		t.Fatalf("encoded data length = %d, want %d", got, want.GetDataLen())
	}
	if got := binary.LittleEndian.Uint32(data[4:8]); got != want.GetMsgID() {
		t.Fatalf("encoded message id = %d, want %d", got, want.GetMsgID())
	}

	got, err := pack.Unpack(data[:pack.GetHeadLen()])
	if err != nil {
		t.Fatalf("Unpack() error = %v", err)
	}
	if got.GetDataLen() != want.GetDataLen() || got.GetMsgID() != want.GetMsgID() {
		t.Fatalf("unpacked header = len:%d id:%d, want len:%d id:%d", got.GetDataLen(), got.GetMsgID(), want.GetDataLen(), want.GetMsgID())
	}

	got.SetData(want.GetData())
	if !bytes.Equal(got.GetData(), want.GetData()) {
		t.Fatalf("unpacked data = %q, want %q", got.GetData(), want.GetData())
	}
}

func TestDataPackRejectsOversizedMessage(t *testing.T) {
	oldMaxPacketSize := utils.GlobalObject.MaxPacketSize
	utils.GlobalObject.MaxPacketSize = 4
	defer func() {
		utils.GlobalObject.MaxPacketSize = oldMaxPacketSize
	}()

	header := make([]byte, NewDataPack().GetHeadLen())
	binary.LittleEndian.PutUint32(header[0:4], 5)
	binary.LittleEndian.PutUint32(header[4:8], 7)

	if _, err := NewDataPack().Unpack(header); err == nil {
		t.Fatal("Unpack() accepted a packet larger than MaxPacketSize")
	}
}
