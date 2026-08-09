package test

import (
	"Ginx/gface"
	"Ginx/gnet"
	"Ginx/utils"
	"net"
	"testing"
	"time"
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

func TestConnectionUsesWorkerPool(t *testing.T) {
	setWorkerConfig(t, 2, 4)
	handler := gnet.NewMsgHandle()
	handler.AddRouter(1, newReplyRouter(10, "move accepted"))
	handler.StartWorkerPool()
	t.Cleanup(handler.StopWorkerPool)

	client, startDone := startTestConnection(t, handler)
	writeTestMessage(t, client, 1, []byte("move"))
	assertReply(t, client, 10, []byte("move accepted"))

	client.Close()
	waitForConnectionStop(t, startDone)
}

func TestConnectionHandlesFragmentedPacket(t *testing.T) {
	setWorkerConfig(t, 1, 4)
	handler := gnet.NewMsgHandle()
	router := newRecordingRouter(1)
	handler.AddRouter(1, router)
	handler.StartWorkerPool()
	t.Cleanup(handler.StopWorkerPool)

	client, startDone := startTestConnection(t, handler)
	packet, err := gnet.NewDataPack().Pack(gnet.NewMsgPackage(1, []byte("fragmented")))
	if err != nil {
		t.Fatalf("Pack() error = %v", err)
	}
	for _, part := range packet {
		if _, err := client.Write([]byte{part}); err != nil {
			t.Fatalf("Write() error = %v", err)
		}
	}

	select {
	case data := <-router.requests:
		if data != "fragmented" {
			t.Fatalf("request data = %q, want fragmented", data)
		}
	case <-time.After(time.Second):
		t.Fatal("timed out waiting for fragmented packet")
	}
	client.Close()
	waitForConnectionStop(t, startDone)
}

func TestConnectionHandlesStickyPacketsInOrder(t *testing.T) {
	setWorkerConfig(t, 1, 4)
	handler := gnet.NewMsgHandle()
	router := newRecordingRouter(2)
	handler.AddRouter(1, router)
	handler.StartWorkerPool()
	t.Cleanup(handler.StopWorkerPool)

	client, startDone := startTestConnection(t, handler)
	pack := gnet.NewDataPack()
	first, err := pack.Pack(gnet.NewMsgPackage(1, []byte("first")))
	if err != nil {
		t.Fatalf("Pack() first error = %v", err)
	}
	second, err := pack.Pack(gnet.NewMsgPackage(1, []byte("second")))
	if err != nil {
		t.Fatalf("Pack() second error = %v", err)
	}
	if _, err := client.Write(append(first, second...)); err != nil {
		t.Fatalf("Write() error = %v", err)
	}

	for _, want := range []string{"first", "second"} {
		select {
		case data := <-router.requests:
			if data != want {
				t.Fatalf("request data = %q, want %q", data, want)
			}
		case <-time.After(time.Second):
			t.Fatalf("timed out waiting for %q packet", want)
		}
	}
	client.Close()
	waitForConnectionStop(t, startDone)
}

func TestConnectionFallsBackWithoutWorkerPool(t *testing.T) {
	setWorkerConfig(t, 0, 4)
	handler := gnet.NewMsgHandle()
	handler.AddRouter(1, newReplyRouter(10, "fallback accepted"))

	client, startDone := startTestConnection(t, handler)
	writeTestMessage(t, client, 1, []byte("fallback"))
	assertReply(t, client, 10, []byte("fallback accepted"))

	client.Close()
	waitForConnectionStop(t, startDone)
}

func TestConnectionSendsBufferedMessage(t *testing.T) {
	setWorkerConfig(t, 0, 4)
	setMessageBufferConfig(t, 2)
	handler := gnet.NewMsgHandle()
	client, connection, startDone := startTestConnectionWithConnection(t, handler)

	if err := connection.SendBuffMsg(10, []byte("buffered reply")); err != nil {
		t.Fatalf("SendBuffMsg() error = %v", err)
	}
	assertReply(t, client, 10, []byte("buffered reply"))

	client.Close()
	waitForConnectionStop(t, startDone)
}

func TestConnectionPreservesBufferedMessageOrder(t *testing.T) {
	setWorkerConfig(t, 0, 4)
	setMessageBufferConfig(t, 2)
	handler := gnet.NewMsgHandle()
	client, connection, startDone := startTestConnectionWithConnection(t, handler)

	for _, data := range []string{"first buffered", "second buffered"} {
		if err := connection.SendBuffMsg(10, []byte(data)); err != nil {
			t.Fatalf("SendBuffMsg() error = %v", err)
		}
	}
	assertReply(t, client, 10, []byte("first buffered"))
	assertReply(t, client, 10, []byte("second buffered"))

	client.Close()
	waitForConnectionStop(t, startDone)
}

func TestConnectionBufferedMessageReturnsWhenQueueIsFull(t *testing.T) {
	setMessageBufferConfig(t, 1)
	client, server := newTCPPair(t)
	connection := gnet.NewConntion(server, 42, gnet.NewMsgHandle())
	defer client.Close()
	defer server.Close()

	if err := connection.SendBuffMsg(10, []byte("first")); err != nil {
		t.Fatalf("SendBuffMsg() first error = %v", err)
	}

	result := make(chan error, 1)
	go func() {
		result <- connection.SendBuffMsg(10, []byte("second"))
	}()

	select {
	case err := <-result:
		if err == nil || err.Error() != "Connection outbound msg queue is full" {
			t.Fatalf("SendBuffMsg() error = %v, want queue full error", err)
		}
	case <-time.After(time.Second):
		t.Fatal("SendBuffMsg() blocked when the buffer queue was full")
	}
}

func TestConnectionPreservesMixedOutboundMessageOrder(t *testing.T) {
	setWorkerConfig(t, 0, 4)
	setMessageBufferConfig(t, 4)
	client, connection, startDone := startTestConnectionWithConnection(t, gnet.NewMsgHandle())

	if err := connection.SendMsg(10, []byte("first")); err != nil {
		t.Fatalf("SendMsg() error = %v", err)
	}
	if err := connection.SendBuffMsg(10, []byte("second")); err != nil {
		t.Fatalf("SendBuffMsg() error = %v", err)
	}
	if err := connection.SendMsg(10, []byte("third")); err != nil {
		t.Fatalf("SendMsg() second error = %v", err)
	}

	assertReply(t, client, 10, []byte("first"))
	assertReply(t, client, 10, []byte("second"))
	assertReply(t, client, 10, []byte("third"))
	client.Close()
	waitForConnectionStop(t, startDone)
}

func TestConnectionBufferedMessageRejectsAfterStop(t *testing.T) {
	setMessageBufferConfig(t, 1)
	client, server := newTCPPair(t)
	connection := gnet.NewConntion(server, 42, gnet.NewMsgHandle())
	defer client.Close()
	defer server.Close()

	connection.Stop()
	if err := connection.SendBuffMsg(10, []byte("closed")); err == nil || err.Error() != "Connection closed when send buff msg" {
		t.Fatalf("SendBuffMsg() after Stop() error = %v, want closed error", err)
	}
}

func TestConnectionClosesWhenWorkerQueueIsFull(t *testing.T) {
	setWorkerConfig(t, 1, 1)
	utils.GlobalObject.WorkerTaskQueueWaitTime = 20
	handler := gnet.NewMsgHandle()
	router := &blockingRouter{
		entered: make(chan struct{}, 1),
		release: make(chan struct{}),
	}
	defer close(router.release)
	handler.AddRouter(1, router)
	handler.StartWorkerPool()
	t.Cleanup(handler.StopWorkerPool)

	client, startDone := startTestConnection(t, handler)
	writeTestMessage(t, client, 1, []byte("running"))
	select {
	case <-router.entered:
	case <-time.After(time.Second):
		t.Fatal("timed out waiting for the blocking worker")
	}

	writeTestMessage(t, client, 1, []byte("queued"))
	writeTestMessage(t, client, 1, []byte("rejected"))
	waitForConnectionStop(t, startDone)
}

func TestConnectionStopsAfterHeartbeatTimeout(t *testing.T) {
	setWorkerConfig(t, 0, 4)
	utils.GlobalObject.HeartbeatMax = 1
	handler := gnet.NewMsgHandle()
	client, startDone := startTestConnection(t, handler)

	select {
	case <-startDone:
	case <-time.After(2 * time.Second):
		client.Close()
		t.Fatal("connection did not stop after heartbeat timeout")
	}
}

func TestConnectionRefreshesHeartbeatAfterMessage(t *testing.T) {
	setWorkerConfig(t, 0, 4)
	utils.GlobalObject.HeartbeatMax = 1
	handler := gnet.NewMsgHandle()
	client, startDone := startTestConnection(t, handler)

	time.Sleep(500 * time.Millisecond)
	writeTestMessage(t, client, 99, []byte("heartbeat"))
	time.Sleep(700 * time.Millisecond)
	select {
	case <-startDone:
		t.Fatal("connection timed out before heartbeat was refreshed")
	default:
	}

	client.Close()
	waitForConnectionStop(t, startDone)
}

func TestConnectionKeepsAliveWhenHeartbeatIsDisabled(t *testing.T) {
	setWorkerConfig(t, 0, 4)
	utils.GlobalObject.HeartbeatMax = 0
	handler := gnet.NewMsgHandle()
	client, startDone := startTestConnection(t, handler)

	time.Sleep(1200 * time.Millisecond)
	select {
	case <-startDone:
		t.Fatal("connection stopped while heartbeat timeout was disabled")
	default:
	}

	client.Close()
	waitForConnectionStop(t, startDone)
}

func newReplyRouter(responseID uint32, responseData string) *replyRouter {
	return &replyRouter{
		responseID:   responseID,
		responseData: []byte(responseData),
		requests:     make(chan string, 1),
		errors:       make(chan error, 1),
	}
}

func waitForConnectionStop(t *testing.T, startDone chan struct{}) {
	t.Helper()
	select {
	case <-startDone:
	case <-time.After(time.Second):
		t.Fatal("connection did not stop after the client disconnected")
	}
}

func startTestConnectionWithConnection(t *testing.T, handler *gnet.MsgHandle) (*net.TCPConn, *gnet.Connection, chan struct{}) {
	t.Helper()
	client, server := newTCPPair(t)
	connection := gnet.NewConntion(server, 42, handler)
	startDone := make(chan struct{})
	go func() {
		connection.Start()
		close(startDone)
	}()

	t.Cleanup(func() {
		client.Close()
		select {
		case <-startDone:
		case <-time.After(time.Second):
			connection.Stop()
			select {
			case <-startDone:
			case <-time.After(time.Second):
			}
		}
		server.Close()
	})

	return client, connection, startDone
}

type recordingRouter struct {
	gnet.BaseRouter
	requests chan string
}

func newRecordingRouter(size int) *recordingRouter {
	return &recordingRouter{requests: make(chan string, size)}
}

func (r *recordingRouter) Handle(request gface.IRequest) {
	r.requests <- string(request.GetData())
}
