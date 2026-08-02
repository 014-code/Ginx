package test

import (
	"Ginx/gface"
	"Ginx/gnet"
	"Ginx/utils"
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

type blockingRouter struct {
	entered chan struct{}
	release chan struct{}
}

func (r *blockingRouter) PreHandle(gface.IRequest) {}

func (r *blockingRouter) Handle(gface.IRequest) {
	r.entered <- struct{}{}
	<-r.release
}

func (r *blockingRouter) PostHandle(gface.IRequest) {}

type drainingRouter struct {
	firstStarted chan struct{}
	release      chan struct{}
	handled      chan string
}

func (r *drainingRouter) PreHandle(gface.IRequest) {}

func (r *drainingRouter) Handle(request gface.IRequest) {
	data := string(request.GetData())
	if data == "first" {
		r.firstStarted <- struct{}{}
		<-r.release
	}
	r.handled <- data
}

func (r *drainingRouter) PostHandle(gface.IRequest) {}

func TestWorkerPoolProcessesRequests(t *testing.T) {
	setWorkerConfig(t, 2, 4)
	handler := gnet.NewMsgHandle()
	router := &workerRouter{requests: make(chan string, 1)}
	handler.AddRouter(7, router)
	handler.StartWorkerPool()
	t.Cleanup(handler.StopWorkerPool)

	if handler.WorkerPoolSize != 2 {
		t.Fatalf("WorkerPoolSize = %d, want %d", handler.WorkerPoolSize, 2)
	}
	if len(handler.TaskQueue) != 2 {
		t.Fatalf("TaskQueue length = %d, want %d", len(handler.TaskQueue), 2)
	}

	if err := handler.SendMsgToTaskQueue(&testRequest{
		connection: &testConnection{id: 3},
		msgID:      7,
		data:       []byte("login"),
	}); err != nil {
		t.Fatalf("SendMsgToTaskQueue() error = %v", err)
	}

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
	t.Cleanup(handler.StopWorkerPool)

	connection := &testConnection{id: 5}
	for _, data := range []string{"first", "second", "third"} {
		if err := handler.SendMsgToTaskQueue(&testRequest{
			connection: connection,
			msgID:      7,
			data:       []byte(data),
		}); err != nil {
			t.Fatalf("SendMsgToTaskQueue() error = %v", err)
		}
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

	if err := handler.SendMsgToTaskQueue(&testRequest{
		connection: &testConnection{id: 1},
		msgID:      7,
		data:       []byte("fallback"),
	}); err != nil {
		t.Fatalf("SendMsgToTaskQueue() error = %v", err)
	}

	select {
	case got := <-router.requests:
		if got != "fallback" {
			t.Fatalf("fallback request = %q, want %q", got, "fallback")
		}
	case <-time.After(time.Second):
		t.Fatal("timed out waiting for fallback request")
	}
}

func TestWorkerPoolRejectsWhenQueueIsFull(t *testing.T) {
	setWorkerConfig(t, 1, 1)
	utils.GlobalObject.WorkerTaskQueueWaitTime = 20
	handler := gnet.NewMsgHandle()
	router := &blockingRouter{
		entered: make(chan struct{}, 1),
		release: make(chan struct{}),
	}
	handler.AddRouter(7, router)
	handler.StartWorkerPool()
	t.Cleanup(handler.StopWorkerPool)

	connection := &testConnection{id: 0}
	if err := handler.SendMsgToTaskQueue(&testRequest{
		connection: connection,
		msgID:      7,
		data:       []byte("running"),
	}); err != nil {
		t.Fatalf("SendMsgToTaskQueue() error = %v", err)
	}

	select {
	case <-router.entered:
	case <-time.After(time.Second):
		t.Fatal("timed out waiting for the blocking worker")
	}
	if err := handler.SendMsgToTaskQueue(&testRequest{
		connection: connection,
		msgID:      7,
		data:       []byte("queued"),
	}); err != nil {
		t.Fatalf("SendMsgToTaskQueue() error = %v", err)
	}

	err := handler.SendMsgToTaskQueue(&testRequest{
		connection: connection,
		msgID:      7,
		data:       []byte("rejected"),
	})
	if err == nil {
		t.Fatal("SendMsgToTaskQueue() accepted a full queue")
	}
	if err.Error() != "worker task queue wait timeout" {
		t.Fatalf("SendMsgToTaskQueue() error = %q, want queue wait timeout", err)
	}
	close(router.release)
}

func TestWorkerPoolDrainsQueuedRequestsOnStop(t *testing.T) {
	setWorkerConfig(t, 1, 2)
	handler := gnet.NewMsgHandle()
	router := &drainingRouter{
		firstStarted: make(chan struct{}, 1),
		release:      make(chan struct{}),
		handled:      make(chan string, 2),
	}
	handler.AddRouter(7, router)
	handler.StartWorkerPool()
	t.Cleanup(handler.StopWorkerPool)

	connection := &testConnection{id: 0}
	for _, data := range []string{"first", "second"} {
		if err := handler.SendMsgToTaskQueue(&testRequest{
			connection: connection,
			msgID:      7,
			data:       []byte(data),
		}); err != nil {
			t.Fatalf("SendMsgToTaskQueue() error = %v", err)
		}
	}

	select {
	case <-router.firstStarted:
	case <-time.After(time.Second):
		t.Fatal("timed out waiting for the first request")
	}

	stopped := make(chan struct{})
	go func() {
		handler.StopWorkerPool()
		close(stopped)
	}()

	select {
	case <-stopped:
		t.Fatal("StopWorkerPool() returned before the active task completed")
	case <-time.After(50 * time.Millisecond):
	}

	close(router.release)
	for _, want := range []string{"first", "second"} {
		select {
		case got := <-router.handled:
			if got != want {
				t.Fatalf("handled request = %q, want %q", got, want)
			}
		case <-time.After(time.Second):
			t.Fatalf("timed out waiting for %q to drain", want)
		}
	}
	select {
	case <-stopped:
	case <-time.After(time.Second):
		t.Fatal("StopWorkerPool() did not return after draining queued requests")
	}
}
