package gnet

import "Ginx/gface"

// 实现router时，先嵌入这个基类，然后根据需要对这个基类的方法进行重写
type BaseRouter struct{}

// 前置方法
func (b BaseRouter) PreHandle(request gface.IRequest) {}

// 业务处理方法
func (b BaseRouter) Handle(request gface.IRequest) {}

// 后置方法
func (b BaseRouter) PostHandle(request gface.IRequest) {}
