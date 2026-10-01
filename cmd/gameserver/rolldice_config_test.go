package main

import (
	"os"
	"path/filepath"
	"testing"
	"time"
)

// TestLoadRollDiceDelay pins the dice reuse delay to server.properties
// RollDiceTime with its shipped default of 4200 ms; 0 turns the gate off.
func TestLoadRollDiceDelay(t *testing.T) {
	dir := t.TempDir()
	for _, tt := range []struct {
		name, props string
		want        time.Duration
	}{
		{"default", "", 4200 * time.Millisecond},
		{"set", "RollDiceTime = 750\n", 750 * time.Millisecond},
		{"disabled", "RollDiceTime = 0\n", 0},
	} {
		path := filepath.Join(dir, tt.name+".properties")
		if err := os.WriteFile(path, []byte(tt.props), 0o600); err != nil {
			t.Fatal(err)
		}
		got, err := loadRollDiceDelay(gameServerPaths{ConfigPath: path})
		if err != nil || time.Duration(got) != tt.want {
			t.Errorf("%s: loadRollDiceDelay = %v, %v; want %v", tt.name, time.Duration(got), err, tt.want)
		}
	}
}
