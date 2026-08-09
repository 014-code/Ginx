package gnet

import (
	"Ginx/gface"
	"Ginx/metrics"
	"Ginx/utils"
	"errors"
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

	msgHandler gface.IMsgHandle
	connMgr    *ConnManager

	listenerLock sync.Mutex
	listener     *net.TCPListener
	startOnce    sync.Once
	stopOnce     sync.Once
	stopChan     chan struct{}
	stopped      atomic.Bool
	connWait     sync.WaitGroup

	onConnStart gface.HookFunc
	onConnStop  gface.HookFunc
	metrics     *metrics.Metrics
}

func (s *Server) AddRouter(msgId uint32, router gface.IRouter) {
	s.msgHandler.AddRouter(msgId, router)
}

func (s *Server) SetOnConnStart(hookFunc gface.HookFunc) {
	s.onConnStart = hookFunc
}

func (s *Server) SetOnConnStop(hookFunc gface.HookFunc) {
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

// 当前客户端连接的回调方法
func CallBackToClient(conn *net.TCPConn, data []byte, cnt int) error {
	fmt.Println("[Conn Handle] CallBackToClient ... ")
	if _, err := conn.Write(data[:cnt]); err != nil {
		fmt.Println("write back buf err ", err)
		return errors.New("CallBackToClient error")
	}
	return nil
}

func (s *Server) Start() {
	s.startOnce.Do(func() {
		if s.stopped.Load() {
			return
		}

		fmt.Printf("[START] Server listenner at IP: %s, Port %d, is starting\n", s.IP, s.Port)
		s.msgHandler.StartWorkerPool()
		s.connWait.Add(1)
		go s.startAccept()
	})
}

func (s *Server) startAccept() {
	defer s.connWait.Done()

	addr, err := net.ResolveTCPAddr(s.IPVersion, fmt.Sprintf("%s:%d", s.IP, s.Port))
	if err != nil {
		fmt.Println("resolve tcp addr err: ", err)
		s.signalStop()
		s.msgHandler.StopWorkerPool()
		return
	}

	listener, err := net.ListenTCP(s.IPVersion, addr)
	if err != nil {
		fmt.Println("listen", s.IPVersion, "err", err)
		s.signalStop()
		s.msgHandler.StopWorkerPool()
		return
	}

	s.listenerLock.Lock()
	if s.stopped.Load() {
		s.listenerLock.Unlock()
		listener.Close()
		return
	}
	s.listener = listener
	s.listenerLock.Unlock()

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

		connection := NewConntion(conn, cid, s.msgHandler)
		connection.setMetrics(s.metrics)
		cid++
		if err := s.connMgr.Add(connection); err != nil {
			fmt.Println("Add connection error: ", err)
			conn.Close()
			continue
		}
		s.metrics.AddConnections(1)

		s.connWait.Add(1)
		go s.startConnection(connection)
	}
}

func (s *Server) startConnection(connection *Connection) {
	defer s.connWait.Done()
	connection.startIO()
	if s.onConnStart != nil {
		s.onConnStart(connection)
	}

	connection.wait()

	if s.onConnStop != nil {
		s.onConnStop(connection)
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
	s.signalStop()

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

func NewServer() gface.IServer {
	utils.GlobalObject.Reload()
	serverMetrics := &metrics.Metrics{}
	messageHandler := NewMsgHandle()
	messageHandler.SetMetrics(serverMetrics)

	s := &Server{
		Name:       utils.GlobalObject.Name,
		IPVersion:  "tcp4",
		IP:         utils.GlobalObject.Host,
		Port:       utils.GlobalObject.TcpPort,
		msgHandler: messageHandler,
		connMgr:    NewConnManager(),
		stopChan:   make(chan struct{}),
		metrics:    serverMetrics,
	}
	return s
}
