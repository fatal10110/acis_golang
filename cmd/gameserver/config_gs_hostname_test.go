package main

import (
	"strings"
	"testing"

	"github.com/fatal10110/acis_golang/internal/config"
)

// TestGameServerConfigRequiresGameserverHostname: an absent
// GameserverHostname aborts boot instead of listening on every address
// (#4776); only an explicit "*" binds every address. A config reload does
// not rebind the listener and keeps accepting a file without the key
// (TestDataReloadsReadConfig).
func TestGameServerConfigRequiresGameserverHostname(t *testing.T) {
	props, err := config.ParseString("ServerID = 1\nHexID = -7fff\n")
	if err != nil {
		t.Fatal(err)
	}
	hexProps := hexIDProperties{Props: props}

	absent, err := config.ParseString("GameserverPort = 7777\n")
	if err != nil {
		t.Fatal(err)
	}
	cfg, err := gameServerConfigFromLoadedProperties(gameServerPaths{}, absent, hexProps)
	if err == nil {
		t.Fatalf("absent GameserverHostname: got config with ListenAddr %q, want error", cfg.ListenAddr)
	}
	if !strings.Contains(err.Error(), "GameserverHostname") {
		t.Fatalf("absent GameserverHostname: error %q does not name the key", err)
	}

	wildcard, err := config.ParseString("GameserverHostname = *\nGameserverPort = 7777\n")
	if err != nil {
		t.Fatal(err)
	}
	cfg, err = gameServerConfigFromLoadedProperties(gameServerPaths{}, wildcard, hexProps)
	if err != nil {
		t.Fatalf("explicit *: %v", err)
	}
	if cfg.ListenAddr != ":7777" {
		t.Fatalf("explicit *: ListenAddr = %q, want %q", cfg.ListenAddr, ":7777")
	}
}
