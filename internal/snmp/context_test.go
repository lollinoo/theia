package snmp

import (
	"context"
	"net"
	"testing"
	"time"

	"github.com/lollinoo/theia/internal/domain"
)

func TestSNMPCancellationInterruptsPendingRead(t *testing.T) {
	server, err := net.ListenUDP("udp", &net.UDPAddr{IP: net.ParseIP("127.0.0.1")})
	if err != nil {
		t.Fatal(err)
	}
	defer server.Close()
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	client, err := NewClientContext(ctx, "127.0.0.1", domain.SNMPCredentials{Version: domain.SNMPVersionV2c, V2c: &domain.SNMPv2cCredentials{Community: "public"}}, 5*time.Second, 10)
	if err != nil {
		t.Fatal(err)
	}
	client.snmp.Port = uint16(server.LocalAddr().(*net.UDPAddr).Port)
	if err := client.Connect(); err != nil {
		t.Fatal(err)
	}
	defer client.Close()
	done := make(chan error, 1)
	go func() { _, err := client.Get([]string{OidSysName}); done <- err }()
	server.SetReadDeadline(time.Now().Add(time.Second))
	if _, _, err := server.ReadFromUDP(make([]byte, 65535)); err != nil {
		t.Fatal(err)
	}
	cancel()
	select {
	case err := <-done:
		if err == nil {
			t.Fatal("cancelled SNMP read succeeded")
		}
	case <-time.After(time.Second):
		t.Fatal("SNMP read ignored cancellation")
	}
}
