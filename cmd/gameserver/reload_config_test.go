package main

import (
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/rs/zerolog"
)

// TestDataReloadsReadConfig requires the config reload hook to read the
// server and players properties anew into the player settings, and to fail
// on a value boot would refuse.
func TestDataReloadsReadConfig(t *testing.T) {
	dir := t.TempDir()
	write := func(name, body string) string {
		t.Helper()
		path := filepath.Join(dir, name)
		if err := os.WriteFile(path, []byte(body), 0o600); err != nil {
			t.Fatal(err)
		}
		return path
	}
	paths := gameServerPaths{
		ConfigPath:        write("server.properties", "AllowDiscardItem = True\n"),
		HexIDPath:         write("hexid.txt", "ServerID = 3\nHexID = -7fff\n"),
		PlayersConfigPath: write("players.properties", "KarmaPlayerCanTrade = True\nPartyRange = 900\n"),
		NpcsConfigPath:    write("npcs.properties", ""),
		GeoConfigPath:     write("geoengine.properties", ""),
	}
	reloads := provideDataReloads(paths, gameServerConfig{}, &gameData{}, nil, nil, nil, nil, gameplayConfig{}, zerolog.Nop())

	settings, err := reloads.Config()
	if err != nil {
		t.Fatalf("config reload: %v", err)
	}
	if settings.DiscardItemDisabled || !settings.KarmaPlayerCanTrade || settings.PartyRange != 900 {
		t.Fatalf("reloaded settings = discard disabled %v, karma trade %v, party range %d; want false, true, 900",
			settings.DiscardItemDisabled, settings.KarmaPlayerCanTrade, settings.PartyRange)
	}

	write("server.properties", "AllowDiscardItem = False\nServerBypassTime = 2500\n")
	write("players.properties", "KarmaPlayerCanTrade = False\nPartyRange = 300\n")
	settings, err = reloads.Config()
	if err != nil {
		t.Fatalf("second config reload: %v", err)
	}
	if !settings.DiscardItemDisabled || settings.KarmaPlayerCanTrade || settings.PartyRange != 300 || settings.ServerBypassDelay != 2500*time.Millisecond {
		t.Fatalf("reloaded settings = discard disabled %v, karma trade %v, party range %d, bypass delay %s; want the edited files' values",
			settings.DiscardItemDisabled, settings.KarmaPlayerCanTrade, settings.PartyRange, settings.ServerBypassDelay)
	}

	write("players.properties", "PartyRange = far\n")
	if _, err := reloads.Config(); err == nil {
		t.Fatal("config reload of a malformed value succeeded")
	}
}
