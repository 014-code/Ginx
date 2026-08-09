package test

import (
	"Ginx/gface"
	"Ginx/gnet"
	"Ginx/utils"
	"io"
	"net"
	"testing"
	"time"
)

type testConnection struct {
	id uint32
}

func (c *testConnection) Start() {}

func (c *testConnection) Stop() {}

func (c *testConnection) GetConnId() uint32 {
	return c.id
}

func (c *testConnection) GetConnection() net.Conn {
	return nil
}

func (c *testConnection) RemoteAddr() net.Addr {
	return nil
}

func (c *testConnection) SendMsg(uint32, []byte) error {
	return nil
}

func (c *testConnection) SendBuffMsg(uint32, []byte) error {
	return nil
}

func (c *testConnection) SetProperty(string, interface{}) {}

func (c *testConnection) GetProperty(string) (interface{}, error) {
	return nil, nil
}

func (c *testConnection) RemoveProperty(string) {}

type managedTestConnection struct {
	testConnection
	stopped chan struct{}
}

func newManagedTestConnection(id uint32) *managedTestConnection {
	return &managedTestConnection{
		testConnection: testConnection{id: id},
		stopped:        make(chan struct{}),
	}
}

func (c *managedTestConnection) Stop() {
	select {
	case <-c.stopped:
	default:
		close(c.stopped)
	}
}

type testRequest struct {
	connection gface.IConnection
	msgID      uint32
	data       []byte
}

func (r *testRequest) GetConnection() gface.IConnection {
	return r.connection
}

func (r *testRequest) GetData() []byte {
	return r.data
}

func (r *testRequest) GetMsgID() uint32 {
	return r.msgID
}

func setWorkerConfig(t *testing.T, workerPoolSize uint32, maxWorkerTaskLen uint32) {
	t.Helper()
	original := *utils.GlobalObject
	t.Cleanup(func() {
		*utils.GlobalObject = original
	})

	utils.GlobalObject.WorkerPoolSize = workerPoolSize
	utils.GlobalObject.MaxWorkerTaskLen = maxWorkerTaskLen
}

func setMessageBufferConfig(t *testing.T, maxMsgChanLen uint32) {
	t.Helper()
	original := *utils.GlobalObject
	t.Cleanup(func() {
		*utils.GlobalObject = original
	})

	utils.GlobalObject.MaxMsgChanLen = maxMsgChanLen
}

func setServerConfig(t *testing.T, maxConn int) {
	t.Helper()
	original := *utils.GlobalObject
	t.Cleanup(func() {
		*utils.GlobalObject = original
	})

	utils.GlobalObject.MaxConn = maxConn
	utils.GlobalObject.WorkerPoolSize = 0
	utils.GlobalObject.HeartbeatMax = 0
}

func newTCPPair(t *testing.T) (*net.TCPConn, *net.TCPConn) {
	t.Helper()
	listener, err := net.ListenTCP("tcp4", &net.TCPAddr{IP: net.IPv4(127, 0, 0, 1)})
	if err != nil {
		t.Fatalf("ListenTCP() error = %v", err)
	}

	accepted := make(chan *net.TCPConn, 1)
	acceptErrors := make(chan error, 1)
	go func() {
		conn, acceptErr := listener.AcceptTCP()
		if acceptErr != nil {
			acceptErrors <- acceptErr
			return
		}
		accepted <- conn
	}()

	client, err := net.DialTCP("tcp4", nil, listener.Addr().(*net.TCPAddr))
	if err != nil {
		listener.Close()
		t.Fatalf("DialTCP() error = %v", err)
	}

	var server *net.TCPConn
	select {
	case server = <-accepted:
	case acceptErr := <-acceptErrors:
		client.Close()
		listener.Close()
		t.Fatalf("AcceptTCP() error = %v", acceptErr)
	case <-time.After(time.Second):
		client.Close()
		listener.Close()
		t.Fatal("timed out waiting for accepted connection")
	}
	listener.Close()
	return client, server
}

func startTestConnection(t *testing.T, handler *gnet.MsgHandle) (*net.TCPConn, chan struct{}) {
	t.Helper()
	client, server := newTCPPair(t)
	connection := gnet.NewConntion(server, 42, handler)
	startDone := make(chan struct{})
	go func() {
		connection.Start()
		close(startDone)
	}()

	t.Cleanup(func() {
		client.Close()
		select {
		case <-startDone:
		case <-time.After(time.Second):
			connection.Stop()
			select {
			case <-startDone:
			case <-time.After(time.Second):
			}
		}
		server.Close()
	})

	return client, startDone
}

func writeTestMessage(t *testing.T, conn net.Conn, msgID uint32, data []byte) {
	t.Helper()
	packet, err := gnet.NewDataPack().Pack(gnet.NewMsgPackage(msgID, data))
	if err != nil {
		t.Fatalf("Pack() error = %v", err)
	}
	if _, err = conn.Write(packet); err != nil {
		t.Fatalf("Write() error = %v", err)
	}
}

func assertReply(t *testing.T, conn net.Conn, wantID uint32, wantData []byte) {
	t.Helper()
	if err := conn.SetReadDeadline(time.Now().Add(time.Second)); err != nil {
		t.Fatalf("SetReadDeadline() error = %v", err)
	}

	header := make([]byte, gnet.NewDataPack().GetHeadLen())
	if _, err := io.ReadFull(conn, header); err != nil {
		t.Fatalf("ReadFull() header error = %v", err)
	}
	message, err := gnet.NewDataPack().Unpack(header)
	if err != nil {
		t.Fatalf("Unpack() error = %v", err)
	}
	data := make([]byte, message.GetDataLen())
	if _, err = io.ReadFull(conn, data); err != nil {
		t.Fatalf("ReadFull() body error = %v", err)
	}
	if message.GetMsgID() != wantID || string(data) != string(wantData) {
		t.Fatalf("reply = id:%d data:%q, want id:%d data:%q", message.GetMsgID(), data, wantID, wantData)
	}
}
