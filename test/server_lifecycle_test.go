package test

import (
	"Ginx/gface"
	"Ginx/gnet"
	"Ginx/utils"
	"errors"
	"io"
	"net"
	"sync"
	"testing"
	"time"
)

func TestServerRunsLifecycleHooksAndStopsConnections(t *testing.T) {
	setServerConfig(t, 2)
	server, address := newTestServer(t, 2)

	var hookLock sync.Mutex
	started := make([]uint32, 0, 1)
	stopped := make([]uint32, 0, 1)
	startHookDone := make(chan struct{}, 1)
	stopHookDone := make(chan struct{}, 1)
	server.SetOnConnStart(func(connection gface.IConnection) {
		hookLock.Lock()
		started = append(started, connection.GetConnId())
		hookLock.Unlock()
		startHookDone <- struct{}{}
	})
	server.SetOnConnStop(func(connection gface.IConnection) {
		hookLock.Lock()
		stopped = append(stopped, connection.GetConnId())
		hookLock.Unlock()
		stopHookDone <- struct{}{}
	})

	client := dialTestServer(t, address)
	defer client.Close()

	select {
	case <-startHookDone:
	case <-time.After(time.Second):
		t.Fatal("OnConnStart hook was not called")
	}
	waitForConnManagerLen(t, server, 1)

	server.Stop()

	select {
	case <-stopHookDone:
	case <-time.After(time.Second):
		t.Fatal("OnConnStop hook was not called")
	}
	waitForConnManagerLen(t, server, 0)

	hookLock.Lock()
	if len(started) != 1 || len(stopped) != 1 || started[0] != stopped[0] {
		hookLock.Unlock()
		t.Fatalf("lifecycle hooks start=%v stop=%v, want one matching connection ID", started, stopped)
	}
	hookLock.Unlock()

	if err := client.SetReadDeadline(time.Now().Add(time.Second)); err != nil {
		t.Fatalf("SetReadDeadline() error = %v", err)
	}
	_, err := client.Read(make([]byte, 1))
	if err == nil || !errors.Is(err, io.EOF) {
		t.Fatalf("Read() after Server.Stop() error = %v, want EOF", err)
	}
}

func TestServerRejectsConnectionsOverMaxConn(t *testing.T) {
	setServerConfig(t, 1)
	server, address := newTestServer(t, 1)

	first := dialTestServer(t, address)
	defer first.Close()
	waitForConnManagerLen(t, server, 1)

	second, err := net.DialTimeout("tcp4", address, time.Second)
	if err != nil {
		t.Fatalf("DialTimeout() second connection error = %v", err)
	}
	defer second.Close()
	if err := second.SetReadDeadline(time.Now().Add(time.Second)); err != nil {
		t.Fatalf("SetReadDeadline() error = %v", err)
	}
	_, err = second.Read(make([]byte, 1))
	if err == nil || !errors.Is(err, io.EOF) {
		t.Fatalf("Read() from rejected connection error = %v, want EOF", err)
	}

	if got := server.GetConnMgr().Len(); got != 1 {
		t.Fatalf("managed connections = %d, want 1", got)
	}
	server.Stop()
}

func newTestServer(t *testing.T, maxConn int) (*gnet.Server, string) {
	t.Helper()
	listener, err := net.ListenTCP("tcp4", &net.TCPAddr{IP: net.IPv4(127, 0, 0, 1)})
	if err != nil {
		t.Fatalf("ListenTCP() error = %v", err)
	}
	address := listener.Addr().String()
	port := listener.Addr().(*net.TCPAddr).Port
	if err := listener.Close(); err != nil {
		t.Fatalf("Close() error = %v", err)
	}

	server := gnet.NewServer().(*gnet.Server)
	utils.GlobalObject.MaxConn = maxConn
	server.IP = "127.0.0.1"
	server.Port = port
	server.Start()
	t.Cleanup(server.Stop)

	return server, address
}

func dialTestServer(t *testing.T, address string) net.Conn {
	t.Helper()
	deadline := time.Now().Add(time.Second)
	var lastErr error
	for time.Now().Before(deadline) {
		connection, err := net.DialTimeout("tcp4", address, 100*time.Millisecond)
		if err == nil {
			return connection
		}
		lastErr = err
		time.Sleep(10 * time.Millisecond)
	}
	t.Fatalf("DialTimeout() error = %v", lastErr)
	return nil
}

func waitForConnManagerLen(t *testing.T, server *gnet.Server, want int) {
	t.Helper()
	deadline := time.Now().Add(time.Second)
	for time.Now().Before(deadline) {
		if server.GetConnMgr().Len() == want {
			return
		}
		time.Sleep(10 * time.Millisecond)
	}
	t.Fatalf("connection manager length = %d, want %d", server.GetConnMgr().Len(), want)
}
