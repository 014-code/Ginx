package test

import (
	"Ginx/gnet"
	"Ginx/utils"
	"bytes"
	"encoding/binary"
	"io"
	"testing"
)

func TestDataPackPackAndUnpack(t *testing.T) {
	pack := gnet.NewDataPack()
	want := gnet.NewMsgPackage(1001, []byte("login request"))

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

	header := make([]byte, gnet.NewDataPack().GetHeadLen())
	binary.LittleEndian.PutUint32(header[0:4], 5)
	binary.LittleEndian.PutUint32(header[4:8], 7)

	if _, err := gnet.NewDataPack().Unpack(header); err == nil {
		t.Fatal("Unpack() accepted a packet larger than MaxPacketSize")
	}
}

func TestDataPackReadMessageHandlesFragmentedAndStickyPackets(t *testing.T) {
	pack := gnet.NewDataPack()
	first, err := pack.Pack(gnet.NewMsgPackage(1, []byte("first")))
	if err != nil {
		t.Fatalf("Pack() first error = %v", err)
	}
	second, err := pack.Pack(gnet.NewMsgPackage(2, []byte("second")))
	if err != nil {
		t.Fatalf("Pack() second error = %v", err)
	}

	reader := &chunkReader{
		data: append(first, second...),
		step: 1,
	}
	firstMessage, err := pack.ReadMessage(reader)
	if err != nil {
		t.Fatalf("ReadMessage() first error = %v", err)
	}
	secondMessage, err := pack.ReadMessage(reader)
	if err != nil {
		t.Fatalf("ReadMessage() second error = %v", err)
	}
	if firstMessage.GetMsgID() != 1 || string(firstMessage.GetData()) != "first" {
		t.Fatalf("first message = id:%d data:%q", firstMessage.GetMsgID(), firstMessage.GetData())
	}
	if secondMessage.GetMsgID() != 2 || string(secondMessage.GetData()) != "second" {
		t.Fatalf("second message = id:%d data:%q", secondMessage.GetMsgID(), secondMessage.GetData())
	}
}

func TestDataPackReadMessageHandlesZeroLengthPayload(t *testing.T) {
	pack := gnet.NewDataPack()
	data, err := pack.Pack(gnet.NewMsgPackage(99, nil))
	if err != nil {
		t.Fatalf("Pack() error = %v", err)
	}

	message, err := pack.ReadMessage(bytes.NewReader(data))
	if err != nil {
		t.Fatalf("ReadMessage() error = %v", err)
	}
	if message.GetMsgID() != 99 || message.GetDataLen() != 0 || message.GetData() == nil {
		t.Fatalf("message = id:%d len:%d data:%v", message.GetMsgID(), message.GetDataLen(), message.GetData())
	}
}

func TestDataPackRejectsInvalidOutboundMessage(t *testing.T) {
	pack := gnet.NewDataPack()
	if _, err := pack.Pack(nil); err == nil {
		t.Fatal("Pack() accepted a nil message")
	}
	if _, err := pack.Unpack(make([]byte, pack.GetHeadLen()+1)); err == nil {
		t.Fatal("Unpack() accepted a header with an invalid length")
	}
}

type chunkReader struct {
	data []byte
	step int
	read int
}

func (r *chunkReader) Read(data []byte) (int, error) {
	if r.read >= len(r.data) {
		return 0, io.EOF
	}
	n := r.step
	if n > len(data) {
		n = len(data)
	}
	if n > len(r.data)-r.read {
		n = len(r.data) - r.read
	}
	copy(data[:n], r.data[r.read:r.read+n])
	r.read += n
	return n, nil
}
