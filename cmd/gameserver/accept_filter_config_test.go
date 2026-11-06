package main

import (
	"net/netip"
	"slices"
	"testing"
	"time"

	"github.com/fatal10110/acis_golang/internal/commons/netutil"
	"github.com/fatal10110/acis_golang/internal/config"
)

// TestGameServerConfigReadsTheAcceptFilter pins the game-client accept
// filter keys of server.properties: absent, the defaults; set, each value;
// malformed, a config error.
func TestGameServerConfigReadsTheAcceptFilter(t *testing.T) {
	hexProps, err := config.ParseString("ServerID = 3\nHexID = -7fff\n")
	if err != nil {
		t.Fatal(err)
	}
	load := func(text string) (gameServerConfig, error) {
		t.Helper()
		props, err := config.ParseString(text)
		if err != nil {
			t.Fatal(err)
		}
		return gameServerConfigFromProperties(gameServerPaths{}, props, hexProps)
	}

	cfg, err := load("GameserverPort = 7777\n")
	if err != nil {
		t.Fatal(err)
	}
	def := netutil.DefaultAcceptFilterConfig()
	if got := cfg.AcceptFilter; got.Enabled != def.Enabled || got.BurstWindow != def.BurstWindow ||
		got.BurstLimit != def.BurstLimit || got.BanTime != def.BanTime || got.Allowlist != nil {
		t.Fatalf("AcceptFilter with no keys = %+v, want the defaults", got)
	}

	cfg, err = load("EnableAcceptFilter = False\nAcceptFilterBurstTime = 50\nAcceptFilterBurstLimit = 1000\n" +
		"AcceptFilterBanTime = 5\nAcceptFilterAllowlist = 10.0.0.0/8\n")
	if err != nil {
		t.Fatal(err)
	}
	if got := cfg.AcceptFilter; got.Enabled || got.BurstWindow != 50*time.Millisecond || got.BurstLimit != 1000 ||
		got.BanTime != 5*time.Second || !slices.Equal(got.Allowlist, []netip.Prefix{netip.MustParsePrefix("10.0.0.0/8")}) {
		t.Fatalf("AcceptFilter = %+v", got)
	}

	if _, err := load("AcceptFilterAllowlist = not-an-ip\n"); err == nil {
		t.Fatal("malformed allowlist accepted")
	}
}
