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

FestivalMinPlayer = 5
FestivalFirstSpawn = 120000
FestivalFirstSwarm = 300000
FestivalSecondSpawn = 540000
FestivalSecondSwarm = 720000
FestivalChestSpawn = 900000
`

// TestLoadFestivalConfig pins the festival settings to events.properties,
// the times in milliseconds: the shipped file and an empty one both give
// the reference defaults (120000, 1080000, 2280000, a party of 5, monsters
// at 120000, 300000, 540000, 720000 and 900000); set keys override them, the
// party minimum held to 2 to 9; a malformed value fails the load instead of
// defaulting.
func TestLoadFestivalConfig(t *testing.T) {
	dir := t.TempDir()
	shipped := festival.Config{
		ManagerStart: 2 * time.Minute, Length: 18 * time.Minute, CycleLength: 38 * time.Minute, MinPlayers: 5,
		FirstSpawn: 2 * time.Minute, FirstSwarm: 5 * time.Minute, SecondSpawn: 9 * time.Minute, SecondSwarm: 12 * time.Minute, ChestSpawn: 15 * time.Minute,
	}
	set := shipped
	set.ManagerStart, set.Length, set.CycleLength, set.MinPlayers, set.ChestSpawn = time.Second, time.Minute, 3*time.Minute, 9, time.Second
	low := shipped
	low.MinPlayers = 2
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
			props: "FestivalManagerStart = 1000\nFestivalLength = 60000\nFestivalCycleLength = 180000\nFestivalMinPlayer = 12\nFestivalChestSpawn = 1000\n",
			want:  set,
		},
		{name: "low", props: "FestivalMinPlayer = 1\n", want: low},
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
