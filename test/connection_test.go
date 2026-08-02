package test

import (
	"Ginx/gface"
	"Ginx/gnet"
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
