package main

import (
	"os"
	"path/filepath"
	"testing"
)

// TestLoadHeroMinMatches pins OlyMinMatchesToBeClassed: the shipped 9, the
// reference default 5 when unset, and a malformed value failing the load.
func TestLoadHeroMinMatches(t *testing.T) {
	dir := t.TempDir()
	for _, tt := range []struct {
		name, props string
		want        int
		wantErr     bool
	}{
		{name: "shipped", props: "# Minimum matches.\nOlyMinMatchesToBeClassed = 9\n", want: 9},
		{name: "empty", props: "", want: 5},
		{name: "malformed", props: "OlyMinMatchesToBeClassed = nine\n", wantErr: true},
	} {
		path := filepath.Join(dir, tt.name+".properties")
		if err := os.WriteFile(path, []byte(tt.props), 0o600); err != nil {
			t.Fatal(err)
		}
		got, err := loadHeroMinMatches(gameServerPaths{EventsConfigPath: path})
		if tt.wantErr {
			if err == nil {
				t.Errorf("%s: loadHeroMinMatches = %d, nil; want an error", tt.name, got)
			}
			continue
		}
		if err != nil || got != tt.want {
			t.Errorf("%s: loadHeroMinMatches = %d, %v; want %d", tt.name, got, err, tt.want)
		}
	}
}
