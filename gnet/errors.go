package gnet

import "errors"

var (
	ErrConnectionClosed = errors.New("connection closed")
	ErrSendQueueFull    = errors.New("Connection outbound msg queue is full")
	ErrWorkerStopped    = errors.New("worker pool stopped")
)
