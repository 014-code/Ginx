package test

import (
	"Ginx/gface"
	"Ginx/gnet"
	"testing"
	"time"
)

type workerRouter struct {
	requests chan string
}

func (r *workerRouter) PreHandle(gface.IRequest) {}

func (r *workerRouter) Handle(request gface.IRequest) {
	r.requests <- string(request.GetData())
}

func (r *workerRouter) PostHandle(gface.IRequest) {}

func TestWorkerPoolProcessesRequests(t *testing.T) {
	setWorkerConfig(t, 2, 4)
	handler := gnet.NewMsgHandle()
	router := &workerRouter{requests: make(chan string, 1)}
	handler.AddRouter(7, router)
	handler.StartWorkerPool()

	if handler.WorkerPoolSize != 2 {
		t.Fatalf("WorkerPoolSize = %d, want %d", handler.WorkerPoolSize, 2)
	}
	if len(handler.TaskQueue) != 2 {
		t.Fatalf("TaskQueue length = %d, want %d", len(handler.TaskQueue), 2)
	}

	handler.SendMsgToTaskQueue(&testRequest{
		connection: &testConnection{id: 3},
		msgID:      7,
		data:       []byte("login"),
	})

	select {
	case got := <-router.requests:
		if got != "login" {
			t.Fatalf("worker request = %q, want %q", got, "login")
		}
	case <-time.After(time.Second):
		t.Fatal("timed out waiting for worker request")
	}
}

func TestWorkerPoolPreservesOrderForOneConnection(t *testing.T) {
	setWorkerConfig(t, 3, 8)
	handler := gnet.NewMsgHandle()
	router := &workerRouter{requests: make(chan string, 3)}
	handler.AddRouter(7, router)
	handler.StartWorkerPool()

	connection := &testConnection{id: 5}
	for _, data := range []string{"first", "second", "third"} {
		handler.SendMsgToTaskQueue(&testRequest{
			connection: connection,
			msgID:      7,
			data:       []byte(data),
		})
	}

	for _, want := range []string{"first", "second", "third"} {
		select {
		case got := <-router.requests:
			if got != want {
				t.Fatalf("worker request = %q, want %q", got, want)
			}
		case <-time.After(time.Second):
			t.Fatalf("timed out waiting for %q", want)
		}
	}
}

func TestMessageHandlerFallsBackWithoutWorkerPool(t *testing.T) {
	setWorkerConfig(t, 0, 0)
	handler := gnet.NewMsgHandle()
	router := &workerRouter{requests: make(chan string, 1)}
	handler.AddRouter(7, router)

	handler.SendMsgToTaskQueue(&testRequest{
		connection: &testConnection{id: 1},
		msgID:      7,
		data:       []byte("fallback"),
	})

	select {
	case got := <-router.requests:
		if got != "fallback" {
			t.Fatalf("fallback request = %q, want %q", got, "fallback")
		}
	case <-time.After(time.Second):
		t.Fatal("timed out waiting for fallback request")
	}
}
