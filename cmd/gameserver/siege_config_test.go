package main

import (
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/fatal10110/acis_golang/internal/gameserver/clanhall"
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

// TestLoadSiegeConfig pins the siege.properties reads: the shipped values,
// the reference defaults for a missing key (SiegeLength 120 minutes, level
// 4, 10 clans a side, 10000 ms; level 4 and 10 attackers for the halls),
// and a malformed number failing the boot as Integer.parseInt does.
func TestLoadSiegeConfig(t *testing.T) {
	dir := t.TempDir()
	paths := func(content string) gameServerPaths {
		t.Helper()
		path := filepath.Join(dir, "siege.properties")
		if err := os.WriteFile(path, []byte(content), 0o600); err != nil {
			t.Fatal(err)
		}
		return gameServerPaths{SiegeConfigPath: path}
	}
	shipped := siege.Config{Length: 2 * time.Hour, MinClanLevel: 4, MaxAttackers: 10, MaxDefenders: 10, AttackerRespawn: 10 * time.Second}
	shippedHall := clanhall.SiegeConfig{MinClanLevel: 4, MaxAttackers: 10}
	for _, content := range []string{shippedSiege, ""} {
		if got, err := loadSiegeConfig(paths(content)); err != nil || got != shipped {
			t.Fatalf("loadSiegeConfig(%q) = %+v, %v; want %+v", content, got, err, shipped)
		}
		if got, err := loadHallSiegeConfig(paths(content)); err != nil || got != shippedHall {
			t.Fatalf("loadHallSiegeConfig(%q) = %+v, %v; want %+v", content, got, err, shippedHall)
		}
	}
	custom := paths("SiegeLength = 90\nSiegeClanMinLevel = 5\nAttackerMaxClans = 3\nDefenderMaxClans = 2\nAttackerRespawn = 2500\nChSiegeClanMinLevel = 6\nChAttackerMaxClans = 7\n")
	if got, err := loadSiegeConfig(custom); err != nil || got != (siege.Config{Length: 90 * time.Minute, MinClanLevel: 5, MaxAttackers: 3, MaxDefenders: 2, AttackerRespawn: 2500 * time.Millisecond}) {
		t.Fatalf("loadSiegeConfig(custom) = %+v, %v", got, err)
	}
	if got, err := loadHallSiegeConfig(custom); err != nil || got != (clanhall.SiegeConfig{MinClanLevel: 6, MaxAttackers: 7}) {
		t.Fatalf("loadHallSiegeConfig(custom) = %+v, %v", got, err)
	}
	if _, err := loadSiegeConfig(paths("SiegeLength = two hours\n")); err == nil {
		t.Fatal("a malformed SiegeLength loaded")
	}
	if _, err := loadHallSiegeConfig(paths("ChAttackerMaxClans = ten\n")); err == nil {
		t.Fatal("a malformed ChAttackerMaxClans loaded")
	}
}
