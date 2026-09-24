package test

import (
	"Ginx/examples/gameapp"
	"Ginx/examples/sqlitestore"
	"Ginx/gnet"
	"Ginx/persist"
	"Ginx/utils"
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"io"
	"net"
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"strings"
	"sync"
	"testing"
	"time"

	"golang.org/x/crypto/bcrypt"
)

func gameAccounts(t *testing.T) []gameapp.Account {
	t.Helper()
	hash, err := bcrypt.GenerateFromPassword([]byte("test-password"), bcrypt.MinCost)
	if err != nil {
		t.Fatal(err)
	}
	return []gameapp.Account{{AccountID: "alice", PlayerID: 1001, PasswordHash: string(hash)}, {AccountID: "bob", PlayerID: 1002, PasswordHash: string(hash)}}
}

func gameService(t *testing.T, ttl time.Duration) *gameapp.Service {
	t.Helper()
	s, err := gameapp.New(gameAccounts(t), persist.NewMemoryStore(), ttl, []gameapp.RoomConfig{{ID: 1, MaxPlayers: 1}, {ID: 2, MaxPlayers: 2}})
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(s.Close)
	return s
}

func gameHTTP(t *testing.T, handler http.Handler, method, path, token, body string) *httptest.ResponseRecorder {
	t.Helper()
	req := httptest.NewRequest(method, path, strings.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	if token != "" {
		req.Header.Set("Authorization", "Bearer "+token)
	}
	response := httptest.NewRecorder()
	handler.ServeHTTP(response, req)
	return response
}

func TestGameHTTPAuthenticationAndValidation(t *testing.T) {
	s := gameService(t, time.Hour)
	router := gameapp.NewHTTPHandler(s)
	for _, path := range []string{"/api/v1/me", "/api/v1/rooms"} {
		for _, token := range []string{"", "invalid"} {
			if response := gameHTTP(t, router, "GET", path, token, ""); response.Code != 401 {
				t.Fatalf("%s: %d", path, response.Code)
			}
		}
	}
	for _, test := range []struct {
		body   string
		status int
	}{
		{`{`, 400}, {`{}`, 400}, {`{"account_id":"alice","password":"wrong"}`, 401},
		{`{"account_id":"unknown","password":"test-password"}`, 401},
		{`{"account_id":"alice","password":"` + strings.Repeat("a", 5000) + `"}`, 400},
		{`{"account_id":"alice","password":"test-password"}{}`, 400},
		{`{"account_id":"alice","password":"test-password"}` + strings.Repeat(" ", 5000), 400},
	} {
		response := gameHTTP(t, router, "POST", "/api/v1/login", "", test.body)
		if response.Code != test.status {
			t.Fatalf("login status=%d want=%d", response.Code, test.status)
		}
	}
	response := gameHTTP(t, router, "POST", "/api/v1/login", "", `{"account_id":"alice","password":"test-password","player_id":999}`)
	var login gameapp.LoginResult
	if response.Code != 200 || json.Unmarshal(response.Body.Bytes(), &login) != nil || login.PlayerID != 1001 || login.Token == "" {
		t.Fatal("login failed or accepted client identity")
	}
	if strings.Contains(response.Body.String(), "password") || response.Header().Get("Cache-Control") != "no-store" {
		t.Fatal("login response leaks or caches credentials")
	}
	if response := gameHTTP(t, router, "GET", "/api/v1/me", login.Token, ""); response.Code != 200 {
		t.Fatal("valid bearer rejected")
	}
	if response := gameHTTP(t, router, "GET", "/api/v1/me?token="+login.Token, "", ""); response.Code != 401 {
		t.Fatal("query-string token accepted")
	}
	if response := gameHTTP(t, router, "GET", "/healthz", "", ""); response.Code != 200 {
		t.Fatal("health check failed")
	}
	// 绝对有效期：回收后 HTTP 查询立即失效。
	s.Sweep(login.ExpiresAt)
	if response := gameHTTP(t, router, "GET", "/api/v1/me", login.Token, ""); response.Code != 401 {
		t.Fatal("expired token accepted")
	}
}

type failingGameStore struct {
	persist.PlayerStore
	loadErr error
	saveErr error
}

func (s *failingGameStore) Load(ctx context.Context, id uint64) (*persist.Player, error) {
	if s.loadErr != nil {
		return nil, s.loadErr
	}
	return s.PlayerStore.Load(ctx, id)
}

func (s *failingGameStore) Save(ctx context.Context, player *persist.Player) error {
	if s.saveErr != nil {
		return s.saveErr
	}
	return s.PlayerStore.Save(ctx, player)
}

func TestGameHTTPStorageErrorsAreNotExposed(t *testing.T) {
	accounts := gameAccounts(t)
	for _, stage := range []string{"load", "save", "profile"} {
		t.Run(stage, func(t *testing.T) {
			store := &failingGameStore{PlayerStore: persist.NewMemoryStore()}
			internal := errors.New("private storage path and database details")
			if stage == "load" {
				store.loadErr = internal
			}
			if stage == "save" {
				store.saveErr = internal
			}
			s, err := gameapp.New(accounts, store, time.Hour, nil)
			if err != nil {
				t.Fatal(err)
			}
			t.Cleanup(s.Close)
			router := gameapp.NewHTTPHandler(s)
			var response *httptest.ResponseRecorder
			if stage == "profile" {
				login, err := s.Login(context.Background(), "alice", "test-password")
				if err != nil {
					t.Fatal(err)
				}
				store.loadErr = internal
				response = gameHTTP(t, router, "GET", "/api/v1/me", login.Token, "")
			} else {
				response = gameHTTP(t, router, "POST", "/api/v1/login", "", `{"account_id":"alice","password":"test-password"}`)
			}
			if response.Code != 500 || response.Body.String() != `{"code":"internal_error"}` {
				t.Fatalf("unexpected error response: %d %s", response.Code, response.Body)
			}
		})
	}
}

func TestGameConcurrentRoomJoinAndDisconnect(t *testing.T) {
	s := gameService(t, time.Hour)
	connections := []*managedTestConnection{newManagedTestConnection(0), newManagedTestConnection(1)}
	for i, account := range []string{"alice", "bob"} {
		login, err := s.Login(context.Background(), account, "test-password")
		if err != nil {
			t.Fatal(err)
		}
		s.Connected(connections[i])
		if _, err := s.Authenticate(uint32(i), login.Token); err != nil {
			t.Fatal(err)
		}
	}
	var wg sync.WaitGroup
	results := make(chan error, 2)
	for i := range connections {
		wg.Add(1)
		go func(id uint32) { defer wg.Done(); _, err := s.Join(id, 1); results <- err }(uint32(i))
	}
	wg.Wait()
	close(results)
	winners := 0
	for err := range results {
		if err == nil {
			winners++
		} else if !errors.Is(err, gameapp.ErrRoomUnavailable) {
			t.Fatal(err)
		}
	}
	if winners != 1 {
		t.Fatalf("room capacity race: %d joined", winners)
	}
	wg.Add(2)
	go func() {
		defer wg.Done()
		for i := 0; i < 50; i++ {
			_, _ = s.Join(0, 2)
		}
	}()
	go func() { defer wg.Done(); s.Disconnected(connections[0]) }()
	wg.Wait()
	if _, err := s.Join(0, 2); !errors.Is(err, gameapp.ErrUnauthorized) {
		t.Fatal("disconnected connection rejoined")
	}
}

func TestGameServiceCapacityReplacementAndPersistence(t *testing.T) {
	path := filepath.Join(t.TempDir(), "players.json")
	store, _ := persist.NewJSONFileStore(path)
	accounts := gameAccounts(t)
	s, err := gameapp.New(accounts, store, time.Hour, []gameapp.RoomConfig{{ID: 1, MaxPlayers: 1}, {ID: 2, MaxPlayers: 1}})
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(s.Close)
	a, err := s.Login(context.Background(), "alice", "test-password")
	if err != nil {
		t.Fatal(err)
	}
	b, _ := s.Login(context.Background(), "bob", "test-password")
	c0, c1 := newManagedTestConnection(0), newManagedTestConnection(1)
	s.Connected(c0)
	s.Connected(c1)
	if _, err := s.Join(0, 1); !errors.Is(err, gameapp.ErrUnauthorized) {
		t.Fatal("unauthenticated join")
	}
	if _, err := s.Authenticate(0, a.Token); err != nil {
		t.Fatal(err)
	}
	if _, err := s.Authenticate(1, b.Token); err != nil {
		t.Fatal(err)
	}
	if _, err := s.Join(0, 1); err != nil {
		t.Fatal(err)
	}
	if _, err := s.Join(1, 2); err != nil {
		t.Fatal(err)
	}
	if _, err := s.Join(1, 1); !errors.Is(err, gameapp.ErrRoomUnavailable) {
		t.Fatal("room capacity bypassed")
	}
	view, _ := s.Me(context.Background(), b.Token)
	if view.RoomID != 2 {
		t.Fatal("failed room switch removed old membership")
	}
	if _, err := s.Join(0, 999); !errors.Is(err, gameapp.ErrRoomNotFound) {
		t.Fatal("missing room accepted")
	}
	newLogin, err := s.Login(context.Background(), "alice", "test-password")
	if err != nil || newLogin.PlayerID != a.PlayerID {
		t.Fatal("unstable player identity")
	}
	select {
	case <-c0.stopped:
	default:
		t.Fatal("old connection not closed")
	}
	if _, err := s.Join(0, 1); !errors.Is(err, gameapp.ErrUnauthorized) {
		t.Fatal("old queued request still authorized")
	}
	s.Disconnected(c0) // 旧连接延迟清理不能注销新登录。
	if _, err := s.Me(context.Background(), newLogin.Token); err != nil {
		t.Fatal("late disconnect revoked new token")
	}
	rooms, _ := s.Rooms(newLogin.Token)
	if rooms[0].Players != 0 || rooms[1].Players != 1 {
		t.Fatalf("room counts: %+v", rooms)
	}
	s.Sweep(b.ExpiresAt)
	select {
	case <-c1.stopped:
	default:
		t.Fatal("expired bound connection not closed")
	}
	if err := s.Heartbeat(1); !errors.Is(err, gameapp.ErrUnauthorized) {
		t.Fatal("expired heartbeat accepted")
	}
	stored, _ := store.Load(context.Background(), 1001)
	stored.Data = []byte("persisted state")
	if err := store.Save(context.Background(), stored); err != nil {
		t.Fatal(err)
	}
	s.Close()
	store2, _ := persist.NewJSONFileStore(path)
	restarted, err := gameapp.New(accounts, store2, time.Hour, nil)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(restarted.Close)
	login, err := restarted.Login(context.Background(), "alice", "test-password")
	if err != nil {
		t.Fatal(err)
	}
	profile, err := restarted.Me(context.Background(), login.Token)
	if err != nil || profile.Player.PlayerID != 1001 || string(profile.Player.Data) != "persisted state" {
		t.Fatal("restart lost or overwrote player")
	}
}

func TestGameServiceRejectsExpiredTCPTokenAndCleansUnauthenticatedConnection(t *testing.T) {
	s := gameService(t, time.Nanosecond)
	login, err := s.Login(context.Background(), "alice", "test-password")
	if err != nil {
		t.Fatal(err)
	}
	c := newManagedTestConnection(0)
	s.Connected(c)
	if _, err := s.Authenticate(0, login.Token); !errors.Is(err, gameapp.ErrUnauthorized) {
		t.Fatal("expired TCP token accepted")
	}
	s.Sweep(time.Now().Add(time.Minute))
	select {
	case <-c.stopped:
	default:
		t.Fatal("unauthenticated connection not reclaimed")
	}
}

func gamePacket(t *testing.T, conn net.Conn, id uint32, data any) gameapp.Reply {
	t.Helper()
	if err := conn.SetDeadline(time.Now().Add(2 * time.Second)); err != nil {
		t.Fatal(err)
	}
	body, err := json.Marshal(data)
	if err != nil {
		t.Fatal(err)
	}
	packet, err := gnet.NewDataPack().Pack(gnet.NewMsgPackage(id, body))
	if err != nil {
		t.Fatal(err)
	}
	if _, err := io.Copy(conn, bytes.NewReader(packet)); err != nil {
		t.Fatal(err)
	}
	message, err := gnet.NewDataPack().ReadMessage(conn)
	if err != nil {
		t.Fatal(err)
	}
	var reply gameapp.Reply
	if message.GetMsgID() != id || json.Unmarshal(message.GetData(), &reply) != nil {
		t.Fatal("invalid TCP reply")
	}
	return reply
}

func TestGameHTTPToTCPFlowAndShutdown(t *testing.T) {
	original := *utils.GlobalObject
	t.Cleanup(func() { *utils.GlobalObject = original })
	store, err := sqlitestore.Open(filepath.Join(t.TempDir(), "players.db"))
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { store.Close() })
	s, err := gameapp.New(gameAccounts(t), store, time.Hour, []gameapp.RoomConfig{{ID: 1, MaxPlayers: 2}})
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(s.Close)
	tcp := gnet.NewServer().(*gnet.Server)
	tcp.IP, tcp.Port = "127.0.0.1", 0
	gameapp.RegisterTCP(tcp, s)
	listener, err := net.Listen("tcp4", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	ctx, cancel := context.WithCancel(context.Background())
	done := make(chan error, 1)
	go func() { done <- gameapp.Serve(ctx, listener, tcp, s) }()
	t.Cleanup(func() {
		cancel()
		select {
		case err := <-done:
			if err != nil {
				t.Error(err)
			}
		case <-time.After(5 * time.Second):
			t.Error("runtime shutdown timed out")
		}
	})
	client := &http.Client{Timeout: 3 * time.Second}
	url := "http://" + listener.Addr().String()
	response, err := client.Post(url+"/api/v1/login", "application/json", strings.NewReader(`{"account_id":"alice","password":"test-password"}`))
	if err != nil {
		t.Fatal(err)
	}
	var login gameapp.LoginResult
	err = json.NewDecoder(response.Body).Decode(&login)
	response.Body.Close()
	if err != nil || response.StatusCode != 200 || login.Token == "" {
		t.Fatal("HTTP login failed")
	}
	conn, err := net.DialTimeout("tcp4", tcp.Addr().String(), time.Second)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { conn.Close() })
	if reply := gamePacket(t, conn, gameapp.MsgJoinRoom, map[string]any{"room_id": 1}); reply.Code != "unauthorized" {
		t.Fatalf("join before auth: %+v", reply)
	}
	if reply := gamePacket(t, conn, gameapp.MsgAuthenticate, map[string]any{"token": "invalid"}); reply.Code != "unauthorized" {
		t.Fatal("invalid token accepted")
	}
	if reply := gamePacket(t, conn, gameapp.MsgAuthenticate, map[string]any{"token": login.Token}); reply.Code != "ok" {
		t.Fatalf("auth: %+v", reply)
	}
	if reply := gamePacket(t, conn, gameapp.MsgJoinRoom, map[string]any{"room_id": 1, "player_id": 999}); reply.Code != "invalid_request" {
		t.Fatal("unknown TCP identity field accepted")
	}
	if reply := gamePacket(t, conn, gameapp.MsgJoinRoom, map[string]any{"room_id": 1}); reply.Code != "ok" {
		t.Fatalf("join: %+v", reply)
	}
	if reply := gamePacket(t, conn, gameapp.MsgHeartbeat, struct{}{}); reply.Code != "ok" {
		t.Fatal("heartbeat failed")
	}
	if reply := gamePacket(t, conn, gameapp.MsgStarterReward, map[string]any{"experience": 999}); reply.Code != "invalid_request" {
		t.Fatal("client selected reward")
	}
	if reply := gamePacket(t, conn, gameapp.MsgStarterReward, struct{}{}); reply.Code != "ok" {
		t.Fatalf("reward: %+v", reply)
	}
	if reply := gamePacket(t, conn, gameapp.MsgStarterReward, struct{}{}); reply.Code != "reward_already_claimed" {
		t.Fatal("duplicate reward")
	}
	if reply := gamePacket(t, conn, gameapp.MsgUseItem, map[string]string{"item_id": "potion"}); reply.Code != "ok" {
		t.Fatalf("use item: %+v", reply)
	}
	if reply := gamePacket(t, conn, gameapp.MsgProgress, struct{}{}); reply.Code != "ok" {
		t.Fatal("progress query failed")
	}
	progressReq, _ := http.NewRequest("GET", url+"/api/v1/progress", nil)
	progressReq.Header.Set("Authorization", "Bearer "+login.Token)
	progressRes, err := client.Do(progressReq)
	if err != nil {
		t.Fatal(err)
	}
	var progress gameapp.Progress
	err = json.NewDecoder(progressRes.Body).Decode(&progress)
	progressRes.Body.Close()
	if err != nil || progressRes.StatusCode != 200 || progress.Level != 2 || progress.Inventory["potion"] != 1 {
		t.Fatal("TCP/HTTP progress mismatch", progress, err)
	}
	req, _ := http.NewRequest("GET", url+"/api/v1/me", nil)
	req.Header.Set("Authorization", "Bearer "+login.Token)
	response, err = client.Do(req)
	if err != nil {
		t.Fatal(err)
	}
	var me gameapp.PlayerView
	err = json.NewDecoder(response.Body).Decode(&me)
	response.Body.Close()
	if err != nil || response.StatusCode != 200 || !me.Online || me.RoomID != 1 || me.Player.PlayerID != login.PlayerID {
		t.Fatalf("HTTP/TCP state mismatch: %+v %v", me, err)
	}
	if reply := gamePacket(t, conn, gameapp.MsgLeaveRoom, struct{}{}); reply.Code != "ok" {
		t.Fatal("leave failed")
	}
	if reply := gamePacket(t, conn, gameapp.MsgJoinRoom, map[string]any{"room_id": 1}); reply.Code != "ok" {
		t.Fatal("rejoin failed")
	}
	conn.Close()
	deadline := time.Now().Add(time.Second)
	for time.Now().Before(deadline) {
		if _, err := s.Me(context.Background(), login.Token); errors.Is(err, gameapp.ErrUnauthorized) {
			break
		}
		time.Sleep(time.Millisecond)
	}
	if _, err := s.Me(context.Background(), login.Token); !errors.Is(err, gameapp.ErrUnauthorized) {
		t.Fatal("disconnect did not revoke token")
	}
	// 保留一个活动连接，确认联合停服也关闭 TCP 连接和监听端口。
	active, err := net.DialTimeout("tcp4", tcp.Addr().String(), time.Second)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { active.Close() })
	tcpAddress := tcp.Addr().String()
	cancel()
	_ = active.SetReadDeadline(time.Now().Add(3 * time.Second))
	if _, err := active.Read(make([]byte, 1)); !errors.Is(err, io.EOF) {
		t.Fatalf("shutdown connection: %v", err)
	}
	deadline = time.Now().Add(time.Second)
	for tcp.Addr() != nil && time.Now().Before(deadline) {
		time.Sleep(time.Millisecond)
	}
	if probe, err := net.DialTimeout("tcp4", tcpAddress, 100*time.Millisecond); err == nil {
		probe.Close()
		t.Fatal("TCP listener survived shutdown")
	}
}

func TestGameRuntimeClosesHTTPWhenTCPPortIsBusy(t *testing.T) {
	original := *utils.GlobalObject
	t.Cleanup(func() { *utils.GlobalObject = original })
	busy, err := net.Listen("tcp4", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { busy.Close() })
	httpListener, err := net.Listen("tcp4", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	address := httpListener.Addr().String()
	tcp := gnet.NewServer().(*gnet.Server)
	tcp.IP, tcp.Port = "127.0.0.1", busy.Addr().(*net.TCPAddr).Port
	s := gameService(t, time.Hour)
	gameapp.RegisterTCP(tcp, s)
	if err := gameapp.Serve(context.Background(), httpListener, tcp, s); err == nil {
		t.Fatal("busy TCP port was ignored")
	}
	if probe, err := net.DialTimeout("tcp4", address, 100*time.Millisecond); err == nil {
		probe.Close()
		t.Fatal("HTTP listener leaked after TCP startup failure")
	}
}
