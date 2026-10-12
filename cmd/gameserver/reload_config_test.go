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
// on a value boot would refuse, PartyRange among them: the reload keeps the
// party range in force, but a malformed one still fails the whole read.
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
		PlayersConfigPath: write("players.properties", "KarmaPlayerCanTrade = True\n"),
		NpcsConfigPath:    write("npcs.properties", ""),
		GeoConfigPath:     write("geoengine.properties", ""),
	}
	reloads := provideDataReloads(paths, gameServerConfig{}, &gameData{}, nil, nil, nil, nil, gameplayConfig{}, zerolog.Nop())

	settings, err := reloads.Config()
	if err != nil {
		t.Fatalf("config reload: %v", err)
	}
	if settings.DiscardItemDisabled || !settings.KarmaPlayerCanTrade {
		t.Fatalf("reloaded settings = discard disabled %v, karma trade %v; want false, true",
			settings.DiscardItemDisabled, settings.KarmaPlayerCanTrade)
	}

	write("server.properties", "AllowDiscardItem = False\nServerBypassTime = 2500\n")
	write("players.properties", "KarmaPlayerCanTrade = False\n")
	settings, err = reloads.Config()
	if err != nil {
		t.Fatalf("second config reload: %v", err)
	}
	if !settings.DiscardItemDisabled || settings.KarmaPlayerCanTrade || settings.ServerBypassDelay != 2500*time.Millisecond {
		t.Fatalf("reloaded settings = discard disabled %v, karma trade %v, bypass delay %s; want the edited files' values",
			settings.DiscardItemDisabled, settings.KarmaPlayerCanTrade, settings.ServerBypassDelay)
	}

	write("players.properties", "PartyRange = far\n")
	if _, err := reloads.Config(); err == nil {
		t.Fatal("config reload of a malformed value succeeded")
	}
}
