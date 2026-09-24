package test

import (
	webgame "Ginx/examples/webgame/server"
	"Ginx/gnet"
	"Ginx/wsbridge"
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"os"
	"regexp"
	"strings"
	"testing"
	"time"

	"github.com/gorilla/websocket"
)

type webArenaTest struct {
	world  *webgame.World
	tcp    *gnet.Server
	bridge *wsbridge.Bridge
	http   *httptest.Server
}

func startWebArena(t *testing.T) *webArenaTest {
	t.Helper()
	cfg := gnet.DefaultConfig()
	cfg.Host = "127.0.0.1"
	cfg.TcpPort = 0
	cfg.MaxPacketSize = webgame.MaxPacketSize
	cfg.WorkerPoolSize = 2
	cfg.MaxConn = 32
	cfg.MaxMsgChanLen = 64
	cfg.HeartbeatMax = 5
	cfg.WriteTimeout = 1000
	cfg.SendTimeout = 1000
	s := &webArenaTest{world: webgame.NewWorld(), tcp: gnet.NewServerWithConfig(cfg)}
	s.world.Register(s.tcp)
	if err := s.tcp.StartWithError(); err != nil {
		t.Fatal(err)
	}
	b, err := wsbridge.New(wsbridge.Options{Upstream: s.tcp.Addr().String(), MaxPacketSize: webgame.MaxPacketSize, MaxConnections: 16, IdleTimeout: 5 * time.Second})
	if err != nil {
		s.tcp.Stop()
		t.Fatal(err)
	}
	s.bridge = b
	s.http = httptest.NewServer(webgame.NewHTTPHandler(s.world, s.tcp, b, t.TempDir()))
	ctx, cancel := context.WithCancel(context.Background())
	done := make(chan struct{})
	go func() { defer close(done); s.world.Run(ctx) }()
	t.Cleanup(func() {
		cancel()
		s.bridge.Close()
		s.world.Close()
		s.tcp.Stop()
		s.http.Close()
		select {
		case <-done:
		case <-time.After(3 * time.Second):
			t.Error("world did not exit")
		}
	})
	return s
}
func (s *webArenaTest) dial(t *testing.T) *websocket.Conn {
	t.Helper()
	ws, _, err := (&websocket.Dialer{HandshakeTimeout: 2 * time.Second}).Dial("ws"+strings.TrimPrefix(s.http.URL, "http")+"/ws", http.Header{"Origin": {s.http.URL}})
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { ws.Close() })
	return ws
}
func webSend(t *testing.T, c *websocket.Conn, id uint32, value any) {
	t.Helper()
	data, err := json.Marshal(value)
	if err != nil {
		t.Fatal(err)
	}
	packet, err := gnet.NewDataPackWithLimit(webgame.MaxPacketSize).Pack(gnet.NewMsgPackage(id, data))
	if err != nil {
		t.Fatal(err)
	}
	_ = c.SetWriteDeadline(time.Now().Add(2 * time.Second))
	if err := c.WriteMessage(websocket.BinaryMessage, packet); err != nil {
		t.Fatal(err)
	}
}
func webRead(t *testing.T, c *websocket.Conn, id uint32, target any) {
	t.Helper()
	_ = c.SetReadDeadline(time.Now().Add(3 * time.Second))
	for {
		kind, data, err := c.ReadMessage()
		if err != nil {
			t.Fatal(err)
		}
		if kind != websocket.BinaryMessage {
			t.Fatal("nonbinary response")
		}
		reader := bytes.NewReader(data)
		msg, err := gnet.NewDataPackWithLimit(webgame.MaxPacketSize).ReadMessage(reader)
		if err != nil || reader.Len() != 0 {
			t.Fatalf("invalid Ginx packet: %v", err)
		}
		if msg.GetMsgID() != id {
			continue
		}
		if err := json.Unmarshal(msg.GetData(), target); err != nil {
			t.Fatal(err)
		}
		return
	}
}
func (s *webArenaTest) enter(t *testing.T, name string, room uint32) (*websocket.Conn, uint32) {
	t.Helper()
	guest, err := s.world.Guest(name)
	if err != nil {
		t.Fatal(err)
	}
	ws := s.dial(t)
	webSend(t, ws, webgame.MsgAuthenticate, map[string]any{"token": guest.Token})
	var auth struct {
		ID uint32 `json:"id"`
	}
	webRead(t, ws, webgame.MsgAuthenticate, &auth)
	webSend(t, ws, webgame.MsgJoin, map[string]any{"room_id": room})
	var joined map[string]any
	webRead(t, ws, webgame.MsgJoin, &joined)
	return ws, auth.ID
}
func webSnapshot(t *testing.T, c *websocket.Conn, accept func(webgame.Snapshot) bool) webgame.Snapshot {
	t.Helper()
	deadline := time.Now().Add(3 * time.Second)
	for time.Now().Before(deadline) {
		var snapshot webgame.Snapshot
		webRead(t, c, webgame.MsgSnapshot, &snapshot)
		if accept(snapshot) {
			return snapshot
		}
	}
	t.Fatal("snapshot condition timed out")
	return webgame.Snapshot{}
}

func TestWebGameEndToEndRoomsMovementAndDisconnect(t *testing.T) {
	s := startWebArena(t)
	a, idA := s.enter(t, "Alice", 1)
	b, _ := s.enter(t, "Bob", 1)
	c, _ := s.enter(t, "Carol", 2)
	webSnapshot(t, a, func(v webgame.Snapshot) bool { return len(v.Players) == 2 })
	webSnapshot(t, b, func(v webgame.Snapshot) bool { return len(v.Players) == 2 })
	separate := webSnapshot(t, c, func(v webgame.Snapshot) bool { return len(v.Players) == 1 })
	if separate.Players[0].Name != "Carol" {
		t.Fatal("cross-room snapshot leaked")
	}
	// 重复序列号不能覆盖有效输入；分数只由服务端实际碰撞产生。
	for seq := uint32(1); seq <= 6; seq++ {
		webSend(t, a, webgame.MsgInput, map[string]any{"dx": 1, "dy": 0, "seq": seq})
		webSend(t, a, webgame.MsgInput, map[string]any{"dx": -1, "dy": 0, "seq": seq})
		time.Sleep(100 * time.Millisecond)
	}
	webSend(t, a, webgame.MsgInput, map[string]any{"dx": 0, "dy": 0, "seq": 7})
	snapshot := webSnapshot(t, b, func(v webgame.Snapshot) bool {
		for _, p := range v.Players {
			if p.ID == idA && p.Score == 1 {
				return true
			}
		}
		return false
	})
	for _, p := range snapshot.Players {
		if p.ID == idA && (p.X < 220 || p.X > 300) {
			t.Fatalf("invalid server movement: %+v", p)
		}
	}
	a.Close()
	webSnapshot(t, b, func(v webgame.Snapshot) bool { return len(v.Players) == 1 && v.Players[0].Name == "Bob" })
	deadline := time.Now().Add(2 * time.Second)
	for s.tcp.GetMetrics().Connections != 2 && time.Now().Before(deadline) {
		time.Sleep(10 * time.Millisecond)
	}
	m := s.tcp.GetMetrics()
	if m.Connections != 2 || m.Messages < 10 || m.BytesIn == 0 || m.BytesOut == 0 || m.RouterPanics != 0 {
		t.Fatalf("framework metrics not exercised: %+v", m)
	}
}

func TestWebGameAuthInputAndGuestValidation(t *testing.T) {
	s := startWebArena(t)
	c := s.dial(t)
	webSend(t, c, webgame.MsgInput, map[string]any{"dx": 1, "dy": 0, "seq": 1})
	var failure struct {
		Code string `json:"code"`
	}
	webRead(t, c, webgame.MsgError, &failure)
	if failure.Code != "unauthorized" {
		t.Fatal(failure)
	}
	guest, _ := s.world.Guest("Tester")
	webSend(t, c, webgame.MsgAuthenticate, map[string]any{"token": guest.Token})
	var auth map[string]any
	webRead(t, c, webgame.MsgAuthenticate, &auth)
	duplicate := s.dial(t)
	webSend(t, duplicate, webgame.MsgAuthenticate, map[string]any{"token": guest.Token})
	webRead(t, duplicate, webgame.MsgError, &failure)
	if failure.Code != "unauthorized" {
		t.Fatal("bound token reused")
	}
	webSend(t, c, webgame.MsgJoin, map[string]any{"room_id": 1})
	var reply map[string]any
	webRead(t, c, webgame.MsgJoin, &reply)
	for _, body := range []map[string]any{{"dx": 10, "dy": 0, "seq": 1}, {"dx": 0, "dy": 0, "seq": 0}, {"dx": 0, "dy": 0, "seq": 2, "score": 10000}} {
		webSend(t, c, webgame.MsgInput, body)
		webRead(t, c, webgame.MsgError, &failure)
		if failure.Code != "invalid_input" {
			t.Fatal(failure)
		}
	}
	webSend(t, c, webgame.MsgHeartbeat, map[string]any{"sent_at": 1234})
	webRead(t, c, webgame.MsgHeartbeat, &reply)
	if reply["sent_at"] != float64(1234) {
		t.Fatal("heartbeat not routed")
	}
	for _, name := range []string{"", "  ", strings.Repeat("a", 17), "bad\nname", "bad\u202ename"} {
		if _, err := s.world.Guest(name); err == nil {
			t.Fatalf("invalid name accepted: %q", name)
		}
	}
}

func TestWebSocketBridgeRejectsOriginMalformedAndOversize(t *testing.T) {
	s := startWebArena(t)
	for _, origin := range []string{"", "https://evil.invalid", s.http.URL + "/path"} {
		headers := http.Header{}
		if origin != "" {
			headers.Set("Origin", origin)
		}
		c, r, err := websocket.DefaultDialer.Dial("ws"+strings.TrimPrefix(s.http.URL, "http")+"/ws", headers)
		if c != nil {
			c.Close()
		}
		if err == nil || r == nil || r.StatusCode != 403 {
			t.Fatalf("origin %q accepted: %v", origin, err)
		}
		r.Body.Close()
	}
	for _, tc := range []struct {
		name string
		kind int
		data []byte
	}{
		{"text", websocket.TextMessage, []byte("{}")},
		{"short", websocket.BinaryMessage, []byte{1, 2}},
		{"multiple", websocket.BinaryMessage, make([]byte, 16)},
		{"oversize", websocket.BinaryMessage, make([]byte, webgame.MaxPacketSize+9)},
	} {
		t.Run(tc.name, func(t *testing.T) {
			c := s.dial(t)
			_ = c.SetWriteDeadline(time.Now().Add(time.Second))
			if err := c.WriteMessage(tc.kind, tc.data); err != nil {
				t.Fatal(err)
			}
			_ = c.SetReadDeadline(time.Now().Add(2 * time.Second))
			_, _, err := c.ReadMessage()
			if err == nil {
				t.Fatal("invalid packet accepted")
			}
			if !websocket.IsCloseError(err, websocket.ClosePolicyViolation, websocket.CloseMessageTooBig) {
				t.Fatalf("unexpected close: %v", err)
			}
		})
	}
}

func TestWebSocketBridgeCapacityCloseAndUpstreamFailure(t *testing.T) {
	s := startWebArena(t)
	b, err := wsbridge.New(wsbridge.Options{Upstream: s.tcp.Addr().String(), MaxConnections: 1})
	if err != nil {
		t.Fatal(err)
	}
	h := httptest.NewServer(b)
	t.Cleanup(h.Close)
	t.Cleanup(func() { b.Close() })
	headers := http.Header{"Origin": {h.URL}}
	c, _, err := websocket.DefaultDialer.Dial("ws"+strings.TrimPrefix(h.URL, "http"), headers)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { c.Close() })
	other, response, err := websocket.DefaultDialer.Dial("ws"+strings.TrimPrefix(h.URL, "http"), headers)
	if other != nil {
		other.Close()
	}
	if err == nil || response.StatusCode != 503 {
		t.Fatal("bridge limit not enforced")
	}
	response.Body.Close()
	done := make(chan struct{})
	go func() { b.Close(); close(done) }()
	select {
	case <-done:
	case <-time.After(2 * time.Second):
		t.Fatal("bridge close stuck")
	}
	_ = c.SetReadDeadline(time.Now().Add(time.Second))
	if _, _, err := c.ReadMessage(); err == nil {
		t.Fatal("closed bridge still connected")
	}
	// 固定上游停止后应在 HTTP 升级前报告错误。
	s.tcp.Stop()
	failed, response, err := websocket.DefaultDialer.Dial("ws"+strings.TrimPrefix(s.http.URL, "http")+"/ws", http.Header{"Origin": {s.http.URL}})
	if failed != nil {
		failed.Close()
	}
	if err == nil || response == nil || response.StatusCode != 502 {
		t.Fatal("upstream failure not surfaced")
	}
	response.Body.Close()
}

func TestWebGameHTTPAndFrontendProtocol(t *testing.T) {
	s := startWebArena(t)
	client := &http.Client{Timeout: 2 * time.Second}
	response, err := client.Post(s.http.URL+"/api/guest", "application/json", strings.NewReader(`{"name":"Browser"}`))
	if err != nil {
		t.Fatal(err)
	}
	defer response.Body.Close()
	var guest webgame.Guest
	if err := json.NewDecoder(response.Body).Decode(&guest); err != nil || response.StatusCode != 200 || len(guest.Token) != 48 {
		t.Fatal("guest HTTP failed", err)
	}
	for _, path := range []string{"/healthz", "/api/metrics"} {
		r, err := client.Get(s.http.URL + path)
		if err != nil {
			t.Fatal(err)
		}
		r.Body.Close()
		if r.StatusCode != 200 {
			t.Fatal(path, r.StatusCode)
		}
	}
	source, err := os.ReadFile("../examples/webgame/frontend/src/protocol.ts")
	if err != nil {
		t.Fatal(err)
	}
	for name, id := range map[string]uint32{"Authenticate": webgame.MsgAuthenticate, "Join": webgame.MsgJoin, "Input": webgame.MsgInput, "Leave": webgame.MsgLeave, "Heartbeat": webgame.MsgHeartbeat, "Snapshot": webgame.MsgSnapshot, "Error": webgame.MsgError, "Event": webgame.MsgEvent} {
		pattern := regexp.MustCompile(fmt.Sprintf(`\b%s:\s*%d\b`, name, id))
		if !pattern.Match(source) {
			t.Fatalf("Go/TS message ID drift: %s", name)
		}
	}
}

func TestWebGameRoomCapacityAndLeave(t *testing.T) {
	s := startWebArena(t)
	for i := 0; i < 8; i++ {
		conn, _ := s.enter(t, fmt.Sprintf("Member-%d", i), 1)
		view := webSnapshot(t, conn, func(v webgame.Snapshot) bool { return len(v.Players) == i+1 })
		if view.Players[len(view.Players)-1].Nearby != i {
			t.Fatal("AOI did not include colocated room members")
		}
	}
	visitor, _ := s.enter(t, "Visitor", 2)
	webSend(t, visitor, webgame.MsgJoin, map[string]any{"room_id": 1})
	var failure struct {
		Code string `json:"code"`
	}
	webRead(t, visitor, webgame.MsgError, &failure)
	if failure.Code != "room_full" {
		t.Fatal(failure)
	}
	// 目标房间满时保留原房间，不能先移除原成员。
	webSnapshot(t, visitor, func(v webgame.Snapshot) bool { return v.RoomID == 2 && len(v.Players) == 1 })
	webSend(t, visitor, webgame.MsgLeave, struct{}{})
	var reply map[string]any
	webRead(t, visitor, webgame.MsgLeave, &reply)
	webSend(t, visitor, webgame.MsgInput, map[string]any{"dx": 1, "dy": 0, "seq": 1})
	webRead(t, visitor, webgame.MsgError, &failure)
	if failure.Code != "not_in_room" {
		t.Fatal(failure)
	}
}

func TestWebSocketBridgeIdleTimeout(t *testing.T) {
	s := startWebArena(t)
	b, err := wsbridge.New(wsbridge.Options{Upstream: s.tcp.Addr().String(), IdleTimeout: 100 * time.Millisecond})
	if err != nil {
		t.Fatal(err)
	}
	h := httptest.NewServer(b)
	t.Cleanup(h.Close)
	t.Cleanup(func() { b.Close() })
	c, _, err := (&websocket.Dialer{HandshakeTimeout: time.Second}).Dial("ws"+strings.TrimPrefix(h.URL, "http"), http.Header{"Origin": {h.URL}})
	if err != nil {
		t.Fatal(err)
	}
	defer c.Close()
	_ = c.SetReadDeadline(time.Now().Add(2 * time.Second))
	if _, _, err := c.ReadMessage(); err == nil {
		t.Fatal("idle bridge was not closed")
	}
	deadline := time.Now().Add(time.Second)
	for s.tcp.GetMetrics().Connections != 0 && time.Now().Before(deadline) {
		time.Sleep(10 * time.Millisecond)
	}
	if s.tcp.GetMetrics().Connections != 0 {
		t.Fatal("idle TCP upstream leaked")
	}
}
