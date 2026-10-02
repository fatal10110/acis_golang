package main

import (
	"os"
	"path/filepath"
	"testing"
)

// TestLoadAdminConfigGMStartup pins the GM startup modes to
// players.properties with their shipped defaults: listed on /gmlist, and
// neither invulnerable, invisible nor blocking everything.
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
			"set", "GMStartupInvulnerable = True\nGMStartupInvisible = True\nGMStartupBlockAll = True\nGMStartupAutoList = False\n",
			adminConfig{GMStartupInvulnerable: true, GMStartupInvisible: true, GMStartupBlockAll: true},
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
