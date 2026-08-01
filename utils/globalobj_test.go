package utils

import "testing"

func TestReloadLoadsParentConfig(t *testing.T) {
	original := *GlobalObject
	defer func() {
		*GlobalObject = original
	}()

	GlobalObject.Name = "default name"
	GlobalObject.Host = "0.0.0.0"
	GlobalObject.TcpPort = 0
	GlobalObject.Reload()

	if GlobalObject.Name != "demo server" {
		t.Fatalf("Name = %q, want %q", GlobalObject.Name, "demo server")
	}
	if GlobalObject.Host != "127.0.0.1" {
		t.Fatalf("Host = %q, want %q", GlobalObject.Host, "127.0.0.1")
	}
	if GlobalObject.TcpPort != 7777 {
		t.Fatalf("TcpPort = %d, want %d", GlobalObject.TcpPort, 7777)
	}
}
