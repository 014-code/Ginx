package test

import (
	"Ginx/gface"
	"Ginx/gnet"
	"Ginx/utils"
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
