package main

import (
	"os"
	"path/filepath"
	"testing"
)

// TestLoadAdminConfigGMStartup pins the GM startup modes to
// players.properties with their shipped defaults: listed on /gmlist,
// neither invulnerable, invisible nor blocking everything, and without the
// hero aura.
func TestLoadAdminConfigGMStartup(t *testing.T) {
	dir := t.TempDir()
	server := filepath.Join(dir, "server.properties")
	if err := os.WriteFile(server, nil, 0o600); err != nil {
		t.Fatal(err)
	}
	for _, tt := range []struct {
		name, props string
		want        adminConfig
	}{
		{"default", "", adminConfig{GMStartupAutoList: true}},
		{
			"set", "GMStartupInvulnerable = True\nGMStartupInvisible = True\nGMStartupBlockAll = True\nGMStartupAutoList = False\nGMHeroAura = True\n",
			adminConfig{GMStartupInvulnerable: true, GMStartupInvisible: true, GMStartupBlockAll: true, GMHeroAura: true},
		},
	} {
		path := filepath.Join(dir, tt.name+".properties")
		if err := os.WriteFile(path, []byte(tt.props), 0o600); err != nil {
			t.Fatal(err)
		}
		got, err := loadAdminConfig(gameServerPaths{ConfigPath: server, PlayersConfigPath: path})
		if err != nil || got != tt.want {
			t.Errorf("%s: loadAdminConfig = %+v, %v; want %+v", tt.name, got, err, tt.want)
		}
	}
}

// TestLoadAdminConfigDefaultAccessLevel pins players.properties
// DefaultAccessLevel: 0 when absent, the set level otherwise, and a
// malformed value fails the load.
func TestLoadAdminConfigDefaultAccessLevel(t *testing.T) {
	dir := t.TempDir()
	server := filepath.Join(dir, "server.properties")
	if err := os.WriteFile(server, nil, 0o600); err != nil {
		t.Fatal(err)
	}
	for _, tt := range []struct {
		name, props string
		want        int
		wantErr     bool
	}{
		{"absent", "", 0, false},
		{"master", "DefaultAccessLevel = 8\n", 8, false},
		{"banned", "DefaultAccessLevel = -1\n", -1, false},
		{"malformed", "DefaultAccessLevel = eight\n", 0, true},
	} {
		path := filepath.Join(dir, tt.name+".properties")
		if err := os.WriteFile(path, []byte(tt.props), 0o600); err != nil {
			t.Fatal(err)
		}
		got, err := loadAdminConfig(gameServerPaths{ConfigPath: server, PlayersConfigPath: path})
		if (err != nil) != tt.wantErr {
			t.Fatalf("%s: loadAdminConfig err = %v, want error %v", tt.name, err, tt.wantErr)
		}
		if err == nil && got.DefaultAccessLevel != tt.want {
			t.Errorf("%s: DefaultAccessLevel = %d, want %d", tt.name, got.DefaultAccessLevel, tt.want)
		}
	}
}
