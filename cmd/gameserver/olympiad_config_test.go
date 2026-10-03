package main

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/fatal10110/acis_golang/internal/gameserver/olympiad"
)

// shippedOlympiadEvents is lines copied verbatim from the head of the
// reference's shipped config/events.properties: the four calendar keys and
// OlyBattle, which the calendar does not read.
const shippedOlympiadEvents = `#=============================================================
#                          Olympiad
#=============================================================
# Olympiad start time hour, default 18 (6PM).
OlyStartTime = 18

# Olympiad start time minutes, default 00.
OlyMin = 00

# Olympiad competition period, default 6 hours (should be changed by steps of 10mins).
OlyCPeriod = 21600000

# Olympiad battle period, default 6 minutes.
OlyBattle = 360000

# Points allowed every week after first cycle, default: 3.
OlyWeeklyPoints = 3
`

// TestLoadOlympiadConfig pins the Olympiad calendar settings to
// events.properties: the shipped file, with its zero-padded OlyMin, and an
// empty one both give the reference defaults (18, 0, 21600000, 3); set
// keys override them; a malformed value fails the load instead of
// defaulting.
func TestLoadOlympiadConfig(t *testing.T) {
	dir := t.TempDir()
	shipped := olympiad.Config{StartHour: 18, StartMinute: 0, CompetitionMillis: 21600000, WeeklyPoints: 3}
	if def := olympiad.DefaultConfig(); def != shipped {
		t.Fatalf("DefaultConfig() = %+v, want %+v", def, shipped)
	}
	for _, tt := range []struct {
		name, props string
		want        olympiad.Config
		wantErr     bool
	}{
		{name: "shipped", props: shippedOlympiadEvents, want: shipped},
		{name: "empty", props: "", want: olympiad.DefaultConfig()},
		{
			name:  "set",
			props: "OlyStartTime = 20\nOlyMin = 05\nOlyCPeriod = 3600000\nOlyWeeklyPoints = 7\n",
			want:  olympiad.Config{StartHour: 20, StartMinute: 5, CompetitionMillis: 3600000, WeeklyPoints: 7},
		},
		{name: "malformed", props: "OlyCPeriod = abc\n", wantErr: true},
	} {
		path := filepath.Join(dir, tt.name+".properties")
		if err := os.WriteFile(path, []byte(tt.props), 0o600); err != nil {
			t.Fatal(err)
		}
		got, err := loadOlympiadConfig(gameServerPaths{EventsConfigPath: path})
		if tt.wantErr {
			if err == nil {
				t.Errorf("%s: loadOlympiadConfig = %+v, nil; want an error", tt.name, got)
			}
			continue
		}
		if err != nil || got != tt.want {
			t.Errorf("%s: loadOlympiadConfig = %+v, %v; want %+v", tt.name, got, err, tt.want)
		}
	}
}
