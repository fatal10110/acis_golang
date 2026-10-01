package clan

import (
	"slices"
	"testing"
	"time"
)

// TestRestoreWarsAndSubunits restores stored wars, penalties and
// sub-units: a row without expiry is a war, seen from both sides; a row
// with an expiry still ahead is a penalty only; a lapsed penalty and a row
// naming a missing clan are dropped; a sub-unit's captain leads it.
func TestRestoreWarsAndSubunits(t *testing.T) {
	now := time.UnixMilli(1_800_000_000_000)
	nowMs := now.UnixMilli()
	table := NewTable()
	table.Restore(Snapshot{
		Clans: []Row{{ID: 1, Name: "Knights", LeaderID: 10}, {ID: 2, Name: "Rivals"}, {ID: 3, Name: "Others"}},
		Members: []MemberRow{
			{ClanID: 1, Member: Member{ObjectID: 10, Name: "Lord"}},
			{ClanID: 1, Member: Member{ObjectID: 11, Name: "Captain"}},
		},
		Subunits: []SubunitRow{
			{ClanID: 1, SubPledge: SubPledge{ID: SubunitRoyal1, Name: "Guards", LeaderID: 11}},
			{ClanID: 1, SubPledge: SubPledge{ID: SubunitAcademy, Name: "School"}},
			{ClanID: 99, SubPledge: SubPledge{ID: SubunitAcademy, Name: "Orphan"}},
		},
		Wars: []WarRow{
			{ClanID: 1, TargetID: 2},
			{ClanID: 1, TargetID: 3, Expiry: nowMs + 1},
			{ClanID: 2, TargetID: 3, Expiry: nowMs},
			{ClanID: 1, TargetID: 99},
			{ClanID: 99, TargetID: 1},
		},
	}, now, 1)

	knights, _ := table.Get(1)
	rivals, _ := table.Get(2)
	others, _ := table.Get(3)
	if _, attacked := rivals.attackers[1]; !knights.AtWarWith(2) || !attacked || rivals.AtWarWith(1) {
		t.Fatal("stored war 1 -> 2 not restored from both sides")
	}
	if got := knights.WarList(); !slices.Equal(got, []int32{2}) {
		t.Fatalf("Knights' war list = %v, want [2]", got)
	}
	if _, attacked := others.attackers[1]; knights.AtWarWith(3) || attacked {
		t.Fatal("penalty row restored as a war")
	}
	if knights.warPenalties[3] != nowMs+1 {
		t.Fatalf("penalty on 3 = %d, want %d", knights.warPenalties[3], nowMs+1)
	}
	if _, ok := rivals.warPenalties[3]; ok {
		t.Fatal("lapsed penalty restored")
	}
	if !knights.Info().AtWar || rivals.Info().AtWar {
		t.Fatal("AtWar header flag follows the declared wars only")
	}
	if got := knights.SubunitLeaderName(SubunitRoyal1); got != "Captain" {
		t.Fatalf("royal guard captain = %q, want Captain", got)
	}
	if got := knights.SubunitLeaderName(SubunitAcademy); got != "" {
		t.Fatalf("academy leader = %q, want none", got)
	}
	if got := knights.SubunitLeaderName(SubunitMain); got != "Lord" {
		t.Fatalf("main clan leader = %q, want Lord", got)
	}
	if got := knights.leadsSubunit(11); got != SubunitRoyal1 {
		t.Fatalf("captain leads %d, want %d", got, SubunitRoyal1)
	}
}
