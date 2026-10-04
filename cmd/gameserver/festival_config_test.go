package main

import (
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/fatal10110/acis_golang/internal/gameserver/festival"
)

// shippedFestivalEvents is the festival schedule keys copied verbatim from
// the reference's shipped config/events.properties.
const shippedFestivalEvents = `# Festival Manager Start time.
#   Default : 2 minutes
FestivalManagerStart = 120000

# Festival Length.
#   Default : 18 minutes
FestivalLength = 1080000

# Festival Cycle Length.
#   Default : 38 Minutes (20 minutes wait time, + Festival time) 
FestivalCycleLength = 2280000
`

// TestLoadFestivalConfig pins the festival schedule settings to
// events.properties, in milliseconds: the shipped file and an empty one
// both give the reference defaults (120000, 1080000, 2280000); set keys
// override them; a malformed value fails the load instead of defaulting.
func TestLoadFestivalConfig(t *testing.T) {
	dir := t.TempDir()
	shipped := festival.Config{ManagerStart: 2 * time.Minute, Length: 18 * time.Minute, CycleLength: 38 * time.Minute}
	if def := festival.DefaultConfig(); def != shipped {
		t.Fatalf("DefaultConfig() = %+v, want %+v", def, shipped)
	}
	for _, tt := range []struct {
		name, props string
		want        festival.Config
		wantErr     bool
	}{
		{name: "shipped", props: shippedFestivalEvents, want: shipped},
		{name: "empty", props: "", want: shipped},
		{
			name:  "set",
			props: "FestivalManagerStart = 1000\nFestivalLength = 60000\nFestivalCycleLength = 180000\n",
			want:  festival.Config{ManagerStart: time.Second, Length: time.Minute, CycleLength: 3 * time.Minute},
		},
		{name: "malformed", props: "FestivalLength = abc\n", wantErr: true},
	} {
		path := filepath.Join(dir, tt.name+".properties")
		if err := os.WriteFile(path, []byte(tt.props), 0o600); err != nil {
			t.Fatal(err)
		}
		got, err := loadFestivalConfig(gameServerPaths{EventsConfigPath: path})
		if tt.wantErr {
			if err == nil {
				t.Errorf("%s: loadFestivalConfig = %+v, nil; want an error", tt.name, got)
			}
			continue
		}
		if err != nil || got != tt.want {
			t.Errorf("%s: loadFestivalConfig = %+v, %v; want %+v", tt.name, got, err, tt.want)
		}
	}
}
