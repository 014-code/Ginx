package gnet

import (
	"reflect"
	"testing"

	"Ginx/gface"
)

type recordingRouter struct {
	stages []string
	data   []string
}

func (r *recordingRouter) PreHandle(gface.IRequest) {
	r.stages = append(r.stages, "pre")
}

func (r *recordingRouter) Handle(request gface.IRequest) {
	r.stages = append(r.stages, "handle")
	r.data = append(r.data, string(request.GetData()))
}

func (r *recordingRouter) PostHandle(gface.IRequest) {
	r.stages = append(r.stages, "post")
}

func TestMsgHandleRoutesByMessageID(t *testing.T) {
	handler := NewMsgHandle()
	router := &recordingRouter{}
	handler.AddRouter(7, router)

	request := &Request{data: NewMsgPackage(7, []byte("move player"))}
	handler.DoMsgHandler(request)

	if got, want := router.stages, []string{"pre", "handle", "post"}; !reflect.DeepEqual(got, want) {
		t.Fatalf("router stages = %v, want %v", got, want)
	}
	if got, want := router.data, []string{"move player"}; !reflect.DeepEqual(got, want) {
		t.Fatalf("router data = %v, want %v", got, want)
	}
}

func TestMsgHandleRejectsDuplicateMessageID(t *testing.T) {
	handler := NewMsgHandle()
	handler.AddRouter(7, &recordingRouter{})

	defer func() {
		if recover() == nil {
			t.Fatal("AddRouter() did not reject a duplicate message id")
		}
	}()
	handler.AddRouter(7, &recordingRouter{})
}
