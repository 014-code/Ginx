package gameapp

import (
	"Ginx/gnet"
	"context"
	"errors"
	"fmt"
	"net"
	"net/http"
	"time"
)

// Serve 同时管理两个监听入口。接管 listener，任何退出路径都会关闭它。
// TCP 路由应事先通过 RegisterTCP 注册；取消 ctx 会关闭 HTTP、会话及 TCP。
func Serve(ctx context.Context, listener net.Listener, tcp *gnet.Server, service *Service) error {
	defer listener.Close()
	defer tcp.Stop()
	defer service.Close()
	if err := ctx.Err(); err != nil {
		return err
	}
	if err := tcp.StartWithError(); err != nil {
		return fmt.Errorf("start TCP: %w", err)
	}
	server := &http.Server{Handler: NewHTTPHandler(service), ReadHeaderTimeout: 5 * time.Second,
		ReadTimeout: 10 * time.Second, WriteTimeout: 10 * time.Second, IdleTimeout: 30 * time.Second,
		MaxHeaderBytes: 16 << 10, BaseContext: func(net.Listener) context.Context { return ctx }}
	done := make(chan error, 1)
	go func() { done <- server.Serve(listener) }()
	ticker := time.NewTicker(time.Second)
	defer ticker.Stop()
	for {
		select {
		case <-ctx.Done():
			shutdownCtx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
			err := server.Shutdown(shutdownCtx)
			cancel()
			if err != nil {
				_ = server.Close()
			}
			<-done
			return err
		case err := <-done:
			_ = server.Close()
			if errors.Is(err, http.ErrServerClosed) {
				return nil
			}
			return err
		case now := <-ticker.C:
			service.Sweep(now)
		}
	}
}
