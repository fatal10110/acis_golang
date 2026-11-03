package main

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/fatal10110/acis_golang/internal/gameserver/script"
)

// TestLoadEventRules pins the event quest settings to events.properties:
// the shipped file and an empty one both give the shipped defaults
// (NeededPartyMembers 4; RiftMinPartySize 2, costs 21 to 36 by 3, jumps
// every 8 minutes plus up to 5 half minutes, Anakazel 15 percent); set keys
// override them, NeededPartyMembers held to 2 to 9 and the rift settings
// taken as written, negatives included; a malformed value fails the load.
func TestLoadEventRules(t *testing.T) {
	dir := t.TempDir()
	shipped := script.EventRules{
		FourGobletsParty: 4,
		Rift: script.RiftRules{
			MinParty: 2, Costs: [6]int{21, 24, 27, 30, 33, 36},
			JumpDelayMin: 8, JumpDelayRnd: 5, AnakazelPortChance: 15,
		},
	}
	if def := script.DefaultEventRules(); def != shipped {
		t.Fatalf("DefaultEventRules() = %+v, want %+v", def, shipped)
	}
	const shippedProps = "NeededPartyMembers = 4\nRiftMinPartySize = 2\nAutoJumpsDelayMin = 8\nAutoJumpsDelayRnd = 5\n" +
		"RecruitCost = 21\nSoldierCost = 24\nOfficerCost = 27\nCaptainCost = 30\nCommanderCost = 33\nHeroCost = 36\nAnakazelPortChance = 15\n"
	withParty := func(n int) script.EventRules {
		r := shipped
		r.FourGobletsParty = n
		return r
	}
	for _, tt := range []struct {
		name, props string
		want        script.EventRules
		wantErr     bool
	}{
		{name: "shipped", props: shippedProps, want: shipped},
		{name: "empty", want: shipped},
		{name: "party low", props: "NeededPartyMembers = 1\n", want: withParty(2)},
		{name: "party min", props: "NeededPartyMembers = 2\n", want: withParty(2)},
		{name: "party max", props: "NeededPartyMembers = 9\n", want: withParty(9)},
		{name: "party high", props: "NeededPartyMembers = 12\n", want: withParty(9)},
		{
			name: "rift set",
			props: "RiftMinPartySize = -1\nAutoJumpsDelayMin = 0\nAutoJumpsDelayRnd = 40\nRecruitCost = 1\nSoldierCost = 2\n" +
				"OfficerCost = 3\nCaptainCost = 4\nCommanderCost = 5\nHeroCost = 600\nAnakazelPortChance = 101\n",
			want: script.EventRules{FourGobletsParty: 4, Rift: script.RiftRules{
				MinParty: -1, Costs: [6]int{1, 2, 3, 4, 5, 600},
				JumpDelayMin: 0, JumpDelayRnd: 40, AnakazelPortChance: 101,
			}},
		},
		{name: "malformed party", props: "NeededPartyMembers = four\n", wantErr: true},
		{name: "malformed cost", props: "HeroCost = 3.5\n", wantErr: true},
	} {
		path := filepath.Join(dir, tt.name+".properties")
		if err := os.WriteFile(path, []byte(tt.props), 0o600); err != nil {
			t.Fatal(err)
		}
		got, err := loadEventRules(gameServerPaths{EventsConfigPath: path})
		if tt.wantErr {
			if err == nil {
				t.Errorf("%s: loadEventRules = %+v, nil; want an error", tt.name, got)
			}
			continue
		}
		if err != nil || got != tt.want {
			t.Errorf("%s: loadEventRules = %+v, %v; want %+v", tt.name, got, err, tt.want)
		}
	}
}
