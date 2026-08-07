package player

import "testing"

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

	victim.awardKillerPKKarma(killer)

	if killer.PKKills != 0 || killer.KarmaPoints != 0 {
		t.Fatalf("killer = (PKKills=%d, KarmaPoints=%d), want (0, 0)", killer.PKKills, killer.KarmaPoints)
	}
	if notified {
		t.Fatalf("karma-change notifier fired, want no notification when the award is skipped")
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

func TestAwardKillerPKKarmaSkipsSelfKill(t *testing.T) {
	c := &Character{ID: 1}

	c.awardKillerPKKarma(c)

	if c.PKKills != 0 || c.KarmaPoints != 0 {
		t.Fatalf("self-kill: (PKKills=%d, KarmaPoints=%d), want (0, 0)", c.PKKills, c.KarmaPoints)
	}
}
