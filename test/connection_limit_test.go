package test

import (
	"Ginx/gface"
	"Ginx/gnet"
	"Ginx/utils"
	"testing"
	"time"
)

type noOpRouter struct {
	gnet.BaseRouter
}

func (r *noOpRouter) Handle(gface.IRequest) {}

func TestConnectionStopsWhenInboundRateLimitIsExceeded(t *testing.T) {
	original := *utils.GlobalObject
	t.Cleanup(func() {
		*utils.GlobalObject = original
	})
	utils.GlobalObject.WorkerPoolSize = 0
	utils.GlobalObject.HeartbeatMax = 0
	utils.GlobalObject.MessageRateLimit = 1
	utils.GlobalObject.MessageRateBurst = 1

	handler := gnet.NewMsgHandle()
	handler.AddRouter(88, &noOpRouter{})
	client, startDone := startTestConnection(t, handler)
	writeTestMessage(t, client, 88, []byte("first"))
	writeTestMessage(t, client, 88, []byte("second"))

	select {
	case <-startDone:
	case <-time.After(time.Second):
		t.Fatal("connection did not stop after inbound rate limit was exceeded")
	}
}
