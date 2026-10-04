package clan

import (
	"testing"
	"time"

	"github.com/fatal10110/acis_golang/internal/gameserver/model/actor/player"
	"github.com/rs/zerolog"
)

// Ids of the dissolving-join world: a clan whose member sorts ahead of its
// leader, so Destroy removes the member while the leader, the inviter, is
// still on the roster.
const (
	djClan    = 1
	djMember  = 1001
	djLeader  = 1002
	djRecruit = 2000
)

// dissolvingJoinWorld restores the clan with its member online and returns
// the service, the clan and the lane writer.
func dissolvingJoinWorld(t *testing.T) (*Service, *Clan, *laneWriter) {
	t.Helper()
	snap := Snapshot{
		Clans: []Row{{ID: djClan, Name: "Doomed", Level: 3, LeaderID: djLeader}},
		Members: []MemberRow{
			{ClanID: djClan, Member: Member{ObjectID: djMember, Name: "Member", PowerGrade: MemberPowerGrade}},
			{ClanID: djClan, Member: Member{ObjectID: djLeader, Name: "Leader"}},
		},
	}
	table := NewTable()
	table.Restore(snap, time.Now(), 1)
	writes := &laneWriter{}
	s := NewService(table, newFakeStore(), writes, nil, DefaultConfig(), nil, zerolog.Nop())
	cl, _ := table.Get(djClan)
	if !cl.SetOnline(djMember, Member{Level: 40}) {
		t.Fatal("member not on the restored roster")
	}
	return s, cl, writes
}

// TestJoinDuringDestroyIsRefused answers the leader's invitation while
// Destroy is emptying the roster, the leader still on it: the join is
// refused, the recruit never reaches the roster and no membership row is
// queued for it.
func TestJoinDuringDestroyIsRefused(t *testing.T) {
	s, cl, writes := dissolvingJoinWorld(t)
	recruit := &player.Character{ID: djRecruit, Name: "Recruit", CharLevel: 40}
	now := time.Now()

	var got JoinRefusal = -1
	online := func(objectID int32) *player.Character {
		if objectID == djMember {
			if !cl.IsMember(djLeader) {
				t.Fatal("leader removed before the member; the join would not land mid-destroy")
			}
			got = s.Join(cl, djLeader, recruit, false, SubunitMain, now)
		}
		return nil
	}
	if _, ok := s.Destroy(cl, false, online, now); !ok {
		t.Fatal("Destroy refused a live clan")
	}
	if got != JoinNotAuthorized {
		t.Fatalf("Join during Destroy = %d, want %d (JoinNotAuthorized)", got, JoinNotAuthorized)
	}
	assertRecruitOutside(t, cl, recruit, writes)
}

// TestJoinAfterDestroyIsRefused answers the leader's invitation on the
// clan as it was looked up before Destroy: the leader keeps every
// privilege by id, so only the destroyed flag refuses it.
func TestJoinAfterDestroyIsRefused(t *testing.T) {
	s, cl, writes := dissolvingJoinWorld(t)
	recruit := &player.Character{ID: djRecruit, Name: "Recruit", CharLevel: 40}
	now := time.Now()
	if _, ok := s.Destroy(cl, false, nil, now); !ok {
		t.Fatal("Destroy refused a live clan")
	}

	if got := s.CheckJoin(cl, djLeader, recruit, false, SubunitMain, now); got != JoinNotAuthorized {
		t.Fatalf("CheckJoin on a destroyed clan = %d, want %d (JoinNotAuthorized)", got, JoinNotAuthorized)
	}
	if got := s.Join(cl, djLeader, recruit, false, SubunitMain, now); got != JoinNotAuthorized {
		t.Fatalf("Join on a destroyed clan = %d, want %d (JoinNotAuthorized)", got, JoinNotAuthorized)
	}
	assertRecruitOutside(t, cl, recruit, writes)
}

func assertRecruitOutside(t *testing.T, cl *Clan, recruit *player.Character, writes *laneWriter) {
	t.Helper()
	if cl.IsMember(djRecruit) {
		t.Fatal("recruit joined the destroyed clan's roster")
	}
	if recruit.ClanID() != 0 {
		t.Fatalf("recruit clan id = %d, want 0", recruit.ClanID())
	}
	writes.mu.Lock()
	defer writes.mu.Unlock()
	if n := len(writes.lanes[djRecruit]); n != 0 {
		t.Fatalf("queued %d rows for the recruit, want none", n)
	}
	if len(cl.Members()) != 0 {
		t.Fatalf("destroyed clan roster = %v, want empty", cl.Members())
	}
}
