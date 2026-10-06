package main

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/fatal10110/acis_golang/internal/config"
)

// TestKillRewardConfigReadsMultipleItemDrop pins server.properties
// MultipleItemDrop into the kill rewards: the shipped default is on, and
// False turns it off.
func TestKillRewardConfigReadsMultipleItemDrop(t *testing.T) {
	dir := t.TempDir()
	players := filepath.Join(dir, "players.properties")
	if err := os.WriteFile(players, nil, 0o600); err != nil {
		t.Fatal(err)
	}
	for _, tc := range []struct {
		body string
		want bool
	}{{"", true}, {"MultipleItemDrop = False\n", false}, {"MultipleItemDrop = True\n", true}} {
		server := filepath.Join(dir, "server.properties")
		if err := os.WriteFile(server, []byte(tc.body), 0o600); err != nil {
			t.Fatal(err)
		}
		props, err := config.LoadFile(server)
		if err != nil {
			t.Fatal(err)
		}
		got, err := provideKillRewardConfig(gameServerPaths{PlayersConfigPath: players}, props, &gameData{})
		if err != nil {
			t.Fatalf("provideKillRewardConfig(%q) error = %v", tc.body, err)
		}
		if got.MultipleItemDrop != tc.want {
			t.Fatalf("provideKillRewardConfig(%q).MultipleItemDrop = %v, want %v", tc.body, got.MultipleItemDrop, tc.want)
		}
	}
}

// TestKillRewardConfigReadsExpSpRates pins server.properties RateXp and
// RateSp into the kill rewards, defaulting to 1 when absent.
func TestKillRewardConfigReadsExpSpRates(t *testing.T) {
	dir := t.TempDir()
	players := filepath.Join(dir, "players.properties")
	if err := os.WriteFile(players, nil, 0o600); err != nil {
		t.Fatal(err)
	}
	for _, tc := range []struct {
		body   string
		xp, sp float64
	}{{"", 1, 1}, {"RateXp = 100.\nRateSp = 2.5\n", 100, 2.5}} {
		server := filepath.Join(dir, "server.properties")
		if err := os.WriteFile(server, []byte(tc.body), 0o600); err != nil {
			t.Fatal(err)
		}
		props, err := config.LoadFile(server)
		if err != nil {
			t.Fatal(err)
		}
		got, err := provideKillRewardConfig(gameServerPaths{PlayersConfigPath: players}, props, &gameData{})
		if err != nil {
			t.Fatalf("provideKillRewardConfig(%q) error = %v", tc.body, err)
		}
		if got.RateXP != tc.xp || got.RateSP != tc.sp {
			t.Fatalf("provideKillRewardConfig(%q) rates = %v/%v, want %v/%v", tc.body, got.RateXP, got.RateSP, tc.xp, tc.sp)
		}
	}
}
