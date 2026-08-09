package test

import (
	"Ginx/gface"
	"Ginx/gnet"
	"fmt"
	"sync"
	"testing"
	"time"
)

func TestConnectionPropertySetGetAndRemove(t *testing.T) {
	connection := gnet.NewConntion(nil, 42, gnet.NewMsgHandle())

	connection.SetProperty("Name", "Ginx Player")
	connection.SetProperty("Level", 10)

	name, err := connection.GetProperty("Name")
	if err != nil {
		t.Fatalf("GetProperty() error = %v", err)
	}
	if name != "Ginx Player" {
		t.Fatalf("Name property = %v, want %q", name, "Ginx Player")
	}

	connection.SetProperty("Level", 11)
	level, err := connection.GetProperty("Level")
	if err != nil {
		t.Fatalf("GetProperty() updated value error = %v", err)
	}
	if level != 11 {
		t.Fatalf("Level property = %v, want 11", level)
	}

	connection.SetProperty("NilValue", nil)
	nilValue, err := connection.GetProperty("NilValue")
	if err != nil {
		t.Fatalf("GetProperty() nil value error = %v", err)
	}
	if nilValue != nil {
		t.Fatalf("NilValue property = %v, want nil", nilValue)
	}

	connection.RemoveProperty("Level")
	if _, err := connection.GetProperty("Level"); err == nil {
		t.Fatal("GetProperty() found a removed property")
	}
	connection.RemoveProperty("missing")
}

func TestConnectionPropertiesAreConcurrentSafe(t *testing.T) {
	connection := gnet.NewConntion(nil, 42, gnet.NewMsgHandle())
	var waitGroup sync.WaitGroup

	for i := 0; i < 100; i++ {
		waitGroup.Add(1)
		go func(propertyID int) {
			defer waitGroup.Done()
			key := fmt.Sprintf("property-%d", propertyID)
			connection.SetProperty(key, propertyID)
			value, err := connection.GetProperty(key)
			if err != nil || value != propertyID {
				t.Errorf("property %q = %v, error = %v, want %d", key, value, err, propertyID)
			}
			connection.RemoveProperty(key)
		}(i)
	}

	waitGroup.Wait()
}

func TestServerLifecycleHooksUseConnectionProperties(t *testing.T) {
	setServerConfig(t, 2)
	server, address := newTestServer(t, 2)
	startHookDone := make(chan error, 1)
	stopHookDone := make(chan string, 1)

	server.SetOnConnStart(func(connection gface.IConnection) {
		connection.SetProperty("Name", "Ginx Player")
		connection.SetProperty("Home", "lobby")
		startHookDone <- connection.SendMsg(99, []byte("connection ready"))
	})
	server.SetOnConnStop(func(connection gface.IConnection) {
		name, nameErr := connection.GetProperty("Name")
		home, homeErr := connection.GetProperty("Home")
		if nameErr != nil || homeErr != nil {
			stopHookDone <- fmt.Sprintf("property error: %v %v", nameErr, homeErr)
			return
		}
		stopHookDone <- fmt.Sprintf("%v@%v", name, home)
	})

	client := dialTestServer(t, address)
	defer client.Close()
	assertReply(t, client, 99, []byte("connection ready"))

	select {
	case err := <-startHookDone:
		if err != nil {
			t.Fatalf("OnConnStart SendMsg() error = %v", err)
		}
	case <-time.After(time.Second):
		t.Fatal("OnConnStart hook did not finish")
	}

	client.Close()
	select {
	case property := <-stopHookDone:
		if property != "Ginx Player@lobby" {
			t.Fatalf("OnConnStop properties = %q, want %q", property, "Ginx Player@lobby")
		}
	case <-time.After(time.Second):
		t.Fatal("OnConnStop hook did not read connection properties")
	}
}
