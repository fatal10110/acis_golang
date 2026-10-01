package sql

import (
	"context"
	"slices"
	"testing"

	"github.com/fatal10110/acis_golang/internal/gameserver/clan"
	"github.com/fatal10110/acis_golang/internal/gameserver/data/sql/sqltest"
)

// TestClanStoreSubunitsAndWarsRoundTrip writes sub-units, member sub-unit
// and mentor columns, an academy join level, and wars through the store,
// then reloads them as a restart does: a lapsed penalty is deleted first,
// a penalty still ahead and a war come back, a re-declared war loses its
// penalty, and an ended war without penalty is gone.
func TestClanStoreSubunitsAndWarsRoundTrip(t *testing.T) {
	ctx := context.Background()
	db := sqltest.SharedDB(t)
	chars := NewCharacterStore(db)
	store := NewClanStore(db)
	const (
		knights, rivals, others, foes int32 = 0x20000001, 0x20000002, 0x20000003, 0x20000004
		lord, captain, pupil          int32 = 0x20000011, 0x20000012, 0x20000013
		now                           int64 = 1_800_000_000_000
	)
	for _, r := range []clan.Row{{ID: knights, Name: "Knights", LeaderID: lord}, {ID: rivals, Name: "Rivals"}, {ID: others, Name: "Others"}, {ID: foes, Name: "Foes"}} {
		if err := store.InsertClan(ctx, r); err != nil {
			t.Fatalf("InsertClan %s: %v", r.Name, err)
		}
	}
	for _, m := range []struct {
		id   int32
		name string
		row  clan.MembershipRow
	}{
		{lord, "Lord", clan.MembershipRow{ClanID: knights}},
		{captain, "Captain", clan.MembershipRow{ClanID: knights, PowerGrade: 6}},
		{pupil, "Pupil", clan.MembershipRow{ClanID: knights, PowerGrade: 9, PledgeType: clan.SubunitAcademy, LvlJoinedAcademy: 12}},
	} {
		if err := chars.Create(ctx, testCharacter(m.id, m.name)); err != nil {
			t.Fatalf("create %s: %v", m.name, err)
		}
		m.row.ObjectID = m.id
		if err := store.SaveMembership(ctx, m.row); err != nil {
			t.Fatalf("SaveMembership %s: %v", m.name, err)
		}
	}

	steps := []struct {
		name string
		run  func() error
	}{
		{"insert academy", func() error {
			return store.InsertSubunit(ctx, clan.SubunitRow{ClanID: knights, SubPledge: clan.SubPledge{ID: clan.SubunitAcademy, Name: "School"}})
		}},
		{"insert royal", func() error {
			return store.InsertSubunit(ctx, clan.SubunitRow{ClanID: knights, SubPledge: clan.SubPledge{ID: clan.SubunitRoyal1, Name: "Guards"}})
		}},
		{"rename and staff royal", func() error {
			return store.UpdateSubunit(ctx, clan.SubunitRow{ClanID: knights, SubPledge: clan.SubPledge{ID: clan.SubunitRoyal1, Name: "Wardens", LeaderID: captain}})
		}},
		{"move captain", func() error { return store.SetPledgeType(ctx, captain, clan.SubunitRoyal1) }},
		{"link pupil", func() error { return store.SetMentor(ctx, pupil, 0, lord) }},
		{"link lord", func() error { return store.SetMentor(ctx, lord, pupil, 0) }},
		{"war on rivals", func() error { return store.InsertWar(ctx, knights, rivals) }},
		{"war on others", func() error { return store.InsertWar(ctx, knights, others) }},
		{"end war on others", func() error { return store.EndWar(ctx, knights, others, now+1) }},
		{"war on foes", func() error { return store.InsertWar(ctx, knights, foes) }},
		{"end war on foes", func() error { return store.EndWar(ctx, knights, foes, now-1) }},
		{"rivals war back", func() error { return store.InsertWar(ctx, rivals, knights) }},
		{"rivals end at once", func() error { return store.EndWar(ctx, rivals, knights, 0) }},
		{"rivals on others", func() error { return store.InsertWar(ctx, rivals, others) }},
		{"rivals end on others", func() error { return store.EndWar(ctx, rivals, others, now+5) }},
		{"rivals redeclare", func() error { return store.InsertWar(ctx, rivals, others) }},
		{"drop lapsed", func() error { return store.DeleteExpiredWars(ctx, now) }},
	}
	for _, step := range steps {
		if err := step.run(); err != nil {
			t.Fatalf("%s: %v", step.name, err)
		}
	}

	snap, err := store.Load(ctx)
	if err != nil {
		t.Fatalf("Load: %v", err)
	}
	var subunits []clan.SubunitRow
	for _, r := range snap.Subunits {
		if r.ClanID == knights {
			subunits = append(subunits, r)
		}
	}
	slices.SortFunc(subunits, func(a, b clan.SubunitRow) int { return a.ID - b.ID })
	wantSubunits := []clan.SubunitRow{
		{ClanID: knights, SubPledge: clan.SubPledge{ID: clan.SubunitAcademy, Name: "School"}},
		{ClanID: knights, SubPledge: clan.SubPledge{ID: clan.SubunitRoyal1, Name: "Wardens", LeaderID: captain}},
	}
	if !slices.Equal(subunits, wantSubunits) {
		t.Fatalf("sub-units = %+v, want %+v", subunits, wantSubunits)
	}

	members := map[int32]clan.Member{}
	for _, m := range snap.Members {
		if m.ClanID == knights {
			members[m.ObjectID] = m.Member
		}
	}
	if m := members[captain]; m.PledgeType != clan.SubunitRoyal1 {
		t.Fatalf("captain sub-unit = %d, want %d", m.PledgeType, clan.SubunitRoyal1)
	}
	if m := members[pupil]; m.PledgeType != clan.SubunitAcademy || m.LvlJoinedAcademy != 12 || m.Sponsor != lord || m.Apprentice != 0 {
		t.Fatalf("pupil row = %+v, want academy, joined at 12, sponsored by the lord", m)
	}
	if m := members[lord]; m.Apprentice != pupil || m.Sponsor != 0 {
		t.Fatalf("lord row = %+v, want the pupil as apprentice", m)
	}

	var wars []clan.WarRow
	for _, r := range snap.Wars {
		if r.ClanID >= knights && r.ClanID <= foes {
			wars = append(wars, r)
		}
	}
	slices.SortFunc(wars, func(a, b clan.WarRow) int {
		if a.ClanID != b.ClanID {
			return int(a.ClanID - b.ClanID)
		}
		return int(a.TargetID - b.TargetID)
	})
	wantWars := []clan.WarRow{
		{ClanID: knights, TargetID: rivals},
		{ClanID: knights, TargetID: others, Expiry: now + 1},
		{ClanID: rivals, TargetID: others},
	}
	if !slices.Equal(wars, wantWars) {
		t.Fatalf("wars = %+v, want %+v", wars, wantWars)
	}
}
