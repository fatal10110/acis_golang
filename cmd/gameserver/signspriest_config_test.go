package main

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/fatal10110/acis_golang/internal/gameserver/signspriest"
)

// TestLoadSignsPriestConfig pins the Seven Signs dialog settings to
// events.properties: the shipped file and an empty one both give the
// reference defaults (MaxPlayerContrib 1000000, prerequisites enforced);
// set keys override them; a malformed value fails the load.
func TestLoadSignsPriestConfig(t *testing.T) {
	dir := t.TempDir()
	shipped := signspriest.Config{MaxPlayerContrib: 1000000}
	if def := signspriest.DefaultConfig(); def != shipped {
		t.Fatalf("DefaultConfig() = %+v, want %+v", def, shipped)
	}
	for _, tt := range []struct {
		name, props string
		want        signspriest.Config
		wantErr     bool
	}{
		{name: "shipped", props: "SevenSignsBypassPrerequisites = False\nMaxPlayerContrib = 1000000\n", want: shipped},
		{name: "empty", want: shipped},
		{name: "set", props: "SevenSignsBypassPrerequisites = True\nMaxPlayerContrib = 500\n", want: signspriest.Config{MaxPlayerContrib: 500, BypassPrerequisites: true}},
		{name: "malformed", props: "MaxPlayerContrib = lots\n", wantErr: true},
	} {
		path := filepath.Join(dir, tt.name+".properties")
		if err := os.WriteFile(path, []byte(tt.props), 0o600); err != nil {
			t.Fatal(err)
		}
		got, err := loadSignsPriestConfig(gameServerPaths{EventsConfigPath: path})
		if tt.wantErr {
			if err == nil {
				t.Errorf("%s: loadSignsPriestConfig = %+v, nil; want an error", tt.name, got)
			}
			continue
		}
		if err != nil || got != tt.want {
			t.Errorf("%s: loadSignsPriestConfig = %+v, %v; want %+v", tt.name, got, err, tt.want)
		}
	}
}
