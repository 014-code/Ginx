package test

import (
	"Ginx/utils"
	"testing"
)

func TestReloadLoadsParentConfig(t *testing.T) {
	original := *utils.GlobalObject
	t.Cleanup(func() {
		*utils.GlobalObject = original
	})

	utils.GlobalObject.Name = "default name"
	utils.GlobalObject.Host = "0.0.0.0"
	utils.GlobalObject.TcpPort = 0
	utils.GlobalObject.Reload()

	if utils.GlobalObject.Name != "demo server" {
		t.Fatalf("Name = %q, want %q", utils.GlobalObject.Name, "demo server")
	}
	if utils.GlobalObject.Host != "127.0.0.1" {
		t.Fatalf("Host = %q, want %q", utils.GlobalObject.Host, "127.0.0.1")
	}
	if utils.GlobalObject.TcpPort != 7777 {
		t.Fatalf("TcpPort = %d, want %d", utils.GlobalObject.TcpPort, 7777)
	}
}
