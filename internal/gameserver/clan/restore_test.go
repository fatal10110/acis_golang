package clan

import (
	"strconv"
	"testing"
	"time"

	"github.com/rs/zerolog"
)

// TestRestorePrunesPenaltiesAndReputation restores clans whose stored
// penalties straddle now and whose stored reputation sits on either side
// of level 5. An alliance penalty survives only while it lies ahead; a
// recruiting penalty survives while joinDays past its stored time is still
// ahead; a clan below level 5 comes back without reputation.
func TestRestorePrunesPenaltiesAndReputation(t *testing.T) {
	now := time.UnixMilli(1_800_000_000_000)
	nowMs := now.UnixMilli()
	const joinDays = 2
	window := int64(joinDays) * dayMillis
	rows := []Row{
		{ID: 1, Name: "AllyAhead", AllyPenaltyExpiry: nowMs + 1, AllyPenaltyType: 3},
		{ID: 2, Name: "AllyNow", AllyPenaltyExpiry: nowMs, AllyPenaltyType: 3},
		{ID: 3, Name: "CharKept", CharPenaltyExpiry: nowMs - window + 1},
		{ID: 4, Name: "CharDropped", CharPenaltyExpiry: nowMs - window},
		{ID: 5, Name: "LowLevel", Level: 4, Reputation: 500},
		{ID: 6, Name: "Reputed", Level: 5, Reputation: 500},
		{ID: 7, Name: "Capped", Level: 8, Reputation: maxReputation + 1},
		{ID: 8, Name: "Indebted", Level: 5, Reputation: minReputation - 1},
	}
	table := NewTable()
	table.Restore(Snapshot{Clans: rows}, now, joinDays)

	get := func(id int32) *Clan {
		t.Helper()
		cl, ok := table.Get(id)
		if !ok {
			t.Fatalf("clan %d not restored", id)
		}
		return cl
	}
	if cl := get(1); cl.allyPenaltyExpiry != nowMs+1 || cl.allyPenaltyType != 3 {
		t.Errorf("future alliance penalty = %d/%d, want kept", cl.allyPenaltyExpiry, cl.allyPenaltyType)
	}
	if cl := get(2); cl.allyPenaltyExpiry != 0 || cl.allyPenaltyType != 0 {
		t.Errorf("alliance penalty ending now = %d/%d, want dropped", cl.allyPenaltyExpiry, cl.allyPenaltyType)
	}
	if cl := get(3); cl.charPenaltyExpiry != nowMs-window+1 {
		t.Errorf("recruiting penalty inside the join window = %d, want kept", cl.charPenaltyExpiry)
	}
	if cl := get(4); cl.charPenaltyExpiry != 0 {
		t.Errorf("recruiting penalty at the window's end = %d, want dropped", cl.charPenaltyExpiry)
	}
	for id, want := range map[int32]int{5: 0, 6: 500, 7: maxReputation, 8: minReputation} {
		if got := get(id).Info().Reputation; got != want {
			t.Errorf("clan %d reputation = %d, want %d", id, got, want)
		}
	}
}

// TestRestoreRanksTheLadder ranks the clans with positive reputation in
// stored-score order among the 99 best stored scores. A clan below level 5
// takes a ladder slot with its stored score but no rank; the clan with the
// hundredth best score is not ranked.
func TestRestoreRanksTheLadder(t *testing.T) {
	var rows []Row
	rows = append(rows, Row{ID: 1000, Name: "LowButStored", Level: 4, Reputation: 1_000_000})
	for i := range 99 {
		rows = append(rows, Row{ID: int32(1 + i), Name: "Clan" + strconv.Itoa(i), Level: 5, Reputation: 10_000 - i})
	}
	rows = append(rows, Row{ID: 2000, Name: "Broke", Level: 5, Reputation: 0})
	table := NewTable()
	table.Restore(Snapshot{Clans: rows}, time.Now(), 1)

	rank := func(id int32) int {
		cl, _ := table.Get(id)
		return cl.Info().Rank
	}
	if got := rank(1000); got != 0 {
		t.Errorf("level 4 clan rank = %d, want 0", got)
	}
	for i := range 98 {
		if got := rank(int32(1 + i)); got != i+1 {
			t.Fatalf("clan with score %d rank = %d, want %d", 10_000-i, got, i+1)
		}
	}
	if got := rank(99); got != 0 {
		t.Errorf("clan with the hundredth best score rank = %d, want 0", got)
	}
	if got := rank(2000); got != 0 {
		t.Errorf("clan without reputation rank = %d, want 0", got)
	}
}

// TestRestoreRoster puts every stored member offline on its clan's roster,
// drops members of a clan that is gone, and restores the rank privileges.
func TestRestoreRoster(t *testing.T) {
	table := NewTable()
	table.Restore(Snapshot{
		Clans: []Row{{ID: 1, Name: "Roster", LeaderID: 10}},
		Members: []MemberRow{
			{ClanID: 1, Member: Member{ObjectID: 10, Name: "Lead", Online: true}},
			{ClanID: 1, Member: Member{ObjectID: 11, Name: "Mate", PowerGrade: 6}},
			{ClanID: 9, Member: Member{ObjectID: 12, Name: "Stray"}},
		},
		Privileges: []PrivilegeRow{{ClanID: 1, Rank: 6, Privs: int32(PrivInvite)}, {ClanID: 9, Rank: 6, Privs: 1}},
	}, time.Now(), 1)
	cl, _ := table.Get(1)
	if m, ok := cl.Member(10); !ok || m.Online {
		t.Errorf("leader row = %+v, %v; want restored offline", m, ok)
	}
	if !cl.IsMember(11) || cl.MembersCount() != 2 {
		t.Errorf("roster = %+v, want the leader and its mate", cl.Members())
	}
	if _, ok := table.MemberClan(12); ok {
		t.Error("a member of a missing clan was placed on a roster")
	}
	if !cl.HasPrivilege(11, PrivInvite) || cl.HasPrivilege(11, PrivDismiss) {
		t.Errorf("rank 6 privileges = %d, want invite only", cl.MemberPrivileges(11))
	}
}

// TestRestoreMembership settles a character's clan state at selection: a
// clan id naming no clan, or a clan that no longer lists it, is cleared;
// penalties already over are cleared and running ones kept; a member's
// pledge class follows its clan.
func TestRestoreMembership(t *testing.T) {
	now := time.UnixMilli(1_800_000_000_000)
	table := NewTable()
	table.Restore(Snapshot{
		Clans:   []Row{{ID: 1, Name: "Home", Level: 5, LeaderID: 10}},
		Members: []MemberRow{{ClanID: 1, Member: Member{ObjectID: 10, Name: "Lead"}}, {ClanID: 1, Member: Member{ObjectID: 11, Name: "Mate"}}},
	}, now, 1)
	s := NewService(table, nil, nil, nil, DefaultConfig(), nil, zerolog.Nop())

	cases := []struct {
		name      string
		id        int32
		clanID    int32
		hero      bool
		wantClan  int32
		wantClass int
	}{
		{"missing clan", 20, 7, false, 0, 0},
		{"clan no longer lists it", 21, 1, false, 0, 0},
		{"leader", 10, 1, false, 1, 4},
		{"member", 11, 1, false, 1, 2},
		{"hero member", 11, 1, true, 1, 8},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			c := inClan(tc.id, tc.name, tc.clanID)
			c.SetHero(tc.hero)
			c.SetClanJoinExpiryTime(now.UnixMilli() - 1)
			c.SetClanCreateExpiryTime(now.UnixMilli() + 1)
			s.RestoreMembership(c, now)
			if c.ClanID() != tc.wantClan || c.PledgeClass() != tc.wantClass {
				t.Fatalf("clan %d class %d, want clan %d class %d", c.ClanID(), c.PledgeClass(), tc.wantClan, tc.wantClass)
			}
			if c.ClanJoinExpiryTime() != 0 || c.ClanCreateExpiryTime() != now.UnixMilli()+1 {
				t.Fatalf("penalties = join %d create %d, want the ended one cleared and the running one kept", c.ClanJoinExpiryTime(), c.ClanCreateExpiryTime())
			}
		})
	}
}

// TestReputationCrossing moves a clan's reputation across 0 both ways, and
// leaves a clan below level 5 untouched.
func TestReputationCrossing(t *testing.T) {
	cases := []struct {
		level, before, delta int
		changed              bool
		after, crossed       int
	}{
		{4, 0, 100, false, 0, 0},
		{5, 100, -150, true, -50, -1},
		{5, 10, -10, true, 0, -1},
		{5, 0, 10, true, 10, 1},
		{5, -10, 5, true, -5, 0},
		{5, -10, 10, true, 0, 0},
		{5, 10, 5, true, 15, 0},
		{5, maxReputation, 1, true, maxReputation, 0},
	}
	s := NewService(nil, nil, nil, nil, DefaultConfig(), nil, zerolog.Nop())
	for _, tc := range cases {
		cl := &Clan{level: tc.level, reputation: tc.before}
		change, changed := s.addReputationLocked(cl, tc.delta)
		if changed != tc.changed || cl.reputation != tc.after || change.Crossed != tc.crossed || (changed && change.Score != tc.after) {
			t.Errorf("level %d %d%+d = %+v changed %v (now %d), want score %d crossed %d changed %v",
				tc.level, tc.before, tc.delta, change, changed, cl.reputation, tc.after, tc.crossed, tc.changed)
		}
	}
}
