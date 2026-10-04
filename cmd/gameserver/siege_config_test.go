package main

import (
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/fatal10110/acis_golang/internal/gameserver/siege"
)

// shippedSiege is the reference's shipped config/siege.properties, values
// verbatim.
const shippedSiege = `SiegeLength = 120
SiegeClanMinLevel = 4
AttackerMaxClans = 10
DefenderMaxClans = 10
AttackerRespawn = 10000
ChSiegeClanMinLevel = 4
ChAttackerMaxClans = 10
`

// TestLoadSiegeConfig pins Config.loadSieges: the shipped values, the
// reference defaults for a missing key (SiegeLength 120 minutes, level 4,
// 10 clans a side, 10000 ms), and a malformed number failing the boot as
// Integer.parseInt does.
func TestLoadSiegeConfig(t *testing.T) {
	dir := t.TempDir()
	load := func(content string) (siege.Config, error) {
		t.Helper()
		path := filepath.Join(dir, "siege.properties")
		if err := os.WriteFile(path, []byte(content), 0o600); err != nil {
			t.Fatal(err)
		}
		return loadSiegeConfig(gameServerPaths{SiegeConfigPath: path})
	}
	shipped := siege.Config{Length: 2 * time.Hour, MinClanLevel: 4, MaxAttackers: 10, MaxDefenders: 10, AttackerRespawn: 10 * time.Second}
	for _, content := range []string{shippedSiege, ""} {
		got, err := load(content)
		if err != nil || got != shipped {
			t.Fatalf("loadSiegeConfig(%q) = %+v, %v; want %+v", content, got, err, shipped)
		}
	}
	got, err := load("SiegeLength = 90\nSiegeClanMinLevel = 5\nAttackerMaxClans = 3\nDefenderMaxClans = 2\nAttackerRespawn = 2500\n")
	if want := (siege.Config{Length: 90 * time.Minute, MinClanLevel: 5, MaxAttackers: 3, MaxDefenders: 2, AttackerRespawn: 2500 * time.Millisecond}); err != nil || got != want {
		t.Fatalf("loadSiegeConfig(custom) = %+v, %v; want %+v", got, err, want)
	}
	if _, err := load("SiegeLength = two hours\n"); err == nil {
		t.Fatal("a malformed SiegeLength loaded")
	}
}
