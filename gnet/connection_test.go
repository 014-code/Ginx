package gnet

import (
	"io"
	"net"
	"testing"
	"time"

	"Ginx/gface"
)

type replyRouter struct {
	responseID   uint32
	responseData []byte
	requests     chan string
	errors       chan error
}

func (r *replyRouter) PreHandle(gface.IRequest) {}

func (r *replyRouter) Handle(request gface.IRequest) {
	r.requests <- string(request.GetData())
	if err := request.GetConnection().SendMsg(r.responseID, r.responseData); err != nil {
		r.errors <- err
	}
}

func (r *replyRouter) PostHandle(gface.IRequest) {}

func TestConnectionRoutesAndRepliesOverTCP(t *testing.T) {
	listener, err := net.ListenTCP("tcp4", &net.TCPAddr{IP: net.IPv4(127, 0, 0, 1)})
	if err != nil {
		t.Fatalf("ListenTCP() error = %v", err)
	}
	defer listener.Close()

	accepted := make(chan *net.TCPConn, 1)
	acceptErrors := make(chan error, 1)
	go func() {
		conn, acceptErr := listener.AcceptTCP()
		if acceptErr != nil {
			acceptErrors <- acceptErr
			return
		}
		accepted <- conn
	}()

	client, err := net.DialTCP("tcp4", nil, listener.Addr().(*net.TCPAddr))
	if err != nil {
		t.Fatalf("DialTCP() error = %v", err)
	}
	defer client.Close()

	var serverConn *net.TCPConn
	select {
	case serverConn = <-accepted:
	case err = <-acceptErrors:
		t.Fatalf("AcceptTCP() error = %v", err)
	case <-time.After(time.Second):
		t.Fatal("timed out waiting for accepted connection")
	}
	defer serverConn.Close()

	handler := NewMsgHandle()
	router0 := &replyRouter{
		responseID:   10,
		responseData: []byte("move accepted"),
		requests:     make(chan string, 1),
		errors:       make(chan error, 1),
	}
	router1 := &replyRouter{
		responseID:   11,
		responseData: []byte("chat accepted"),
		requests:     make(chan string, 1),
		errors:       make(chan error, 1),
	}
	handler.AddRouter(1, router0)
	handler.AddRouter(2, router1)

	connection := NewConntion(serverConn, 42, handler)
	startDone := make(chan struct{})
	go func() {
		connection.Start()
		close(startDone)
	}()

	writeTestMessage(t, client, 1, []byte("move"))
	assertReply(t, client, 10, []byte("move accepted"))
	select {
	case got := <-router0.requests:
		if got != "move" {
			t.Fatalf("router 0 request = %q, want %q", got, "move")
		}
	case err := <-router0.errors:
		t.Fatalf("router 0 SendMsg() error = %v", err)
	case <-time.After(time.Second):
		t.Fatal("timed out waiting for router 0")
	}

	writeTestMessage(t, client, 2, []byte("hello"))
	assertReply(t, client, 11, []byte("chat accepted"))
	select {
	case got := <-router1.requests:
		if got != "hello" {
			t.Fatalf("router 1 request = %q, want %q", got, "hello")
		}
	case err := <-router1.errors:
		t.Fatalf("router 1 SendMsg() error = %v", err)
	case <-time.After(time.Second):
		t.Fatal("timed out waiting for router 1")
	}

	client.Close()
	select {
	case <-startDone:
	case <-time.After(time.Second):
		t.Fatal("connection did not stop after the client disconnected")
	}
}

func writeTestMessage(t *testing.T, conn net.Conn, msgID uint32, data []byte) {
	t.Helper()
	packet, err := NewDataPack().Pack(NewMsgPackage(msgID, data))
	if err != nil {
		t.Fatalf("Pack() error = %v", err)
	}
	if _, err = conn.Write(packet); err != nil {
		t.Fatalf("Write() error = %v", err)
	}
}

func assertReply(t *testing.T, conn net.Conn, wantID uint32, wantData []byte) {
	t.Helper()
	if err := conn.SetReadDeadline(time.Now().Add(time.Second)); err != nil {
		t.Fatalf("SetReadDeadline() error = %v", err)
	}

	header := make([]byte, NewDataPack().GetHeadLen())
	if _, err := io.ReadFull(conn, header); err != nil {
		t.Fatalf("ReadFull() header error = %v", err)
	}
	message, err := NewDataPack().Unpack(header)
	if err != nil {
		t.Fatalf("Unpack() error = %v", err)
	}
	data := make([]byte, message.GetDataLen())
	if _, err = io.ReadFull(conn, data); err != nil {
		t.Fatalf("ReadFull() body error = %v", err)
	}
	if message.GetMsgID() != wantID || string(data) != string(wantData) {
		t.Fatalf("reply = id:%d data:%q, want id:%d data:%q", message.GetMsgID(), data, wantID, wantData)
	}
}
