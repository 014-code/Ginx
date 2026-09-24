package test

import (
	"Ginx/gface"
	"Ginx/gnet"
	"Ginx/utils"
	"sync"
	"testing"
	"time"
)

type initializationRouter struct {
	gnet.BaseRouter
	values chan any
}

func (r *initializationRouter) Handle(request gface.IRequest) {
	value, _ := request.GetConnection().GetProperty("initialized")
	r.values <- value
}

func TestServerInitializesBeforeDispatchAndAllowsHookSend(t *testing.T) {
	original := *utils.GlobalObject
	t.Cleanup(func() { *utils.GlobalObject = original })
	s := gnet.NewServer().(*gnet.Server)
	s.IP, s.Port = "127.0.0.1", 0
	ready := make(chan struct{})
	var release sync.Once
	router := &initializationRouter{values: make(chan any, 1)}
	s.AddRouter(55, router)
	s.SetOnConnStart(func(c gface.IConnection) {
		_ = c.SendMsg(54, []byte("welcome"))
		<-ready
		c.SetProperty("initialized", true)
	})
	if err := s.StartWithError(); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { release.Do(func() { close(ready) }); s.Stop() })
	client := dialTestServer(t, s.Addr().String())
	t.Cleanup(func() { client.Close() })
	assertReply(t, client, 54, []byte("welcome"))
	writeTestMessage(t, client, 55, []byte("early request"))
	select {
	case <-router.values:
		t.Fatal("request dispatched before initialization")
	case <-time.After(30 * time.Millisecond):
	}
	release.Do(func() { close(ready) })
	select {
	case value := <-router.values:
		if value != true {
			t.Fatal("missing initialized state")
		}
	case <-time.After(time.Second):
		t.Fatal("request never dispatched")
	}
}

func TestServerConcurrentHookUpdates(t *testing.T) {
	setServerConfig(t, 2)
	s, address := newTestServer(t, 2)
	var wg sync.WaitGroup
	wg.Add(1)
	go func() {
		defer wg.Done()
		for i := 0; i < 200; i++ {
			s.SetOnConnStart(func(c gface.IConnection) { c.SetProperty("ready", true) })
			s.SetOnConnStop(func(c gface.IConnection) { c.RemoveProperty("ready") })
		}
	}()
	for i := 0; i < 8; i++ {
		client := dialTestServer(t, address)
		client.Close()
	}
	wg.Wait()
	s.Stop()
}
