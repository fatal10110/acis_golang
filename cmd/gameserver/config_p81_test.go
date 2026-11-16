package main

import (
	"os"
	"path/filepath"
	"slices"
	"testing"

	"github.com/fatal10110/acis_golang/internal/config"
)

// TestRateDropAdenaIsNotACurrencyRate: only RateDropCurrency sets the
// currency drop rate; RateDropAdena alone leaves it at 1 and is reported
// as an unsupported key.
func TestRateDropAdenaIsNotACurrencyRate(t *testing.T) {
	players := filepath.Join(t.TempDir(), "players.properties")
	if err := os.WriteFile(players, nil, 0o600); err != nil {
		t.Fatal(err)
	}
	serverProps, err := config.ParseString("RateDropAdena = 5\n")
	if err != nil {
		t.Fatal(err)
	}
	settings, err := loadKillRewardSettings(gameServerPaths{PlayersConfigPath: players}, serverProps)
	if err != nil {
		t.Fatal(err)
	}
	if settings.Rates.Currency != 1 {
		t.Errorf("currency rate = %v, want 1 with only RateDropAdena set", settings.Rates.Currency)
	}
	unsupported, _ := config.UnreadKeys(config.GameServer, "server.properties", serverProps)
	if !slices.Contains(unsupported, "RateDropAdena") {
		t.Errorf("unsupported keys = %v, want RateDropAdena", unsupported)
	}
}

// TestHexIDServerIDIsInt32: the hexid file's ServerID must fit in 32 bits.
func TestHexIDServerIDIsInt32(t *testing.T) {
	serverProps, err := config.ParseString("GameserverHostname = *\n")
	if err != nil {
		t.Fatal(err)
	}
	for _, tc := range []struct {
		id      string
		wantErr bool
	}{
		{"2147483647", false},
		{"2147483648", true},
	} {
		hexProps, err := config.ParseString("ServerID = " + tc.id + "\nHexID = -7fff\n")
		if err != nil {
			t.Fatal(err)
		}
		cfg, err := gameServerConfigFromProperties(gameServerPaths{}, serverProps, hexProps)
		if (err != nil) != tc.wantErr {
			t.Errorf("ServerID %s: error = %v, want error %v", tc.id, err, tc.wantErr)
		}
		if err == nil && cfg.Auth.ServerID != 2147483647 {
			t.Errorf("ServerID %s: got %d", tc.id, cfg.Auth.ServerID)
		}
	}
}
