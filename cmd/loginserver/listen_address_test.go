package main

import (
	"bytes"
	"context"
	"net"
	"strings"
	"testing"

	"github.com/rs/zerolog"
	"go.uber.org/fx/fxtest"

	"github.com/fatal10110/acis_golang/internal/config"
)

// hostnameConfig builds the login server config for the two listener hosts.
func hostnameConfig(t *testing.T, clientHost, linkHost string) loginServerConfig {
	t.Helper()
	props, err := config.ParseString("LoginserverHostname = " + clientHost + "\nLoginserverPort = 0\n" +
		"LoginHostname = " + linkHost + "\nLoginPort = 0\n")
	if err != nil {
		t.Fatal(err)
	}
	cfg, err := loginServerConfigFromProperties(loginServerPaths{}, props)
	if err != nil {
		t.Fatal(err)
	}
	return cfg
}

// TestEmptyLoginHostnamesBindLoopbackOnly: an empty LoginserverHostname or
// LoginHostname binds the loopback address alone, for the client listener
// and the game server link listener both.
func TestEmptyLoginHostnamesBindLoopbackOnly(t *testing.T) {
	cfg := hostnameConfig(t, "", "")
	if cfg.ClientAddr != "127.0.0.1:0" || cfg.GameServerAddr != "127.0.0.1:0" {
		t.Fatalf("ClientAddr = %q, GameServerAddr = %q, want 127.0.0.1:0 for both", cfg.ClientAddr, cfg.GameServerAddr)
	}
	for _, addr := range []string{
		clientBindAddress(context.Background(), cfg.ClientAddr, net.DefaultResolver.LookupIPAddr, zerolog.Nop()),
		cfg.GameServerAddr,
	} {
		ln, err := net.Listen("tcp", addr)
		if err != nil {
			t.Fatal(err)
		}
		ip := ln.Addr().(*net.TCPAddr).IP
		_ = ln.Close()
		if !ip.Equal(net.IPv4(127, 0, 0, 1)) {
			t.Fatalf("listener for %q bound %v, want 127.0.0.1", addr, ip)
		}
	}
}

// TestClientBindAddressResolvesHostName: a literal or wildcard address binds
// as given without a lookup, and a host name binds one of its addresses,
// IPv4 first.
func TestClientBindAddressResolvesHostName(t *testing.T) {
	noLookup := func(context.Context, string) ([]net.IPAddr, error) {
		t.Fatal("lookup called for a literal address")
		return nil, nil
	}
	for _, addr := range []string{":2106", "127.0.0.3:2106", "[::1]:2106"} {
		if got := clientBindAddress(context.Background(), addr, noLookup, zerolog.Nop()); got != addr {
			t.Errorf("clientBindAddress(%q) = %q, want it unchanged", addr, got)
		}
	}
	lookup := func(_ context.Context, host string) ([]net.IPAddr, error) {
		if host != "login.example" {
			t.Fatalf("lookup(%q), want login.example", host)
		}
		return []net.IPAddr{{IP: net.ParseIP("2001:db8::5")}, {IP: net.ParseIP("192.0.2.5")}}, nil
	}
	if got := clientBindAddress(context.Background(), "login.example:2106", lookup, zerolog.Nop()); got != "192.0.2.5:2106" {
		t.Errorf("clientBindAddress(login.example) = %q, want 192.0.2.5:2106", got)
	}
}

// TestUnresolvableLoginserverHostnameBindsAllAddresses: a client host name
// that does not resolve logs and binds every address instead of failing
// boot.
func TestUnresolvableLoginserverHostnameBindsAllAddresses(t *testing.T) {
	cfg := hostnameConfig(t, "no-such-host.invalid", "127.0.0.1")
	var logs bytes.Buffer
	addr := clientBindAddress(context.Background(), cfg.ClientAddr, net.DefaultResolver.LookupIPAddr, zerolog.New(&logs))
	if addr != ":0" {
		t.Fatalf("clientBindAddress(no-such-host.invalid) = %q, want :0", addr)
	}
	if !strings.Contains(logs.String(), "The LoginServer bind address is invalid, using all available IPs.") || !strings.Contains(logs.String(), `"level":"error"`) {
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

// TestUnresolvableLoginHostnameFailsBoot: a game server link host name that
// does not resolve stops the listener start, and so the boot.
func TestUnresolvableLoginHostnameFailsBoot(t *testing.T) {
	cfg := hostnameConfig(t, "127.0.0.1", "no-such-host.invalid")
	lc := fxtest.NewLifecycle(t)
	startLoginServer(lc, cfg, nil, nil, nil, zerolog.Nop())
	err := lc.Start(context.Background())
	if err == nil {
		_ = lc.Stop(context.Background())
		t.Fatal("listener start succeeded with an unresolvable LoginHostname")
	}
	if !strings.Contains(err.Error(), "listen for gameservers on no-such-host.invalid:0") {
		t.Fatalf("start error = %v, want the game server link listen failure", err)
	}
}
