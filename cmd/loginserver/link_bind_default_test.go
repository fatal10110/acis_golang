package main

import (
	"net"
	"testing"

	"github.com/fatal10110/acis_golang/internal/config"
)

// TestMissingLoginHostnameBindsLinkLoopbackOnly: with no LoginHostname the
// game server link listens on the loopback address alone (#4655), and an
// explicit * still binds every address.
func TestMissingLoginHostnameBindsLinkLoopbackOnly(t *testing.T) {
	props, err := config.ParseString("LoginPort = 0\n")
	if err != nil {
		t.Fatal(err)
	}
	cfg, err := loginServerConfigFromProperties(loginServerPaths{}, props)
	if err != nil {
		t.Fatal(err)
	}
	if cfg.GameServerAddr != "127.0.0.1:0" {
		t.Fatalf("GameServerAddr = %q, want 127.0.0.1:0", cfg.GameServerAddr)
	}
	ln, err := net.Listen("tcp", cfg.GameServerAddr)
	if err != nil {
		t.Fatal(err)
	}
	ip := ln.Addr().(*net.TCPAddr).IP
	_ = ln.Close()
	if !ip.IsLoopback() {
		t.Fatalf("default link listener bound %v, want a loopback address", ip)
	}

	if cfg := hostnameConfig(t, "*", "*"); cfg.GameServerAddr != ":0" {
		t.Fatalf("GameServerAddr with LoginHostname = * is %q, want :0", cfg.GameServerAddr)
	}
}
