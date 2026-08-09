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

// TestCharacterReduceDeathPenaltyLevelFiresReducedUpdater matches the
// reference's reduceDeathPenaltyBuffLevel (Player.java:6544-6553): the
// packet-layer notification fires with the resulting level on every actual
// decrement (so the caller can tell the S1_ADDED reapply case, level > 0,
// from the LIFTED case, level == 0), and does not fire on the no-op at
// zero (Player.java:6537-6538).
func TestCharacterReduceDeathPenaltyLevelFiresReducedUpdater(t *testing.T) {
	c := &Character{ID: 1}
	c.SetDeathPenaltyLevel(1)

	var got []int
	c.SetDeathPenaltyReducedUpdater(func(level int) { got = append(got, level) })

	c.ReduceDeathPenaltyLevel() // 1 -> 0: reapply-message branch never fires, LIFTED does.
	c.ReduceDeathPenaltyLevel() // already 0: no-op, updater must not fire again.

	if want := []int{0}; len(got) != len(want) || got[0] != want[0] {
		t.Fatalf("reduced-updater calls = %v, want %v", got, want)
	}
}

func TestCharacterDeathPenaltySkillUpdaterReplacesLevelOnEachChange(t *testing.T) {
	c := &Character{ID: 1}
	c.SetDeathPenaltyChance(100)

	var got [][2]int
	c.SetDeathPenaltySkillUpdater(func(oldLevel, newLevel int) {
		got = append(got, [2]int{oldLevel, newLevel})
	})

	c.RaiseDeathPenaltyLevel(nil, 1)
	c.RaiseDeathPenaltyLevel(nil, 1)
	c.ReduceDeathPenaltyLevel()
	c.ReduceDeathPenaltyLevel()
	c.ReduceDeathPenaltyLevel()

	want := [][2]int{{0, 1}, {1, 2}, {2, 1}, {1, 0}}
	if len(got) != len(want) {
		t.Fatalf("death-penalty skill updates = %v, want %v", got, want)
	}
	for i := range want {
		if got[i] != want[i] {
			t.Fatalf("death-penalty skill update %d = %v, want %v", i, got[i], want[i])
		}
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
	c.SetDeathPenaltyChance(20)

	// roll above the configured chance, no karma: blocked.
	if got, changed := c.RaiseDeathPenaltyLevel(deathPenaltyKiller{}, 21); changed || got != 0 {
		t.Fatalf("RaiseDeathPenaltyLevel(highRoll) = (%d, %v), want (0, false)", got, changed)
	}
	// roll at or below the configured chance, no karma: passes.
	if got, changed := c.RaiseDeathPenaltyLevel(deathPenaltyKiller{}, 20); !changed || got != 1 {
		t.Fatalf("RaiseDeathPenaltyLevel(lowRoll) = (%d, %v), want (1, true)", got, changed)
	}
}

func TestRaiseDeathPenaltyLevelUsesConfiguredChance(t *testing.T) {
	c := &Character{ID: 1}
	c.SetDeathPenaltyChance(0)

	if got, changed := c.RaiseDeathPenaltyLevel(deathPenaltyKiller{}, 1); changed || got != 0 {
		t.Fatalf("RaiseDeathPenaltyLevel(configured chance) = (%d, %v), want (0, false)", got, changed)
	}
}

func TestRaiseDeathPenaltyLevelKarmaBypassesChanceRoll(t *testing.T) {
	c := &Character{ID: 1}
	c.KarmaPoints = 1

	if got, changed := c.RaiseDeathPenaltyLevel(deathPenaltyKiller{}, 100); !changed || got != 1 {
		t.Fatalf("RaiseDeathPenaltyLevel(karma, highRoll) = (%d, %v), want (1, true)", got, changed)
	}
}

func TestRaiseDeathPenaltyLevelExemptsPvPAndSiegeZones(t *testing.T) {
	tests := []struct {
		name string
		set  func(*Character)
	}{
		{name: "pvp", set: func(c *Character) { c.SetInPvPZone(true) }},
		{name: "siege", set: func(c *Character) { c.SetInSiegeZone(true) }},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			c := &Character{ID: 1, KarmaPoints: 1}
			tt.set(c)

			if got, changed := c.RaiseDeathPenaltyLevel(deathPenaltyKiller{}, 100); changed || got != 0 {
				t.Fatalf("RaiseDeathPenaltyLevel() = (%d, %v), want (0, false)", got, changed)
			}
		})
	}
}

// TestRaiseDeathPenaltyLevelFiresRaisedUpdaterOnlyOnPass matches the
// reference's calculateDeathPenaltyBuffLevel (Player.java:6518-6528): the
// packet-layer notification fires with the new level only when the gate
// passes, never on a blocked attempt.
func TestRaiseDeathPenaltyLevelFiresRaisedUpdaterOnlyOnPass(t *testing.T) {
	c := &Character{ID: 1}
	c.KarmaPoints = 1

	var got []int
	c.SetDeathPenaltyRaisedUpdater(func(level int) { got = append(got, level) })

	c.RaiseDeathPenaltyLevel(&Character{ID: 2}, 100) // blocked: player killer, must not fire.
	c.RaiseDeathPenaltyLevel(deathPenaltyKiller{}, 100)

	if want := []int{1}; len(got) != len(want) || got[0] != want[0] {
		t.Fatalf("raised-updater calls = %v, want %v", got, want)
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
