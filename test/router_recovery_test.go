package test

import (
	"Ginx/gface"
	"Ginx/gnet"
	"Ginx/metrics"
	"testing"
)

type panicRouter struct {
	gnet.BaseRouter
	postCalled chan struct{}
}

func (router *panicRouter) Handle(gface.IRequest) {
	panic("router test panic")
}

func (router *panicRouter) PostHandle(gface.IRequest) {
	close(router.postCalled)
}

func TestMsgHandlerRecoversRouterPanic(t *testing.T) {
	handler := gnet.NewMsgHandle()
	collector := &metrics.Metrics{}
	handler.SetMetrics(collector)
	panicRouter := &panicRouter{postCalled: make(chan struct{})}
	handler.AddRouter(7, panicRouter)
	type panicInfo struct {
		msgID     uint32
		recovered interface{}
		stack     []byte
	}
	panicData := make(chan panicInfo, 1)
	handler.SetPanicHandler(func(request gface.IRequest, recovered interface{}, stack []byte) {
		panicData <- panicInfo{msgID: request.GetMsgID(), recovered: recovered, stack: stack}
	})

	handler.DoMsgHandler(&testRequest{connection: &testConnection{id: 1}, msgID: 7})
	select {
	case data := <-panicData:
		if data.msgID != 7 || data.recovered != "router test panic" || len(data.stack) == 0 {
			t.Fatalf("panic callback data = %+v", data)
		}
	default:
		t.Fatal("panic callback was not called")
	}
	select {
	case <-panicRouter.postCalled:
		t.Fatal("PostHandle was called after Handle panic")
	default:
	}
	if snapshot := collector.Snapshot(); snapshot.RouterPanics != 1 {
		t.Fatalf("router panic metrics = %+v", snapshot)
	}
}
