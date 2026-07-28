package player

import "testing"

func TestCharacterSetDeathPenaltyLevelClampsToBounds(t *testing.T) {
	c := &Character{ID: 1}

	c.SetDeathPenaltyLevel(-3)
	if got := c.DeathPenaltyLevel(); got != 0 {
		t.Fatalf("SetDeathPenaltyLevel(-3): DeathPenaltyLevel() = %d, want 0", got)
	}

	c.SetDeathPenaltyLevel(999)
	if got := c.DeathPenaltyLevel(); got != maxDeathPenaltyLevel {
		t.Fatalf("SetDeathPenaltyLevel(999): DeathPenaltyLevel() = %d, want %d", got, maxDeathPenaltyLevel)
	}

	c.SetDeathPenaltyLevel(7)
	if got := c.DeathPenaltyLevel(); got != 7 {
		t.Fatalf("SetDeathPenaltyLevel(7): DeathPenaltyLevel() = %d, want 7", got)
	}
}

func TestCharacterReduceDeathPenaltyLevelDecrementsToFloorZero(t *testing.T) {
	c := &Character{ID: 1}
	c.SetDeathPenaltyLevel(2)

	if got := c.ReduceDeathPenaltyLevel(); got != 1 {
		t.Fatalf("first ReduceDeathPenaltyLevel() = %d, want 1", got)
	}
	if got := c.ReduceDeathPenaltyLevel(); got != 0 {
		t.Fatalf("second ReduceDeathPenaltyLevel() = %d, want 0", got)
	}
	if got := c.ReduceDeathPenaltyLevel(); got != 0 {
		t.Fatalf("ReduceDeathPenaltyLevel() at zero = %d, want unchanged 0", got)
	}
}

// deathPenaltyKiller is a minimal killer double: a non-Player actor whose
// raid-relation is fixed at construction.
type deathPenaltyKiller struct {
	raidRelated bool
}

func (k deathPenaltyKiller) RaidRelated() bool { return k.raidRelated }

func TestRaiseDeathPenaltyLevelCappedAtMax(t *testing.T) {
	c := &Character{ID: 1}
	c.SetDeathPenaltyLevel(maxDeathPenaltyLevel)

	if got, changed := c.RaiseDeathPenaltyLevel(deathPenaltyKiller{}, 1); changed || got != maxDeathPenaltyLevel {
		t.Fatalf("RaiseDeathPenaltyLevel() at cap = (%d, %v), want (%d, false)", got, changed, maxDeathPenaltyLevel)
	}
}

func TestRaiseDeathPenaltyLevelRejectsPlayerKiller(t *testing.T) {
	c := &Character{ID: 1}
	killer := &Character{ID: 2}
	c.KarmaPoints = 1000 // karma alone would otherwise pass the gate

	if got, changed := c.RaiseDeathPenaltyLevel(killer, 1); changed || got != 0 {
		t.Fatalf("RaiseDeathPenaltyLevel(playerKiller) = (%d, %v), want (0, false)", got, changed)
	}
}

func TestRaiseDeathPenaltyLevelNoKarmaFailsChanceRoll(t *testing.T) {
	c := &Character{ID: 1}

	// roll above deathPenaltyChance, no karma: blocked.
	if got, changed := c.RaiseDeathPenaltyLevel(deathPenaltyKiller{}, deathPenaltyChance+1); changed || got != 0 {
		t.Fatalf("RaiseDeathPenaltyLevel(highRoll) = (%d, %v), want (0, false)", got, changed)
	}
	// roll at or below deathPenaltyChance, no karma: passes.
	if got, changed := c.RaiseDeathPenaltyLevel(deathPenaltyKiller{}, deathPenaltyChance); !changed || got != 1 {
		t.Fatalf("RaiseDeathPenaltyLevel(lowRoll) = (%d, %v), want (1, true)", got, changed)
	}
}

func TestRaiseDeathPenaltyLevelKarmaBypassesChanceRoll(t *testing.T) {
	c := &Character{ID: 1}
	c.KarmaPoints = 1

	if got, changed := c.RaiseDeathPenaltyLevel(deathPenaltyKiller{}, 100); !changed || got != 1 {
		t.Fatalf("RaiseDeathPenaltyLevel(karma, highRoll) = (%d, %v), want (1, true)", got, changed)
	}
}

func TestRaiseDeathPenaltyLevelCharmOfLuckBlocksUnidentifiedOrRaidKiller(t *testing.T) {
	c := &Character{ID: 1}
	attachTestLive(t, c)
	c.KarmaPoints = 1
	addCharacterEffect(t, c, "CharmOfLuck")

	// nil killer: blocked.
	if got, changed := c.RaiseDeathPenaltyLevel(nil, 100); changed || got != 0 {
		t.Fatalf("RaiseDeathPenaltyLevel(charmOfLuck, nilKiller) = (%d, %v), want (0, false)", got, changed)
	}
	// raid-related killer: blocked.
	if got, changed := c.RaiseDeathPenaltyLevel(deathPenaltyKiller{raidRelated: true}, 100); changed || got != 0 {
		t.Fatalf("RaiseDeathPenaltyLevel(charmOfLuck, raidKiller) = (%d, %v), want (0, false)", got, changed)
	}
	// non-raid, identified killer: passes.
	if got, changed := c.RaiseDeathPenaltyLevel(deathPenaltyKiller{raidRelated: false}, 100); !changed || got != 1 {
		t.Fatalf("RaiseDeathPenaltyLevel(charmOfLuck, mundaneKiller) = (%d, %v), want (1, true)", got, changed)
	}
}

func TestRaiseDeathPenaltyLevelPhoenixBlessingBlocksAlways(t *testing.T) {
	c := &Character{ID: 1}
	attachTestLive(t, c)
	c.KarmaPoints = 1
	addCharacterEffect(t, c, "PhoenixBless")

	if got, changed := c.RaiseDeathPenaltyLevel(deathPenaltyKiller{raidRelated: false}, 100); changed || got != 0 {
		t.Fatalf("RaiseDeathPenaltyLevel(phoenixBlessed) = (%d, %v), want (0, false)", got, changed)
	}
}
