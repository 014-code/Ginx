package gnet

import (
	"Ginx/gface"
	"Ginx/metrics"
	"Ginx/utils"
	"context"
	"fmt"
	"net"
	"sync"
	"sync/atomic"
)

type Server struct {
	Name      string
	IPVersion string
	IP        string
	Port      int
	config    *Config

	msgHandler *MsgHandle
	connMgr    *ConnManager

	listenerLock sync.Mutex
	listener     *net.TCPListener
	startOnce    sync.Once
	startErr     error
	stopOnce     sync.Once
	stopChan     chan struct{}
	stopped      atomic.Bool
	connWait     sync.WaitGroup
	shutdownOnce sync.Once
	shutdownDone chan struct{}

	hookLock    sync.RWMutex
	onConnStart gface.HookFunc
	onConnStop  gface.HookFunc
	metrics     *metrics.Metrics
}

func (s *Server) AddRouter(msgId uint32, router gface.IRouter) {
	s.msgHandler.AddRouter(msgId, router)
}

func (s *Server) SetOnConnStart(hookFunc gface.HookFunc) {
	s.hookLock.Lock()
	defer s.hookLock.Unlock()
	s.onConnStart = hookFunc
}

func (s *Server) SetOnConnStop(hookFunc gface.HookFunc) {
	s.hookLock.Lock()
	defer s.hookLock.Unlock()
	s.onConnStop = hookFunc
}

func (s *Server) GetConnMgr() gface.IConnManager {
	return s.connMgr
}

// GetMetrics 返回当前服务运行指标快照。
func (s *Server) GetMetrics() metrics.Snapshot {
	if s == nil || s.metrics == nil {
		return metrics.Snapshot{}
	}
	return s.metrics.Snapshot()
}

func (s *Server) Start() {
	if err := s.StartWithError(); err != nil {
		fmt.Println("start server error:", err)
	}
}

// StartWithError 在返回前绑定监听端口，供多入口应用处理启动失败。
func (s *Server) StartWithError() error {
	s.startOnce.Do(func() {
		s.listenerLock.Lock()
		defer s.listenerLock.Unlock()
		if s.stopped.Load() {
			s.startErr = net.ErrClosed
			return
		}
		addr, err := net.ResolveTCPAddr(s.IPVersion, net.JoinHostPort(s.IP, fmt.Sprint(s.Port)))
		if err == nil {
			s.listener, err = net.ListenTCP(s.IPVersion, addr)
		}
		if err != nil {
			s.startErr = err
			s.signalStop()
			return
		}
		s.msgHandler.StartWorkerPool()
		s.connWait.Add(1)
		go s.startAccept(s.listener)
	})
	if s.startErr != nil {
		return s.startErr
	}
	if s.stopped.Load() {
		return net.ErrClosed
	}
	return nil
}

// Addr 返回实际监听地址，支持 Port 为 0 时获取系统分配的端口。
func (s *Server) Addr() net.Addr {
	s.listenerLock.Lock()
	defer s.listenerLock.Unlock()
	if s.listener == nil {
		return nil
	}
	return s.listener.Addr()
}

func (s *Server) startAccept(listener *net.TCPListener) {
	defer s.connWait.Done()
	fmt.Println("start Zinx server  ", s.Name, " succ, now listenning...")
	var cid uint32

	for {
		conn, err := listener.AcceptTCP()
		if err != nil {
			if s.stopped.Load() {
				return
			}
			fmt.Println("Accept err ", err)
			continue
		}

		if s.stopped.Load() {
			conn.Close()
			return
		}

		connection := newConnection(conn, cid, s.msgHandler, s.config)
		connection.setMetrics(s.metrics)
		cid++
		s.listenerLock.Lock()
		if s.stopped.Load() {
			s.listenerLock.Unlock()
			conn.Close()
			return
		}
		if err := s.connMgr.Add(connection); err != nil {
			s.listenerLock.Unlock()
			fmt.Println("Add connection error: ", err)
			conn.Close()
			continue
		}
		s.metrics.AddConnections(1)

		s.connWait.Add(1)
		s.listenerLock.Unlock()
		go s.startConnection(connection)
	}
}

func (s *Server) startConnection(connection *Connection) {
	defer s.connWait.Done()
	// 写协程先启动以支持欢迎消息，读取必须等待初始化 Hook 完成。
	connection.startWriter()
	s.hookLock.RLock()
	startHook := s.onConnStart
	s.hookLock.RUnlock()
	if startHook != nil {
		startHook(connection)
	}
	connection.startReader()

	connection.wait()

	s.hookLock.RLock()
	stopHook := s.onConnStop
	s.hookLock.RUnlock()
	if stopHook != nil {
		stopHook(connection)
	}
	s.connMgr.Remove(connection.GetConnId())
	s.metrics.AddConnections(-1)
}

func (s *Server) signalStop() {
	s.stopOnce.Do(func() {
		s.stopped.Store(true)
		close(s.stopChan)
		fmt.Println("[STOP] Zinx server , name ", s.Name)
	})
}

func (s *Server) Stop() {
	_ = s.Shutdown(context.Background())
}

// Shutdown 发起停服，等待所有网络任务、Hook 及已接收路由完成。
// 超时只结束本次等待，后台清理继续；无法强制终止不响应取消的业务代码。
func (s *Server) Shutdown(ctx context.Context) error {
	s.shutdownOnce.Do(func() {
		s.signalStop()
		s.msgHandler.beginStop()
		go s.shutdown()
	})
	select {
	case <-s.shutdownDone:
		return nil
	default:
	}
	select {
	case <-s.shutdownDone:
		return nil
	case <-ctx.Done():
		return ctx.Err()
	}
}

func (s *Server) shutdown() {
	defer close(s.shutdownDone)
	s.listenerLock.Lock()
	listener := s.listener
	s.listener = nil
	s.listenerLock.Unlock()
	if listener != nil {
		listener.Close()
	}

	s.connMgr.ClearConn()
	s.connWait.Wait()
	s.msgHandler.StopWorkerPool()
}

func (s *Server) Serve() {
	s.Start()
	<-s.stopChan
}

// NewServer 保留旧配置文件搜索和全局配置行为。
//
// Deprecated: 使用 NewServerWithConfig，由应用显式加载配置。
func NewServer() gface.IServer {
	utils.GlobalObject.Reload()
	return newServer(nil)
}

// NewServerWithConfig 创建独立配置的服务器，不读取文件、不修改全局状态。
func NewServerWithConfig(config Config) *Server {
	return newServer(&config)
}

func newServer(config *Config) *Server {
	settings := effectiveConfig(config)
	serverMetrics := &metrics.Metrics{}
	messageHandler := newMsgHandle(config)
	messageHandler.SetMetrics(serverMetrics)

	s := &Server{
		Name:         settings.Name,
		IPVersion:    "tcp4",
		IP:           settings.Host,
		Port:         settings.TcpPort,
		config:       config,
		msgHandler:   messageHandler,
		connMgr:      newConnManager(config),
		stopChan:     make(chan struct{}),
		shutdownDone: make(chan struct{}),
		metrics:      serverMetrics,
	}
	return s
}
