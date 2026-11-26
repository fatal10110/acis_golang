package main

import (
	"bytes"
	"context"
	"net"
	"strings"
	"testing"

	"github.com/rs/zerolog"

	"github.com/fatal10110/acis_golang/internal/commons/netutil"
	"github.com/fatal10110/acis_golang/internal/config"
)

// TestListenAddressFromGameserverHostname: "*" binds every address, an IP
// literal binds that address, and an empty host binds loopback only.
func TestListenAddressFromGameserverHostname(t *testing.T) {
	for _, tc := range []struct{ host, want string }{
		{"*", ":7777"},
		{"127.0.0.3", "127.0.0.3:7777"},
		{"", "127.0.0.1:7777"},
	} {
		serverProps, err := config.ParseString("GameserverHostname = " + tc.host + "\nGameserverPort = 7777\n")
		if err != nil {
			t.Fatal(err)
		}
		hexProps, err := config.ParseString("ServerID = 1\nHexID = -7fff\n")
		if err != nil {
			t.Fatal(err)
		}
		cfg, err := gameServerConfigFromProperties(gameServerPaths{}, serverProps, hexProps)
		if err != nil {
			t.Fatalf("GameserverHostname %q: %v", tc.host, err)
		}
		if cfg.ListenAddr != tc.want {
			t.Errorf("GameserverHostname %q: ListenAddr = %q, want %q", tc.host, cfg.ListenAddr, tc.want)
		}
	}
}

// TestEmptyGameserverHostnameBindsLoopbackOnly: the listener an empty host
// opens accepts on the loopback address alone.
func TestEmptyGameserverHostnameBindsLoopbackOnly(t *testing.T) {
	addr := gameBindAddress(context.Background(), netutil.ListenAddress("", 0), net.DefaultResolver.LookupIPAddr, zerolog.Nop())
	ln, err := net.Listen("tcp", addr)
	if err != nil {
		t.Fatal(err)
	}
	defer ln.Close()
	if ip := ln.Addr().(*net.TCPAddr).IP; !ip.Equal(net.IPv4(127, 0, 0, 1)) {
		t.Fatalf("listener bound %v, want 127.0.0.1", ip)
	}
}

// TestGameBindAddressResolvesHostName: a literal or wildcard address binds
// as given without a lookup, and a host name binds one of its addresses,
// IPv4 first.
func TestGameBindAddressResolvesHostName(t *testing.T) {
	noLookup := func(context.Context, string) ([]net.IPAddr, error) {
		t.Fatal("lookup called for a literal address")
		return nil, nil
	}
	for _, addr := range []string{":7777", "127.0.0.3:7777", "[::1]:7777"} {
		if got := gameBindAddress(context.Background(), addr, noLookup, zerolog.Nop()); got != addr {
			t.Errorf("gameBindAddress(%q) = %q, want it unchanged", addr, got)
		}
	}
	lookup := func(_ context.Context, host string) ([]net.IPAddr, error) {
		if host != "game.example" {
			t.Fatalf("lookup(%q), want game.example", host)
		}
		return []net.IPAddr{{IP: net.ParseIP("2001:db8::5")}, {IP: net.ParseIP("192.0.2.5")}}, nil
	}
	if got := gameBindAddress(context.Background(), "game.example:7777", lookup, zerolog.Nop()); got != "192.0.2.5:7777" {
		t.Errorf("gameBindAddress(game.example) = %q, want 192.0.2.5:7777", got)
	}
}

// TestUnresolvableGameserverHostnameBindsAllAddresses: a host name that
// does not resolve logs and binds every address instead of failing boot.
func TestUnresolvableGameserverHostnameBindsAllAddresses(t *testing.T) {
	var logs bytes.Buffer
	addr := gameBindAddress(context.Background(), netutil.ListenAddress("no-such-host.invalid", 0), net.DefaultResolver.LookupIPAddr, zerolog.New(&logs))
	if addr != ":0" {
		t.Fatalf("gameBindAddress(no-such-host.invalid) = %q, want :0", addr)
	}
	if !strings.Contains(logs.String(), "The GameServer bind address is invalid, using all available IPs.") || !strings.Contains(logs.String(), `"level":"error"`) {
		t.Errorf("log = %q, want the invalid bind address error", logs.String())
	}
	ln, err := net.Listen("tcp", addr)
	if err != nil {
		t.Fatal(err)
	}
	defer ln.Close()
	if ip := ln.Addr().(*net.TCPAddr).IP; !ip.IsUnspecified() {
		t.Fatalf("listener bound %v, want every address", ip)
	}
}
