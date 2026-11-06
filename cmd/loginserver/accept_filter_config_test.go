package main

import (
	"net/netip"
	"slices"
	"testing"
	"time"

	"github.com/fatal10110/acis_golang/internal/commons/netutil"
	"github.com/fatal10110/acis_golang/internal/config"
)

// TestLoginServerConfigReadsTheAcceptFilter pins the login-client accept
// filter keys of loginserver.properties: absent, the defaults; set, each
// value, apart from the game-server link's flood keys; malformed, a config
// error.
func TestLoginServerConfigReadsTheAcceptFilter(t *testing.T) {
	load := func(text string) (loginServerConfig, error) {
		t.Helper()
		props, err := config.ParseString(text)
		if err != nil {
			t.Fatal(err)
		}
		return loginServerConfigFromProperties(loginServerPaths{}, props)
	}

	cfg, err := load("LoginserverPort = 2106\n")
	if err != nil {
		t.Fatal(err)
	}
	def := netutil.DefaultAcceptFilterConfig()
	if got := cfg.AcceptFilter; got.Enabled != def.Enabled || got.BurstWindow != def.BurstWindow ||
		got.BurstLimit != def.BurstLimit || got.BanTime != def.BanTime || got.Allowlist != nil {
		t.Fatalf("AcceptFilter with no keys = %+v, want the defaults", got)
	}

	cfg, err = load("EnableAcceptFilter = False\nAcceptFilterBurstTime = 50\nAcceptFilterBurstLimit = 1000\n" +
		"AcceptFilterBanTime = 5\nAcceptFilterAllowlist = 10.0.0.0/8 192.0.2.7\nFastConnectionLimit = 9\n")
	if err != nil {
		t.Fatal(err)
	}
	want := []netip.Prefix{netip.MustParsePrefix("10.0.0.0/8"), netip.MustParsePrefix("192.0.2.7/32")}
	if got := cfg.AcceptFilter; got.Enabled || got.BurstWindow != 50*time.Millisecond || got.BurstLimit != 1000 ||
		got.BanTime != 5*time.Second || !slices.Equal(got.Allowlist, want) {
		t.Fatalf("AcceptFilter = %+v", got)
	}
	if cfg.Flood.FastConnectionLimit != 9 {
		t.Fatalf("Flood.FastConnectionLimit = %d, want 9", cfg.Flood.FastConnectionLimit)
	}

	if _, err := load("AcceptFilterBurstLimit = -1\n"); err == nil {
		t.Fatal("negative burst limit accepted")
	}
}
