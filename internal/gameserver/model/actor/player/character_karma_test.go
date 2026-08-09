package player

import (
	"testing"

	"github.com/fatal10110/acis_golang/internal/gameserver/task"
)

func TestCalculatePKKillKarmaGainMatchesReferenceFormula(t *testing.T) {
	cases := []struct {
		pkKills int
		want    int
	}{
		{1, 240},     // (((0*0.5)+1)*60)*4
		{2, 360},     // (((1*0.5)+1)*60)*4
		{99, 12000},  // last tier-1 value: (((98*0.5)+1)*60)*4
		{100, 12030}, // first tier-2 value: (((101*0.125)+37.5)*60)*4
		{179, 14400}, // last tier-2 value: converges to the plateau value here
		{180, pkKillKarmaPlateau},
		{500, pkKillKarmaPlateau},
	}
	for _, tc := range cases {
		if got := calculatePKKillKarmaGain(tc.pkKills); got != tc.want {
			t.Errorf("calculatePKKillKarmaGain(%d) = %d, want %d", tc.pkKills, got, tc.want)
		}
	}
}

func TestAwardKillerPKKarmaAwardsInnocentVictimKill(t *testing.T) {
	victim := &Character{ID: 1}
	killer := &Character{ID: 2}
	var notified []int
	killer.SetKarmaChangeNotifier(func(karma int) { notified = append(notified, karma) })
	broadcasts := 0
	killer.SetRelationBroadcaster(func() { broadcasts++ })

	victim.awardKillerPKKarma(killer)

	if killer.PKKills != 1 {
		t.Fatalf("killer.PKKills = %d, want 1", killer.PKKills)
	}
	if killer.KarmaPoints != 240 {
		t.Fatalf("killer.KarmaPoints = %d, want 240", killer.KarmaPoints)
	}
	if want := []int{240}; len(notified) != 1 || notified[0] != want[0] {
		t.Fatalf("karma-change notifications = %v, want %v", notified, want)
	}
	if broadcasts != 1 {
		t.Fatalf("relation broadcasts = %d, want 1", broadcasts)
	}
}

func TestAwardKillerPKKarmaAccumulatesAcrossKills(t *testing.T) {
	killer := &Character{ID: 2}

	(&Character{ID: 1}).awardKillerPKKarma(killer)
	(&Character{ID: 3}).awardKillerPKKarma(killer)

	if killer.PKKills != 2 {
		t.Fatalf("killer.PKKills = %d, want 2", killer.PKKills)
	}
	if want := 240 + 360; killer.KarmaPoints != want {
		t.Fatalf("killer.KarmaPoints = %d, want %d", killer.KarmaPoints, want)
	}
}

func TestAwardKillerPKKarmaSkipsWhenVictimAlreadyHadKarma(t *testing.T) {
	victim := &Character{ID: 1, KarmaPoints: 500}
	killer := &Character{ID: 2}
	notified := false
	killer.SetKarmaChangeNotifier(func(int) { notified = true })
	broadcast := false
	killer.SetRelationBroadcaster(func() { broadcast = true })

	victim.awardKillerPKKarma(killer)

	if killer.PKKills != 0 || killer.KarmaPoints != 0 {
		t.Fatalf("killer = (PKKills=%d, KarmaPoints=%d), want (0, 0)", killer.PKKills, killer.KarmaPoints)
	}
	if notified {
		t.Fatalf("karma-change notifier fired, want no notification when the award is skipped")
	}
	if broadcast {
		t.Fatalf("relation broadcaster fired, want no broadcast when the award is skipped")
	}
}

// npcKiller is a minimal non-player DeathActor double.
type npcKiller struct{ id int32 }

func (k npcKiller) ObjectID() int32 { return k.id }

func TestAwardKillerPKKarmaSkipsNonPlayerKiller(t *testing.T) {
	victim := &Character{ID: 1}

	// Should not panic and should be a no-op for a non-*Character killer.
	victim.awardKillerPKKarma(npcKiller{id: 99})
}

func TestAwardKillerPKKarmaSkipsNilKiller(t *testing.T) {
	victim := &Character{ID: 1}

	victim.awardKillerPKKarma(nil)
}

func TestAwardKillerPKKarmaSkipsWhenVictimIsPvPFlagged(t *testing.T) {
	victim := &Character{ID: 1}
	victim.UpdatePvPFlag(task.PvPFlagOn)
	killer := &Character{ID: 2}
	notified := false
	killer.SetKarmaChangeNotifier(func(int) { notified = true })

	victim.awardKillerPKKarma(killer)

	if killer.PKKills != 0 || killer.KarmaPoints != 0 {
		t.Fatalf("killer = (PKKills=%d, KarmaPoints=%d), want (0, 0): killing an actively-flagged, karma-free victim is a PvP kill, not a PK", killer.PKKills, killer.KarmaPoints)
	}
	if notified {
		t.Fatalf("karma-change notifier fired, want no notification when the award is skipped")
	}
}

func TestAwardKillerPKKarmaSkipsSelfKill(t *testing.T) {
	c := &Character{ID: 1}

	c.awardKillerPKKarma(c)

	if c.PKKills != 0 || c.KarmaPoints != 0 {
		t.Fatalf("self-kill: (PKKills=%d, KarmaPoints=%d), want (0, 0)", c.PKKills, c.KarmaPoints)
	}
}

func TestAwardKillerPvPKillAwardsFlaggedVictimKill(t *testing.T) {
	victim := &Character{ID: 1}
	victim.UpdatePvPFlag(task.PvPFlagOn)
	killer := &Character{ID: 2}
	updates := 0
	killer.updateUserInfo = func() { updates++ }

	victim.awardKillerPvPKill(killer)

	if killer.PvPKills != 1 {
		t.Fatalf("killer.PvPKills = %d, want 1", killer.PvPKills)
	}
	if killer.KarmaPoints != 0 {
		t.Fatalf("killer.KarmaPoints = %d, want 0", killer.KarmaPoints)
	}
	if updates != 1 {
		t.Fatalf("UserInfo updates = %d, want 1", updates)
	}
}

func TestAwardKillerPvPKillSkipsUnflaggedVictim(t *testing.T) {
	victim := &Character{ID: 1}
	killer := &Character{ID: 2}

	victim.awardKillerPvPKill(killer)

	if killer.PvPKills != 0 {
		t.Fatalf("killer.PvPKills = %d, want 0", killer.PvPKills)
	}
}

func TestAwardKillerPvPKillSkipsWhenVictimHasKarma(t *testing.T) {
	victim := &Character{ID: 1, KarmaPoints: 500}
	victim.UpdatePvPFlag(task.PvPFlagOn)
	killer := &Character{ID: 2}

	victim.awardKillerPvPKill(killer)

	if killer.PvPKills != 0 {
		t.Fatalf("killer.PvPKills = %d, want 0", killer.PvPKills)
	}
}

func TestAwardKillerPvPKillAwardsKarmaVictimWhenConfigured(t *testing.T) {
	victim := &Character{ID: 1, KarmaPoints: 500}
	killer := &Character{ID: 2}
	killer.SetAwardPKKillPVPPoint(true)

	victim.awardKillerPvPKill(killer)

	if killer.PvPKills != 1 {
		t.Fatalf("killer.PvPKills = %d, want 1", killer.PvPKills)
	}
}

func TestAwardKillerPvPKillAwardsKarmaVictimForKarmaKillerWhenConfigured(t *testing.T) {
	victim := &Character{ID: 1, KarmaPoints: 500}
	killer := &Character{ID: 2, KarmaPoints: 100}
	killer.SetAwardPKKillPVPPoint(true)

	victim.awardKillerPvPKill(killer)

	if killer.PvPKills != 1 {
		t.Fatalf("killer.PvPKills = %d, want 1", killer.PvPKills)
	}
}

func TestAwardKillerPvPKillSkipsKarmaVictimWhenDisabled(t *testing.T) {
	victim := &Character{ID: 1, KarmaPoints: 500}
	killer := &Character{ID: 2}
	killer.SetAwardPKKillPVPPoint(false)

	victim.awardKillerPvPKill(killer)

	if killer.PvPKills != 0 {
		t.Fatalf("killer.PvPKills = %d, want 0", killer.PvPKills)
	}
}

func TestAwardKillerPvPKillSkipsNegativeKarmaVictim(t *testing.T) {
	victim := &Character{ID: 1, KarmaPoints: -1}
	killer := &Character{ID: 2}
	killer.SetAwardPKKillPVPPoint(true)

	victim.awardKillerPvPKill(killer)

	if killer.PvPKills != 0 {
		t.Fatalf("killer.PvPKills = %d, want 0", killer.PvPKills)
	}
}

func TestAwardKillerPvPKillSkipsWhenKillerHasKarma(t *testing.T) {
	victim := &Character{ID: 1}
	victim.UpdatePvPFlag(task.PvPFlagOn)
	killer := &Character{ID: 2, KarmaPoints: 500}

	victim.awardKillerPvPKill(killer)

	if killer.PvPKills != 0 {
		t.Fatalf("killer.PvPKills = %d, want 0", killer.PvPKills)
	}
}

func TestAwardKillerPvPKillSkipsNonPlayerAndNilAndSelf(t *testing.T) {
	victim := &Character{ID: 1}
	victim.UpdatePvPFlag(task.PvPFlagOn)

	victim.awardKillerPvPKill(npcKiller{id: 99})
	victim.awardKillerPvPKill(nil)

	self := &Character{ID: 2}
	self.UpdatePvPFlag(task.PvPFlagOn)
	self.awardKillerPvPKill(self)
	if self.PvPKills != 0 {
		t.Fatalf("self-kill: PvPKills = %d, want 0", self.PvPKills)
	}
}
