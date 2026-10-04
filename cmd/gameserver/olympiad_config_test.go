package main

import (
	"os"
	"path/filepath"
	"reflect"
	"testing"

	"github.com/fatal10110/acis_golang/internal/gameserver/olympiad"
)

// shippedOlympiadEvents is lines copied verbatim from the head of the
// reference's shipped config/events.properties: the four calendar keys,
// OlyBattle, which the calendar does not read, and the match keys.
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

# Reward for the class based games.
# Format: itemId1-itemNum1;itemId2-itemNum2...
# Default: 6651-50
OlyClassedReward = 6651-50

# Reward for the non-class based games.
# Format: itemId1-itemNum1;itemId2-itemNum2...
# Default: 6651-30
OlyNonClassedReward = 6651-30

# Maximum points that player can gain/lose on a match, default: 10.
OlyMaxPoints = 10

# Divider for points in classed and non-classed games, default: 3, 5.
OlyDividerClassed = 3
OlyDividerNonClassed = 5
`

// TestLoadOlympiadConfig pins the Olympiad settings to events.properties:
// the shipped file, with its zero-padded OlyMin, and an empty one both give
// the reference defaults (18, 0, 21600000, 3; 10 points at most, dividers 3
// and 5, rewards 6651x50 and 6651x30); set keys override them; a malformed
// number fails the load instead of defaulting, while a malformed reward
// list, an id or count outside int32 included, reads as no reward.
func TestLoadOlympiadConfig(t *testing.T) {
	dir := t.TempDir()
	shipped := olympiad.Config{
		StartHour: 18, StartMinute: 0, CompetitionMillis: 21600000, WeeklyPoints: 3,
		MaxPoints: 10, DividerClassed: 3, DividerNonClassed: 5,
		ClassedReward:    []olympiad.Reward{{ItemID: 6651, Count: 50}},
		NonClassedReward: []olympiad.Reward{{ItemID: 6651, Count: 30}},
	}
	if def := olympiad.DefaultConfig(); !reflect.DeepEqual(def, shipped) {
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
			name: "set",
			props: "OlyStartTime = 20\nOlyMin = 05\nOlyCPeriod = 3600000\nOlyWeeklyPoints = 7\n" +
				"OlyMaxPoints = 4\nOlyDividerClassed = 2\nOlyDividerNonClassed = 6\n" +
				"OlyClassedReward = 57-1000;6651-5\nOlyNonClassedReward = 6651-1\n",
			want: olympiad.Config{
				StartHour: 20, StartMinute: 5, CompetitionMillis: 3600000, WeeklyPoints: 7,
				MaxPoints: 4, DividerClassed: 2, DividerNonClassed: 6,
				ClassedReward:    []olympiad.Reward{{ItemID: 57, Count: 1000}, {ItemID: 6651, Count: 5}},
				NonClassedReward: []olympiad.Reward{{ItemID: 6651, Count: 1}},
			},
		},
		{
			name:  "malformed reward",
			props: "OlyClassedReward = 6651\n",
			want: func() olympiad.Config {
				c := olympiad.DefaultConfig()
				c.ClassedReward = []olympiad.Reward{}
				return c
			}(),
		},
		{
			// Integer.parseInt rejects an id or count outside int32, so the
			// reference gives no reward rather than a wrapped item id.
			name:  "out-of-range reward",
			props: "OlyClassedReward = 4294973947-50\nOlyNonClassedReward = 6651-2147483648\n",
			want: func() olympiad.Config {
				c := olympiad.DefaultConfig()
				c.ClassedReward = []olympiad.Reward{}
				c.NonClassedReward = []olympiad.Reward{}
				return c
			}(),
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
		if err != nil || !reflect.DeepEqual(got, tt.want) {
			t.Errorf("%s: loadOlympiadConfig = %+v, %v; want %+v", tt.name, got, err, tt.want)
		}
	}
}
