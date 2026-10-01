package clan

import (
	"slices"
	"strconv"
	"testing"
	"time"

	"github.com/rs/zerolog"
)

// Clan ids of the refusal world. Home is a level 3 clan of two members,
// enough for war with MembersForWar at 2; its second member holds no
// privilege.
const (
	refHome         = 1
	refWeak         = 2
	refWeakAttacker = 3
	refAllied       = 4
	refDissolving   = 5
	refTarget       = 6
	refFoe          = 7
	refBusy         = 8
	refBusyTargets  = 100

	refHomeLeader = 1000
	refHomeMember = 1001
	refBusyLeader = 8000
)

// refusalWorld restores the clans the refusal cases declare on: a weak
// clan, a weak clan that already declared war on Home, a clan of Home's
// alliance, a dissolving clan, a fit target, a foe Home is at war with,
// and a clan already at war with 30 others.
func refusalWorld(t *testing.T) (*Service, *laneWriter) {
	t.Helper()
	snap := Snapshot{}
	addClan := func(id int32, name string, level, members int, ally int32, dissolving int64) {
		leader := id * 1000
		snap.Clans = append(snap.Clans, Row{ID: id, Name: name, Level: level, LeaderID: leader, AllyID: ally, DissolvingExpiry: dissolving})
		for i := range members {
			snap.Members = append(snap.Members, MemberRow{ClanID: id, Member: Member{ObjectID: leader + int32(i), Name: name + strconv.Itoa(i), PowerGrade: MemberPowerGrade}})
		}
	}
	addClan(refHome, "Home", 3, 2, 77, 0)
	addClan(refWeak, "Weak", 1, 1, 0, 0)
	addClan(refWeakAttacker, "WeakAttacker", 1, 1, 0, 0)
	addClan(refAllied, "Allied", 3, 2, 77, 0)
	addClan(refDissolving, "Dissolving", 3, 2, 0, time.Now().Add(time.Hour).UnixMilli())
	addClan(refTarget, "Target", 3, 2, 0, 0)
	addClan(refFoe, "Foe", 3, 2, 0, 0)
	addClan(refBusy, "Busy", 3, 2, 0, 0)
	snap.Wars = append(snap.Wars, WarRow{ClanID: refWeakAttacker, TargetID: refHome}, WarRow{ClanID: refHome, TargetID: refFoe})
	for i := range maxWars {
		id := int32(refBusyTargets + i)
		addClan(id, "Busy"+strconv.Itoa(i), 3, 2, 0, 0)
		snap.Wars = append(snap.Wars, WarRow{ClanID: refBusy, TargetID: id})
	}
	table := NewTable()
	table.Restore(snap, time.Now(), 1)
	cfg := DefaultConfig()
	cfg.MembersForWar = 2
	writes := &laneWriter{}
	return NewService(table, newFakeStore(), writes, nil, cfg, nil, zerolog.Nop()), writes
}

// TestDeclareWarRefusals walks each DeclareWar refusal over the refusal
// world: a refused declaration leaves both clans' war lists as they were
// and queues no row. A weak clan is still a lawful target once it has
// declared war on the clan.
func TestDeclareWarRefusals(t *testing.T) {
	cases := []struct {
		name   string
		actor  int32
		clanID int32
		target string
		want   WarResult
	}{
		{"weak target already attacking the clan", refHomeLeader, refHome, "WeakAttacker", WarDone},
		{"weak target not attacking the clan", refHomeLeader, refHome, "Weak", WarTargetTooWeak},
		{"target of the same alliance", refHomeLeader, refHome, "Allied", WarAllied},
		{"dissolving target", refHomeLeader, refHome, "Dissolving", WarTargetDissolving},
		{"clan already at 30 wars", refBusyLeader, refBusy, "Target", WarTooMany},
		{"member without the clan war privilege", refHomeMember, refHome, "Target", WarNotAuthorized},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			s, writes := refusalWorld(t)
			cl, _ := s.table.Get(tc.clanID)
			target, _ := s.table.ByName(tc.target)
			before, targetBefore := cl.WarList(), target.AttackerList()

			_, got := s.DeclareWar(inClan(tc.actor, "Actor", tc.clanID), tc.target, time.Now())
			if got != tc.want {
				t.Fatalf("DeclareWar = %d, want %d", got, tc.want)
			}
			if tc.want == WarDone {
				if !cl.AtWarWith(target.id) || !slices.Contains(target.AttackerList(), cl.id) {
					t.Fatal("allowed war not filed on both sides")
				}
				if len(writes.lanes[cl.id]) != 1 {
					t.Fatalf("queued %d rows on the declaring clan's lane, want 1", len(writes.lanes[cl.id]))
				}
				return
			}
			if !slices.Equal(cl.WarList(), before) || !slices.Equal(target.AttackerList(), targetBefore) {
				t.Fatalf("refused war changed the lists: %v -> %v, attackers %v -> %v", before, cl.WarList(), targetBefore, target.AttackerList())
			}
			if len(writes.lanes) != 0 {
				t.Fatalf("refused war queued rows: %v", writes.lanes)
			}
		})
	}
}

// TestWarRequestsNeedTheClanWarPrivilege has a member without the clan
// war privilege stop and surrender Home's war on Foe: both are refused and
// the war goes on, with no row queued.
func TestWarRequestsNeedTheClanWarPrivilege(t *testing.T) {
	s, writes := refusalWorld(t)
	home, _ := s.table.Get(refHome)
	member := inClan(refHomeMember, "Member", refHome)

	if _, got := s.StopWar(member, "Foe", func(int32) bool { return false }, time.Now()); got != WarNotAuthorized {
		t.Fatalf("StopWar = %d, want %d", got, WarNotAuthorized)
	}
	if _, got := s.CheckSurrender(member, "Foe"); got != WarNotAuthorized {
		t.Fatalf("CheckSurrender = %d, want %d", got, WarNotAuthorized)
	}
	if !home.AtWarWith(refFoe) {
		t.Fatal("refused stop or surrender ended the war")
	}
	if len(writes.lanes) != 0 {
		t.Fatalf("refused requests queued rows: %v", writes.lanes)
	}
	// The leader holds the privilege: the same surrender passes.
	if _, got := s.CheckSurrender(inClan(refHomeLeader, "Leader", refHome), "Foe"); got != WarDone {
		t.Fatalf("leader's CheckSurrender = %d, want %d", got, WarDone)
	}
}
