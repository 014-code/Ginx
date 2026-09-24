package test

import (
	"Ginx/gface"
	"Ginx/gnet"
	"Ginx/utils"
	"context"
	"errors"
	"fmt"
	"net"
	"sync"
	"testing"
	"time"
)

var _ gface.ContextConnection = (*gnet.Connection)(nil)
var _ gface.ShutdownServer = (*gnet.Server)(nil)

func TestSendCancellationAndNonblockingContention(t *testing.T) {
	setMessageBufferConfig(t, 1)
	client, socket := newTCPPair(t)
	c := gnet.NewConntion(socket, 1, gnet.NewMsgHandle())
	t.Cleanup(func() { c.Stop(); client.Close() })
	if err := c.SendBuffMsg(1, []byte("first")); err != nil {
		t.Fatal(err)
	}
	waiting := make(chan error, 1)
	go func() { waiting <- c.SendMsg(2, []byte("blocked")) }()
	select {
	case err := <-waiting:
		t.Fatalf("full queue did not block: %v", err)
	case <-time.After(20 * time.Millisecond):
	}
	result := make(chan error, 1)
	go func() { result <- c.SendBuffMsg(3, nil) }()
	select {
	case err := <-result:
		if !errors.Is(err, gnet.ErrSendQueueFull) {
			t.Fatal(err)
		}
	case <-time.After(time.Second):
		t.Fatal("nonblocking send waited behind blocked sender")
	}
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Millisecond)
	defer cancel()
	if err := c.SendMsgContext(ctx, 4, nil); !errors.Is(err, context.DeadlineExceeded) {
		t.Fatal(err)
	}
	c.Stop()
	select {
	case err := <-waiting:
		if !errors.Is(err, gnet.ErrConnectionClosed) {
			t.Fatal(err)
		}
	case <-time.After(time.Second):
		t.Fatal("stop did not release blocked sender")
	}
	if err := c.SendBuffMsg(5, nil); !errors.Is(err, gnet.ErrConnectionClosed) {
		t.Fatal(err)
	}
}

func TestSendTimeoutDoesNotEnqueue(t *testing.T) {
	setMessageBufferConfig(t, 1)
	utils.GlobalObject.SendTimeout = 30
	client, socket := newTCPPair(t)
	c := gnet.NewConntion(socket, 1, gnet.NewMsgHandle())
	t.Cleanup(func() { c.Stop(); client.Close() })
	if err := c.SendMsg(1, []byte("kept")); err != nil {
		t.Fatal(err)
	}
	if err := c.SendMsg(2, []byte("expired")); !errors.Is(err, context.DeadlineExceeded) {
		t.Fatal(err)
	}
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	if err := c.SendMsgContext(ctx, 3, nil); !errors.Is(err, context.Canceled) {
		t.Fatal(err)
	}
	done := make(chan struct{})
	go func() { c.StartWriter(); close(done) }()
	t.Cleanup(func() { c.Stop(); awaitDone(t, done) })
	assertReply(t, client, 1, []byte("kept"))
	if err := client.SetReadDeadline(time.Now().Add(50 * time.Millisecond)); err != nil {
		t.Fatal(err)
	}
	if _, err := client.Read(make([]byte, 1)); err == nil {
		t.Fatal("canceled message was sent")
	} else if e, ok := err.(net.Error); !ok || !e.Timeout() {
		t.Fatal(err)
	}
}

func TestWriteTimeoutClosesSlowClient(t *testing.T) {
	config := gnet.DefaultConfig()
	config.Host, config.TcpPort, config.HeartbeatMax = "127.0.0.1", 0, 0
	config.MaxPacketSize, config.MaxMsgChanLen, config.WriteTimeout = 8<<20, 1, 40
	server := gnet.NewServerWithConfig(config)
	connected := make(chan *gnet.Connection, 1)
	server.SetOnConnStart(func(c gface.IConnection) {
		connection := c.(*gnet.Connection)
		if err := connection.GetTCPConnection().SetWriteBuffer(1024); err != nil {
			t.Error(err)
		}
		connected <- connection
	})
	if err := server.StartWithError(); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(server.Stop)
	client, err := net.DialTimeout("tcp4", server.Addr().String(), time.Second)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { client.Close() })
	if err := client.(*net.TCPConn).SetReadBuffer(1024); err != nil {
		t.Fatal(err)
	}
	var c *gnet.Connection
	select {
	case c = <-connected:
	case <-time.After(time.Second):
		t.Fatal("missing connection")
	}
	if err := c.SendMsg(1, make([]byte, 8<<20)); err != nil {
		t.Fatal(err)
	}
	select {
	case <-c.Context().Done():
	case <-time.After(3 * time.Second):
		t.Fatal("slow reader was not disconnected by write timeout")
	}
}

type cancelTestRouter struct {
	gnet.BaseRouter
	entered     chan context.Context
	release     chan struct{}
	cooperative bool
}

func (r *cancelTestRouter) Handle(req gface.IRequest) {
	ctx := gface.RequestContext(req)
	r.entered <- ctx
	if r.cooperative {
		<-ctx.Done()
	} else {
		<-r.release
	}
}

func TestShutdownDeadlineAndConcurrentWaiters(t *testing.T) {
	for _, workers := range []uint32{0, 1} {
		t.Run(fmt.Sprint(workers), func(t *testing.T) {
			config := gnet.DefaultConfig()
			config.Host, config.TcpPort, config.WorkerPoolSize, config.HeartbeatMax = "127.0.0.1", 0, workers, 0
			s := gnet.NewServerWithConfig(config)
			router := &cancelTestRouter{entered: make(chan context.Context, 1), release: make(chan struct{})}
			var release sync.Once
			t.Cleanup(func() { release.Do(func() { close(router.release) }); s.Stop() })
			s.AddRouter(1, router)
			if err := s.StartWithError(); err != nil {
				t.Fatal(err)
			}
			client, err := net.DialTimeout("tcp4", s.Addr().String(), time.Second)
			if err != nil {
				t.Fatal(err)
			}
			t.Cleanup(func() { client.Close() })
			packet, _ := gnet.NewDataPackWithLimit(64).Pack(gnet.NewMsgPackage(1, nil))
			if err := client.SetWriteDeadline(time.Now().Add(time.Second)); err != nil {
				t.Fatal(err)
			}
			if _, err := client.Write(packet); err != nil {
				t.Fatal(err)
			}
			var requestCtx context.Context
			select {
			case requestCtx = <-router.entered:
			case <-time.After(time.Second):
				t.Fatal("router did not start")
			}
			ctx, cancel := context.WithTimeout(context.Background(), 40*time.Millisecond)
			defer cancel()
			if err := s.Shutdown(ctx); !errors.Is(err, context.DeadlineExceeded) {
				t.Fatalf("uncompleted route: %v", err)
			}
			if requestCtx.Err() != context.Canceled {
				t.Fatal("request was not canceled")
			}
			results := make(chan error, 3)
			for i := 0; i < 3; i++ {
				go func() {
					ctx, cancel := context.WithTimeout(context.Background(), time.Second)
					defer cancel()
					results <- s.Shutdown(ctx)
				}()
			}
			select {
			case err := <-results:
				t.Fatalf("returned before route completed: %v", err)
			case <-time.After(20 * time.Millisecond):
			}
			release.Do(func() { close(router.release) })
			for i := 0; i < 3; i++ {
				select {
				case err := <-results:
					if err != nil {
						t.Fatal(err)
					}
				case <-time.After(2 * time.Second):
					t.Fatal("cleanup did not finish")
				}
			}
			if err := s.StartWithError(); !errors.Is(err, net.ErrClosed) {
				t.Fatal("stopped server restarted", err)
			}
		})
	}
}

func TestWorkerStopInterruptsQueueWait(t *testing.T) {
	setWorkerConfig(t, 1, 1)
	utils.GlobalObject.WorkerTaskQueueWaitTime = 30000
	mh := gnet.NewMsgHandle()
	r := &drainingRouter{firstStarted: make(chan struct{}, 1), release: make(chan struct{}), handled: make(chan string, 2)}
	var release sync.Once
	t.Cleanup(func() { release.Do(func() { close(r.release) }); mh.StopWorkerPool() })
	mh.AddRouter(1, r)
	mh.StartWorkerPool()
	request := func(data string) *testRequest {
		return &testRequest{connection: &testConnection{id: 1}, msgID: 1, data: []byte(data)}
	}
	if err := mh.SendMsgToTaskQueue(request("first")); err != nil {
		t.Fatal(err)
	}
	select {
	case <-r.firstStarted:
	case <-time.After(time.Second):
		t.Fatal("worker not started")
	}
	if err := mh.SendMsgToTaskQueue(request("second")); err != nil {
		t.Fatal(err)
	}
	result := make(chan error, 1)
	go func() { result <- mh.SendMsgToTaskQueue(request("third")) }()
	select {
	case err := <-result:
		t.Fatal("queue not full", err)
	case <-time.After(20 * time.Millisecond):
	}
	done := make(chan struct{})
	go func() { mh.StopWorkerPool(); close(done) }()
	select {
	case err := <-result:
		if !errors.Is(err, gnet.ErrWorkerStopped) {
			t.Fatal(err)
		}
	case <-time.After(time.Second):
		t.Fatal("stop waited for queue timeout")
	}
	release.Do(func() { close(r.release) })
	awaitDone(t, done)
	for _, want := range []string{"first", "second"} {
		select {
		case got := <-r.handled:
			if got != want {
				t.Fatal(got)
			}
		case <-time.After(time.Second):
			t.Fatal("queued task not drained")
		}
	}
	if err := mh.SendMsgToTaskQueue(request("after stop")); !errors.Is(err, gnet.ErrWorkerStopped) {
		t.Fatal(err)
	}
}

func TestCooperativeRouteCancellation(t *testing.T) {
	for _, workers := range []uint32{0, 1} {
		for _, disconnect := range []bool{false, true} {
			t.Run(fmt.Sprintf("workers=%d/disconnect=%t", workers, disconnect), func(t *testing.T) {
				config := gnet.DefaultConfig()
				config.Host, config.TcpPort, config.WorkerPoolSize, config.HeartbeatMax = "127.0.0.1", 0, workers, 0
				s := gnet.NewServerWithConfig(config)
				r := &cancelTestRouter{entered: make(chan context.Context, 1), cooperative: true}
				s.AddRouter(1, r)
				if err := s.StartWithError(); err != nil {
					t.Fatal(err)
				}
				t.Cleanup(s.Stop)
				client, err := net.DialTimeout("tcp4", s.Addr().String(), time.Second)
				if err != nil {
					t.Fatal(err)
				}
				t.Cleanup(func() { client.Close() })
				packet, err := gnet.NewDataPackWithLimit(64).Pack(gnet.NewMsgPackage(1, nil))
				if err != nil {
					t.Fatal(err)
				}
				if err := client.SetWriteDeadline(time.Now().Add(time.Second)); err != nil {
					t.Fatal(err)
				}
				if _, err := client.Write(packet); err != nil {
					t.Fatal(err)
				}
				var requestCtx context.Context
				select {
				case requestCtx = <-r.entered:
				case <-time.After(time.Second):
					t.Fatal("route did not start")
				}
				if disconnect {
					client.Close()
					select {
					case <-requestCtx.Done():
					case <-time.After(time.Second):
						t.Fatal("disconnect did not cancel route")
					}
				}
				ctx, cancel := context.WithTimeout(context.Background(), time.Second)
				defer cancel()
				if err := s.Shutdown(ctx); err != nil {
					t.Fatal("cooperative route did not exit", err)
				}
				if requestCtx.Err() != context.Canceled {
					t.Fatal("request not canceled")
				}
			})
		}
	}
}

func TestShutdownBeforeStart(t *testing.T) {
	s := gnet.NewServerWithConfig(gnet.DefaultConfig())
	ctx, cancel := context.WithTimeout(context.Background(), time.Second)
	defer cancel()
	if err := s.Shutdown(ctx); err != nil {
		t.Fatal(err)
	}
	if err := s.Shutdown(ctx); err != nil {
		t.Fatal("repeated shutdown", err)
	}
	if err := s.StartWithError(); !errors.Is(err, net.ErrClosed) {
		t.Fatal(err)
	}
}

func awaitDone(t *testing.T, done <-chan struct{}) {
	t.Helper()
	select {
	case <-done:
	case <-time.After(2 * time.Second):
		t.Fatal("operation did not complete")
	}
}
