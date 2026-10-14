package main

import (
	"os"
	"path/filepath"
	"testing"
	"time"
)

// TestLoadSocialDelay pins the emote reuse delay to server.properties
// SocialTime with its shipped default of 2000 ms; 0 turns the gate off.
func TestLoadSocialDelay(t *testing.T) {
	dir := t.TempDir()
	for _, tt := range []struct {
		name, props string
		want        time.Duration
	}{
		{"default", "", 2000 * time.Millisecond},
		{"set", "SocialTime = 750\n", 750 * time.Millisecond},
		{"disabled", "SocialTime = 0\n", 0},
	} {
		path := filepath.Join(dir, tt.name+".properties")
		if err := os.WriteFile(path, []byte(tt.props), 0o600); err != nil {
			t.Fatal(err)
		}
		got, err := loadSocialDelay(gameServerPaths{ConfigPath: path})
		if err != nil || time.Duration(got) != tt.want {
			t.Errorf("%s: loadSocialDelay = %v, %v; want %v", tt.name, time.Duration(got), err, tt.want)
		}
	}
}
