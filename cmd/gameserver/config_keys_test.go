package main

import (
	"bytes"
	"encoding/json"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"testing"
	"time"

	"github.com/rs/zerolog"
)

// writeProps writes body as a properties file and returns its path.
func writeProps(t *testing.T, body string) string {
	t.Helper()
	path := filepath.Join(t.TempDir(), "file.properties")
	if err := os.WriteFile(path, []byte(body), 0o600); err != nil {
		t.Fatal(err)
	}
	return path
}

// TestLoadServerSwitches pins server.properties DropItemTime (ms, stock
// 1000), AllowWarehouse (stock True) and NoSpawns (stock False): read as
// set, the default when absent, a boolean false unless it reads "true".
func TestLoadServerSwitches(t *testing.T) {
	for _, tc := range []struct {
		name      string
		body      string
		drop      dropItemDelay
		warehouse warehouseAllowed
		none      noSpawns
	}{
		{"absent", "", dropItemDelay(time.Second), true, false},
		{"set", "DropItemTime = 250\nAllowWarehouse = False\nNoSpawns = True\n", dropItemDelay(250 * time.Millisecond), false, true},
		{"off", "DropItemTime = 0\nAllowWarehouse = yes\nNoSpawns = TRUE\n", 0, false, true},
	} {
		t.Run(tc.name, func(t *testing.T) {
			paths := gameServerPaths{ConfigPath: writeProps(t, tc.body)}
			if got, err := loadDropItemDelay(paths); err != nil || got != tc.drop {
				t.Fatalf("loadDropItemDelay() = %v, %v; want %v", got, err, tc.drop)
			}
			if got, err := loadWarehouseAllowed(paths); err != nil || got != tc.warehouse {
				t.Fatalf("loadWarehouseAllowed() = %v, %v; want %v", got, err, tc.warehouse)
			}
			if got, err := loadNoSpawns(paths); err != nil || got != tc.none {
				t.Fatalf("loadNoSpawns() = %v, %v; want %v", got, err, tc.none)
			}
		})
	}
	for _, body := range []string{"DropItemTime = soon\n", "DropItemTime = 2147483648\n"} {
		if _, err := loadDropItemDelay(gameServerPaths{ConfigPath: writeProps(t, body)}); err == nil {
			t.Fatalf("loadDropItemDelay(%q) = nil error, want one", body)
		}
	}
}

// TestLoadFakeDeathUpProtection pins players.properties
// PlayerFakeDeathUpProtection: seconds, stock 5; the milliseconds are an
// int32 product, so a value past 2147483 seconds wraps, and a wrapped
// negative grace is none; a value outside int32 or malformed fails.
func TestLoadFakeDeathUpProtection(t *testing.T) {
	for _, tc := range []struct {
		name, body string
		want       time.Duration
	}{
		{"absent", "", 5 * time.Second},
		{"set", "PlayerFakeDeathUpProtection = 12\n", 12 * time.Second},
		{"off", "PlayerFakeDeathUpProtection = 0\n", 0},
		{"largest unwrapped", "PlayerFakeDeathUpProtection = 2147483\n", 2147483 * time.Second},
		{"wraps negative", "PlayerFakeDeathUpProtection = 2147484\n", -2147483296 * time.Millisecond},
		{"wraps positive", "PlayerFakeDeathUpProtection = 4294968\n", 704 * time.Millisecond},
	} {
		t.Run(tc.name, func(t *testing.T) {
			got, err := loadFakeDeathUpProtection(gameServerPaths{PlayersConfigPath: writeProps(t, tc.body)})
			if err != nil || got != fakeDeathUpProtection(tc.want) {
				t.Fatalf("loadFakeDeathUpProtection() = %v, %v; want %v", time.Duration(got), err, tc.want)
			}
		})
	}
	for _, body := range []string{"PlayerFakeDeathUpProtection = 2147483648\n", "PlayerFakeDeathUpProtection = five\n"} {
		if _, err := loadFakeDeathUpProtection(gameServerPaths{PlayersConfigPath: writeProps(t, body)}); err == nil {
			t.Fatalf("loadFakeDeathUpProtection(%q) = nil error, want one", body)
		}
	}
}

// TestLoadShowSummonCrest pins npcs.properties ShowSummonCrest: read as
// set, false when absent.
func TestLoadShowSummonCrest(t *testing.T) {
	for _, tc := range []struct {
		name, body string
		want       summonCrestShown
	}{
		{"set", "ShowSummonCrest = True\n", true},
		{"cleared", "ShowSummonCrest = False\n", false},
		{"absent", "", false},
	} {
		t.Run(tc.name, func(t *testing.T) {
			got, err := loadShowSummonCrest(gameServerPaths{NpcsConfigPath: writeProps(t, tc.body)})
			if err != nil || got != tc.want {
				t.Fatalf("loadShowSummonCrest() = %v, %v; want %v", got, err, tc.want)
			}
		})
	}
}

// TestReportConfigKeysCoversEveryFile pins the boot report over the seven
// properties files the server reads: each file's unread keys are logged
// once, an ignored one as having no effect, and a missing file fails boot.
func TestReportConfigKeysCoversEveryFile(t *testing.T) {
	dir := t.TempDir()
	paths := gameServerPaths{}
	for _, f := range []struct {
		path *string
		name string
	}{
		{&paths.ClansConfigPath, "clans"},
		{&paths.EventsConfigPath, "events"},
		{&paths.GeoConfigPath, "geoengine"},
		{&paths.NpcsConfigPath, "npcs"},
		{&paths.PlayersConfigPath, "players"},
		{&paths.ConfigPath, "server"},
		{&paths.SiegeConfigPath, "siege"},
	} {
		*f.path = filepath.Join(dir, f.name+".properties")
		body := f.name + "Typo = 1\n"
		if f.name == "server" {
			body += "Developer = True\n"
		}
		if err := os.WriteFile(*f.path, []byte(body), 0o600); err != nil {
			t.Fatal(err)
		}
	}
	var out bytes.Buffer
	if err := reportConfigKeys(paths, zerolog.New(&out)); err != nil {
		t.Fatalf("reportConfigKeys() error = %v", err)
	}
	var got []string
	for line := range strings.Lines(out.String()) {
		var entry struct{ Level, File, Key string }
		if err := json.Unmarshal([]byte(line), &entry); err != nil {
			t.Fatalf("report line %q: %v", line, err)
		}
		got = append(got, entry.Level+" "+entry.File+" "+entry.Key)
	}
	want := []string{
		"warn clans.properties clansTypo", "warn events.properties eventsTypo", "warn geoengine.properties geoengineTypo",
		"warn npcs.properties npcsTypo", "warn players.properties playersTypo", "warn server.properties serverTypo",
		"info server.properties Developer", "warn siege.properties siegeTypo",
	}
	if !slices.Equal(got, want) {
		t.Fatalf("report = %q, want %q", got, want)
	}

	paths.SiegeConfigPath = filepath.Join(dir, "missing.properties")
	if err := reportConfigKeys(paths, zerolog.Nop()); err == nil {
		t.Fatal("reportConfigKeys() with a missing file = nil error, want one")
	}
}
