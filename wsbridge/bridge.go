// Package wsbridge 将浏览器二进制 WebSocket 消息桥接到固定的 Ginx TCP 服务。
// 不处理身份、路由或玩法；每个 WebSocket 对应一个真实 TCP 连接。
package wsbridge

import (
	"Ginx/gnet"
	"bytes"
	"context"
	"errors"
	"io"
	"net"
	"net/http"
	"net/url"
	"sync"
	"time"

	"github.com/gorilla/websocket"
)

type Options struct {
	Upstream       string
	MaxPacketSize  uint32
	MaxConnections int
	IdleTimeout    time.Duration
	WriteTimeout   time.Duration
	// 除同源外允许的精确 Origin，不支持通配符，也不信任转发头。
	AllowedOrigins []string
}

type Bridge struct {
	options  Options
	pack     *gnet.DataPack
	upgrader websocket.Upgrader
	mu       sync.Mutex
	closed   bool
	next     uint64
	active   map[uint64]context.CancelFunc
	wg       sync.WaitGroup
}

// New 只校验和组装配置，不拨号、不启动协程。
func New(options Options) (*Bridge, error) {
	if _, _, err := net.SplitHostPort(options.Upstream); err != nil {
		return nil, errors.New("upstream must be a fixed host:port")
	}
	if options.MaxPacketSize == 0 {
		options.MaxPacketSize = 64 << 10
	}
	if options.MaxConnections == 0 {
		options.MaxConnections = 64
	}
	if options.IdleTimeout == 0 {
		options.IdleTimeout = 20 * time.Second
	}
	if options.WriteTimeout == 0 {
		options.WriteTimeout = 3 * time.Second
	}
	if options.MaxPacketSize > 16<<20 || options.MaxConnections < 1 || options.IdleTimeout < 0 || options.WriteTimeout < 0 {
		return nil, errors.New("invalid bridge limits")
	}
	origins := make(map[string]bool)
	for _, value := range options.AllowedOrigins {
		u, err := url.Parse(value)
		if err != nil || (u.Scheme != "http" && u.Scheme != "https") || u.Host == "" || u.User != nil || u.Path != "" || u.RawQuery != "" || u.Fragment != "" {
			return nil, errors.New("origins must be exact http(s) origins")
		}
		origins[value] = true
	}
	b := &Bridge{options: options, pack: gnet.NewDataPackWithLimit(options.MaxPacketSize), active: make(map[uint64]context.CancelFunc)}
	b.upgrader = websocket.Upgrader{HandshakeTimeout: 3 * time.Second, ReadBufferSize: 4096, WriteBufferSize: 4096, CheckOrigin: func(r *http.Request) bool {
		values := r.Header.Values("Origin")
		if len(values) != 1 {
			return false
		}
		origin := values[0]
		if origins[origin] {
			return true
		}
		u, err := url.Parse(origin)
		scheme := "http"
		if r.TLS != nil {
			scheme = "https"
		}
		return err == nil && u.Scheme == scheme && u.Host == r.Host && u.User == nil && u.Path == "" && u.RawQuery == "" && u.Fragment == ""
	}}
	return b, nil
}

func (b *Bridge) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet || !websocket.IsWebSocketUpgrade(r) {
		http.Error(w, "WebSocket upgrade required", http.StatusBadRequest)
		return
	}
	if !b.upgrader.CheckOrigin(r) {
		http.Error(w, "origin rejected", http.StatusForbidden)
		return
	}
	ctx, cancel := context.WithCancel(r.Context())
	defer cancel()
	b.mu.Lock()
	if b.closed || len(b.active) >= b.options.MaxConnections {
		b.mu.Unlock()
		http.Error(w, "bridge unavailable", http.StatusServiceUnavailable)
		return
	}
	id := b.next
	b.next++
	b.active[id] = cancel
	b.wg.Add(1)
	b.mu.Unlock()
	defer func() { b.mu.Lock(); delete(b.active, id); b.mu.Unlock(); b.wg.Done() }()
	upstream, err := (&net.Dialer{Timeout: 3 * time.Second}).DialContext(ctx, "tcp", b.options.Upstream)
	if err != nil {
		http.Error(w, "upstream unavailable", http.StatusBadGateway)
		return
	}
	defer upstream.Close()
	ws, err := b.upgrader.Upgrade(w, r, nil)
	if err != nil {
		return
	}
	defer ws.Close()
	stop := context.AfterFunc(ctx, func() { upstream.Close(); ws.Close() })
	defer stop()
	ws.SetReadLimit(int64(b.options.MaxPacketSize) + 8)
	// 只允许一个 reader 和一个 writer；不累积无界转发队列。
	done := make(chan struct{})
	go func() {
		defer close(done)
		defer ws.Close()
		for {
			_ = upstream.SetReadDeadline(time.Now().Add(b.options.IdleTimeout))
			msg, err := b.pack.ReadMessage(upstream)
			if err != nil {
				return
			}
			packet, err := b.pack.Pack(msg)
			if err != nil {
				return
			}
			_ = ws.SetWriteDeadline(time.Now().Add(b.options.WriteTimeout))
			if ws.WriteMessage(websocket.BinaryMessage, packet) != nil {
				return
			}
		}
	}()
	defer func() { upstream.Close(); ws.Close(); <-done }()
	for {
		_ = ws.SetReadDeadline(time.Now().Add(b.options.IdleTimeout))
		kind, data, err := ws.ReadMessage()
		if err != nil {
			return
		}
		reader := bytes.NewReader(data)
		_, packetErr := b.pack.ReadMessage(reader)
		if kind != websocket.BinaryMessage || packetErr != nil || reader.Len() != 0 {
			_ = ws.WriteControl(websocket.CloseMessage, websocket.FormatCloseMessage(websocket.ClosePolicyViolation, "one binary Ginx packet required"), time.Now().Add(time.Second))
			return
		}
		_ = upstream.SetWriteDeadline(time.Now().Add(b.options.WriteTimeout))
		if _, err := io.Copy(upstream, bytes.NewReader(data)); err != nil {
			return
		}
	}
}

// Close 拒绝新请求、取消拨号并关闭所有隧道，等待转发协程退出。
// 必须由拥有者显式调用；http.Server.Shutdown 不管理已劫持的 WebSocket。
func (b *Bridge) Close() error {
	b.mu.Lock()
	b.closed = true
	for _, cancel := range b.active {
		cancel()
	}
	b.mu.Unlock()
	b.wg.Wait()
	return nil
}
