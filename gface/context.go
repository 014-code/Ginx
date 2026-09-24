package gface

import "context"

// ContextConnection 是可选扩展，旧 IConnection 实现无需增加方法。
type ContextConnection interface {
	IConnection
	Context() context.Context
	SendMsgContext(context.Context, uint32, []byte) error
}

// ShutdownServer 是可选的限时停服扩展。
type ShutdownServer interface {
	IServer
	Shutdown(context.Context) error
}

// RequestContext 返回请求的取消上下文，旧实现回退为 Background。
func RequestContext(request IRequest) context.Context {
	if value, ok := request.(interface{ Context() context.Context }); ok {
		return value.Context()
	}
	return context.Background()
}
