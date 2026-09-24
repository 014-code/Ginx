package main

import (
	"Ginx/examples/webgame/server"
	"Ginx/gnet"
	"Ginx/wsbridge"
	"context"
	"errors"
	"flag"
	"fmt"
	"log"
	"net"
	"net/http"
	"os"
	"os/signal"
	"strings"
	"syscall"
	"time"
)

func main() {
	addr := flag.String("http", "127.0.0.1:8090", "HTTP/WebSocket listen address")
	staticDir := flag.String("static", "examples/webgame/frontend/dist", "built frontend directory")
	origin := flag.String("origins", "", "comma separated additional exact browser origins for local development")
	health := flag.Bool("healthcheck", false, "check local container HTTP readiness and exit")
	flag.Parse()
	if *health {
		client := http.Client{Timeout: 2 * time.Second}
		r, err := client.Get("http://127.0.0.1:8090/healthz")
		if err != nil {
			os.Exit(1)
		}
		r.Body.Close()
		if r.StatusCode != 200 {
			os.Exit(1)
		}
		return
	}
	ctx, cancel := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer cancel()
	if err := run(ctx, *addr, *staticDir, *origin); err != nil {
		log.Fatal(err)
	}
}

func run(ctx context.Context, addr, staticDir, origin string) error {
	config := gnet.DefaultConfig()
	config.Name = "GinxWebArena"
	config.Host = "127.0.0.1"
	config.TcpPort = 0
	config.MaxConn = 64
	config.MaxPacketSize = server.MaxPacketSize
	config.MaxMsgChanLen = 64
	config.WorkerPoolSize = 4
	config.MaxWorkerTaskLen = 128
	config.WorkerTaskQueueWaitTime = 50
	config.HeartbeatMax = 15
	config.WriteTimeout = 3000
	config.SendTimeout = 1000
	config.MessageRateLimit = 40
	config.MessageRateBurst = 60
	tcp := gnet.NewServerWithConfig(config)
	world := server.NewWorld()
	world.Register(tcp)
	if err := tcp.StartWithError(); err != nil {
		return err
	}
	defer tcp.Stop()
	defer world.Close()
	var origins []string
	if origin != "" {
		for _, v := range strings.Split(origin, ",") {
			origins = append(origins, strings.TrimSpace(v))
		}
	}
	bridge, err := wsbridge.New(wsbridge.Options{Upstream: tcp.Addr().String(), MaxPacketSize: server.MaxPacketSize, AllowedOrigins: origins})
	if err != nil {
		return err
	}
	defer bridge.Close()
	listener, err := net.Listen("tcp", addr)
	if err != nil {
		return err
	}
	defer listener.Close()
	runCtx, stop := context.WithCancel(ctx)
	doneWorld := make(chan struct{})
	go func() { defer close(doneWorld); world.Run(runCtx) }()
	defer func() { stop(); <-doneWorld }()
	httpServer := &http.Server{Handler: server.NewHTTPHandler(world, tcp, bridge, staticDir), ReadHeaderTimeout: 5 * time.Second, ReadTimeout: 10 * time.Second, WriteTimeout: 10 * time.Second, IdleTimeout: 30 * time.Second, MaxHeaderBytes: 16 << 10}
	done := make(chan error, 1)
	go func() { done <- httpServer.Serve(listener) }()
	log.Printf("Web arena http://%s (TCP upstream %s, guests/in-memory scores, local demo only)", listener.Addr(), tcp.Addr())
	select {
	case err := <-done:
		_ = httpServer.Close()
		if !errors.Is(err, http.ErrServerClosed) {
			return fmt.Errorf("HTTP serve: %w", err)
		}
		return nil
	case <-ctx.Done():
		_ = bridge.Close()
		shutdownCtx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer cancel()
		err := httpServer.Shutdown(shutdownCtx)
		if err != nil {
			_ = httpServer.Close()
		}
		<-done
		return err
	}
}
