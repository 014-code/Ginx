package test

import (
	"Ginx/examples/gameprotocol"
	"Ginx/gcore"
	"Ginx/gface"
	"Ginx/gnet"
	"Ginx/session"
	"Ginx/utils"
	"bytes"
	"errors"
	"go/parser"
	"go/token"
	"net"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
	"time"
)

// 防止核心反向依赖示例、账号或具体存储实现。
func TestFrameworkDependencyBoundary(t *testing.T) {
	allowed := map[string]bool{"Ginx/gface": true, "Ginx/gnet": true, "Ginx/utils": true, "Ginx/limit": true, "Ginx/metrics": true}
	for _, pkg := range []string{"gnet", "gface", "gcore", "utils", "limit", "metrics", "session", "persist"} {
		files, err := filepath.Glob(filepath.Join("..", pkg, "*.go"))
		if err != nil || len(files) == 0 {
			t.Fatalf("package %s: %v", pkg, err)
		}
		for _, file := range files {
			ast, err := parser.ParseFile(token.NewFileSet(), file, nil, parser.ImportsOnly)
			if err != nil {
				t.Fatal(err)
			}
			for _, imp := range ast.Imports {
				path, err := strconv.Unquote(imp.Path.Value)
				if err != nil {
					t.Fatal(err)
				}
				if strings.HasPrefix(path, "Ginx/") {
					if !allowed[path] {
						t.Errorf("%s depends on non-core package %s", file, path)
					}
				} else if strings.Contains(strings.Split(path, "/")[0], ".") {
					t.Errorf("%s introduces external dependency %s", file, path)
				}
			}
		}
	}
}

func TestLegacyProtocolCompatibility(t *testing.T) {
	if gcore.GameMsgPlayerMove != gameprotocol.GameMsgPlayerMove || gcore.GameMsgLoginRequest != gameprotocol.GameMsgLoginRequest {
		t.Fatal("legacy message IDs changed")
	}
}

func TestSessionExplicitIdentityAndPolicy(t *testing.T) {
	m, err := session.New(session.Options{})
	if err != nil {
		t.Fatal(err)
	}
	if _, _, err := m.Login("alice", 0); err == nil {
		t.Fatal("new manager allocated a player ID")
	}
	first, _, err := m.Issue("alice", 7000, time.Hour)
	if err != nil || first.PlayerID != 7000 {
		t.Fatal(first, err)
	}
	if _, _, err := m.Issue("alice", 7000, time.Hour); !errors.Is(err, session.ErrAccountActive) {
		t.Fatal(err)
	}
	if _, err := m.GetByToken(first.Token); err != nil {
		t.Fatal("rejected login revoked existing token")
	}
	if _, _, err := m.Issue("bob", 7000, time.Hour); err == nil {
		t.Fatal("identity ownership bypassed")
	}
	m.RemoveExpired(first.ExpiresAt)
	if _, _, err := m.Issue("alice", 7000, time.Hour); err != nil {
		t.Fatal(err)
	}
	replace, err := session.New(session.Options{AccountPolicy: session.ReplaceExisting})
	if err != nil {
		t.Fatal(err)
	}
	a, _, _ := replace.Issue("alice", 7000, time.Hour)
	b, old, err := replace.Issue("alice", 7000, time.Hour)
	if err != nil || old == nil || old.Token != a.Token || b.PlayerID != 7000 {
		t.Fatal("replacement policy failed", err)
	}
	if _, err := session.New(session.Options{AccountPolicy: 255}); err == nil {
		t.Fatal("invalid policy accepted")
	}
	// 默认拒绝策略在并发登录下也只能有一个成功。
	concurrent, _ := session.New(session.Options{})
	var count atomic.Int32
	var wg sync.WaitGroup
	for i := 0; i < 10; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			if _, _, err := concurrent.Issue("alice", 123, time.Hour); err == nil {
				count.Add(1)
			} else if !errors.Is(err, session.ErrAccountActive) {
				t.Error(err)
			}
		}()
	}
	wg.Wait()
	if count.Load() != 1 {
		t.Fatalf("concurrent logins succeeded: %d", count.Load())
	}
}

type configEchoRouter struct{ gnet.BaseRouter }

func (r *configEchoRouter) Handle(req gface.IRequest) {
	_ = req.GetConnection().SendBuffMsg(req.GetMsgID(), req.GetData())
}

func TestServerInstanceConfigIsolation(t *testing.T) {
	original := *utils.GlobalObject
	t.Cleanup(func() { *utils.GlobalObject = original })
	// 构造函数不能尝试读取 cwd 中的无效配置。
	t.Chdir(t.TempDir())
	if err := os.Mkdir("config", 0755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile("config/ginx.json", []byte("not json"), 0600); err != nil {
		t.Fatal(err)
	}
	small := gnet.DefaultConfig()
	small.Host, small.TcpPort, small.MaxConn, small.MaxPacketSize = "127.0.0.1", 0, 1, 4
	small.WorkerPoolSize, small.HeartbeatMax, small.MaxMsgChanLen = 1, 0, 4
	large := small
	large.MaxConn, large.MaxPacketSize, large.WorkerPoolSize = 2, 64, 2
	a, b := gnet.NewServerWithConfig(small), gnet.NewServerWithConfig(large)
	// 构造后改原变量、全局变量均不能影响这两个实例。
	small.MaxPacketSize, large.MaxConn = 1, 1
	utils.GlobalObject.MaxPacketSize, utils.GlobalObject.MaxConn = 1, 1
	utils.GlobalObject.WorkerPoolSize, utils.GlobalObject.MaxWorkerTaskLen = 99, 0
	for _, server := range []*gnet.Server{a, b} {
		server.AddRouter(55, &configEchoRouter{})
		if err := server.StartWithError(); err != nil {
			t.Fatal(err)
		}
		t.Cleanup(server.Stop)
	}
	connect := func(server *gnet.Server) net.Conn {
		conn, err := net.DialTimeout("tcp4", server.Addr().String(), time.Second)
		if err != nil {
			t.Fatal(err)
		}
		t.Cleanup(func() { conn.Close() })
		if err := conn.SetDeadline(time.Now().Add(2 * time.Second)); err != nil {
			t.Fatal(err)
		}
		return conn
	}
	clientA, clientB, secondB := connect(a), connect(b), connect(b)
	waitForConnManagerLen(t, a, 1)
	waitForConnManagerLen(t, b, 2)
	rejectedA := connect(a)
	if _, err := rejectedA.Read(make([]byte, 1)); err == nil {
		t.Fatal("instance connection limit ignored")
	} else if timeout, ok := err.(net.Error); ok && timeout.Timeout() {
		t.Fatal("connection rejection timed out")
	}
	pack := gnet.NewDataPackWithLimit(64)
	echo := func(conn net.Conn, payload string) {
		packet, err := pack.Pack(gnet.NewMsgPackage(55, []byte(payload)))
		if err != nil {
			t.Fatal(err)
		}
		if _, err := conn.Write(packet); err != nil {
			t.Fatal(err)
		}
		message, err := pack.ReadMessage(conn)
		if err != nil {
			t.Fatal(err)
		}
		if string(message.GetData()) != payload {
			t.Fatal("echo mismatch")
		}
	}
	echo(clientA, "four")
	echo(clientB, "larger payload")
	echo(secondB, "second connection")
	oversized, _ := pack.Pack(gnet.NewMsgPackage(55, []byte("large")))
	if _, err := clientA.Write(oversized); err != nil {
		t.Fatal(err)
	}
	if _, err := clientA.Read(make([]byte, 1)); err == nil {
		t.Fatal("instance packet limit ignored")
	} else if timeout, ok := err.(net.Error); ok && timeout.Timeout() {
		t.Fatal("oversized rejection timed out")
	}
}

func TestExplicitConfigLoader(t *testing.T) {
	original := *utils.GlobalObject
	file := filepath.Join(t.TempDir(), "custom.json")
	if err := os.WriteFile(file, []byte(`{"Name":"isolated","TcpPort":0,"WorkerPoolSize":0}`), 0600); err != nil {
		t.Fatal(err)
	}
	config, err := utils.LoadConfig(file)
	if err != nil || config.Name != "isolated" || config.TcpPort != 0 || config.WorkerPoolSize != 0 || config.MaxPacketSize != utils.DefaultConfig().MaxPacketSize {
		t.Fatal(config, err)
	}
	if *utils.GlobalObject != original {
		t.Fatal("loader changed global config")
	}
	if _, err := utils.LoadConfig(file + ".missing"); err == nil {
		t.Fatal("missing file ignored")
	}
	if err := os.WriteFile(file, []byte(`{`), 0600); err != nil {
		t.Fatal(err)
	}
	if _, err := utils.LoadConfig(file); err == nil {
		t.Fatal("invalid file ignored")
	}
	pack := gnet.NewDataPackWithLimit(2)
	if _, err := pack.Pack(gnet.NewMsgPackage(1, []byte("big"))); err == nil {
		t.Fatal("outbound limit ignored")
	}
	data, _ := gnet.NewDataPackWithLimit(0).Pack(gnet.NewMsgPackage(1, []byte("big")))
	if _, err := pack.ReadMessage(bytes.NewReader(data)); err == nil {
		t.Fatal("inbound limit ignored")
	}
}
