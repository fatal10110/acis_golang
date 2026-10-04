package main

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/fatal10110/acis_golang/internal/gameserver/fishchamp"
)

// TestLoadFishingChampionshipConfig pins the events.properties
// AllowFishChampionship switch (default on) and the FishChampionship*
// prize keys, with their defaults (adena, 800000 down to 100000); a
// malformed value, or a whole number outside the int range, fails boot.
func TestLoadFishingChampionshipConfig(t *testing.T) {
	def := fishchamp.Config{Enabled: true, RewardItemID: 57, Rewards: [5]int32{800000, 500000, 300000, 200000, 100000}}
	for _, tc := range []struct {
		name    string
		events  string
		want    fishchamp.Config
		wantErr bool
	}{
		{name: "defaults", want: def},
		{
			name:   "set",
			events: "AllowFishChampionship = False\nFishChampionshipRewardItemId = 4037\nFishChampionshipReward1 = 5\nFishChampionshipReward2 = 4\nFishChampionshipReward3 = 3\nFishChampionshipReward4 = 2\nFishChampionshipReward5 = 1\n",
			want:   fishchamp.Config{RewardItemID: 4037, Rewards: [5]int32{5, 4, 3, 2, 1}},
		},
		{name: "malformed item", events: "FishChampionshipRewardItemId = adena\n", wantErr: true},
		{name: "prize past int", events: "FishChampionshipReward3 = 2147483648\n", wantErr: true},
	} {
		t.Run(tc.name, func(t *testing.T) {
			paths := gameServerPaths{EventsConfigPath: filepath.Join(t.TempDir(), "events.properties")}
			if err := os.WriteFile(paths.EventsConfigPath, []byte(tc.events), 0o600); err != nil {
				t.Fatal(err)
			}
			got, err := loadFishingChampionshipConfig(paths)
			if tc.wantErr {
				if err == nil {
					t.Fatalf("loadFishingChampionshipConfig() = %+v, want an error", got)
				}
				return
			}
			if err != nil || got != tc.want {
				t.Fatalf("loadFishingChampionshipConfig() = %+v, %v; want %+v", got, err, tc.want)
			}
		})
	}
}
