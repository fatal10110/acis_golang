package player

import (
	"testing"

	modelskill "github.com/fatal10110/acis_golang/internal/gameserver/model/skill"
)

// deathExpKarmaTable builds a two-entry level table: level 10 carries the
// penalty parameters under test, and level 11 is the sentinel entry that
// gives level 10's experience band a defined upper bound.
func deathExpKarmaTable(t *testing.T, karmaModifier, expLossAtDeath float64) *LevelTable {
	t.Helper()
	table, err := NewLevelTable(map[int]Level{
		10: {RequiredExpToLevelUp: 1000, KarmaModifier: karmaModifier, ExpLossAtDeath: expLossAtDeath},
		11: {RequiredExpToLevelUp: 3000},
	})
	if err != nil {
		t.Fatalf("NewLevelTable() error = %v", err)
	}
	return table
}

func newDeathExpKarmaCharacter(t *testing.T, karmaModifier, expLossAtDeath float64) *Character {
	c := &Character{ID: 1, CharLevel: 10, Exp: 1500, KarmaPoints: 100}
	c.SetLevelTable(deathExpKarmaTable(t, karmaModifier, expLossAtDeath))
	c.SetAllowDelevel(true)
	c.SetRateKarmaExpLost(2.0)
	return c
}

// TestApplyDeathExpKarmaLossKarmaPositive matches Player.applyDeathPenalty
// (Player.java:2896-2925) and updateKarmaLoss/calculateKarmaLost
// (Player.java:2749-2757, Formulas.java:1267-1270) for a karma-positive
// death: percentLost is scaled by RateKarmaExpLost, the resulting exp loss
// removes the reference's rounded amount, and karma drops by
// floor(lostExp / karmaModifier / 15).
func TestApplyDeathExpKarmaLossKarmaPositive(t *testing.T) {
	c := newDeathExpKarmaCharacter(t, 2.0, 10.0)
	killer := &Character{ID: 2}

	var karmaNotified []int
	c.SetKarmaChangeNotifier(func(karma int) { karmaNotified = append(karmaNotified, karma) })
	var lossNotified [][2]int64
	c.SetExpSpLossNotifier(func(exp int64, sp int) { lossNotified = append(lossNotified, [2]int64{exp, int64(sp)}) })

	c.applyDeathExpKarmaLoss(killer)

	// span = 3000-1000 = 2000; percentLost = 10.0*2.0 = 20.0; lostExp =
	// round(2000*20/100) = 400.
	if want := int64(1100); c.Exp != want {
		t.Fatalf("Exp after death = %d, want %d", c.Exp, want)
	}
	// karmaLost = int(400/2.0/15) = 13.
	if want := 87; c.KarmaPoints != want {
		t.Fatalf("KarmaPoints after death = %d, want %d", c.KarmaPoints, want)
	}
	if len(karmaNotified) != 1 || karmaNotified[0] != 87 {
		t.Fatalf("karma-change notifications = %v, want [87]", karmaNotified)
	}
	if len(lossNotified) != 1 || lossNotified[0] != [2]int64{400, 0} {
		t.Fatalf("exp-loss notifications = %v, want [[400 0]]", lossNotified)
	}
}

// TestApplyDeathExpKarmaLossKarmaZeroSkipsRateAndKarmaLoss matches the
// reference's `if (getKarma() > 0) percentLost *= Config.RATE_KARMA_EXP_LOST`
// (Player.java:2903-2904) and updateKarmaLoss's `getKarma() > 0` guard
// (Player.java:2751): a karma-free death still loses experience at the
// unscaled percentage, and karma stays untouched.
func TestApplyDeathExpKarmaLossKarmaZeroSkipsRateAndKarmaLoss(t *testing.T) {
	c := newDeathExpKarmaCharacter(t, 2.0, 10.0)
	c.KarmaPoints = 0
	killer := &Character{ID: 2}

	c.applyDeathExpKarmaLoss(killer)

	// percentLost = 10.0 (unscaled); lostExp = round(2000*10/100) = 200.
	if want := int64(1300); c.Exp != want {
		t.Fatalf("Exp after death = %d, want %d", c.Exp, want)
	}
	if c.KarmaPoints != 0 {
		t.Fatalf("KarmaPoints after karma-free death = %d, want 0", c.KarmaPoints)
	}
}

// TestApplyDeathExpKarmaLossKarmaFloorsAtZero matches setKarma's
// `Math.max(0, karma)` clamp (Player.java:1073).
func TestApplyDeathExpKarmaLossKarmaFloorsAtZero(t *testing.T) {
	c := newDeathExpKarmaCharacter(t, 2.0, 10.0)
	c.KarmaPoints = 5
	killer := &Character{ID: 2}

	c.applyDeathExpKarmaLoss(killer)

	if c.KarmaPoints != 0 {
		t.Fatalf("KarmaPoints after over-large loss = %d, want 0 (floored)", c.KarmaPoints)
	}
}

// TestApplyDeathExpKarmaLossNoKillerIsNoOp matches the reference's
// `if (killer != null)` guard around the whole penalty (Player.java:2615):
// an environmental death costs nothing.
func TestApplyDeathExpKarmaLossNoKillerIsNoOp(t *testing.T) {
	c := newDeathExpKarmaCharacter(t, 2.0, 10.0)

	c.applyDeathExpKarmaLoss(nil)

	if c.Exp != 1500 || c.KarmaPoints != 100 {
		t.Fatalf("nil-killer death changed state: Exp=%d KarmaPoints=%d, want unchanged (1500, 100)", c.Exp, c.KarmaPoints)
	}
}

// TestApplyDeathExpKarmaLossDelevelDisabledIsNoOp matches the caller-side
// `Config.ALLOW_DELEVEL && ...` gate (Player.java:2650): with the config
// off, no death ever applies the penalty regardless of killer or karma.
func TestApplyDeathExpKarmaLossDelevelDisabledIsNoOp(t *testing.T) {
	c := newDeathExpKarmaCharacter(t, 2.0, 10.0)
	c.SetAllowDelevel(false)
	killer := &Character{ID: 2}

	c.applyDeathExpKarmaLoss(killer)

	if c.Exp != 1500 || c.KarmaPoints != 100 {
		t.Fatalf("delevel-disabled death changed state: Exp=%d KarmaPoints=%d, want unchanged (1500, 100)", c.Exp, c.KarmaPoints)
	}
}

// TestApplyDeathExpKarmaLossLuckySkillBelowTenIsNoOp and
// TestApplyDeathExpKarmaLossLuckySkillAtTenApplies match
// `!hasSkill(SKILL_LUCKY) || getStatus().getLevel() > 9` (Player.java:2650):
// the Lucky skill exempts a death below level 10, but not from level 10 on.
func TestApplyDeathExpKarmaLossLuckySkillBelowTenIsNoOp(t *testing.T) {
	c := newDeathExpKarmaCharacter(t, 2.0, 10.0)
	c.CharLevel = 9
	c.SetSkillLevel(int(modelskill.LuckySkillID), 1)
	killer := &Character{ID: 2}

	c.applyDeathExpKarmaLoss(killer)

	if c.Exp != 1500 || c.KarmaPoints != 100 {
		t.Fatalf("Lucky-skill sub-10 death changed state: Exp=%d KarmaPoints=%d, want unchanged (1500, 100)", c.Exp, c.KarmaPoints)
	}
}

func TestApplyDeathExpKarmaLossLuckySkillAtTenApplies(t *testing.T) {
	c := newDeathExpKarmaCharacter(t, 2.0, 10.0)
	c.SetSkillLevel(int(modelskill.LuckySkillID), 1)
	killer := &Character{ID: 2}

	c.applyDeathExpKarmaLoss(killer)

	if c.Exp == 1500 {
		t.Fatalf("Lucky-skill level-10 death left Exp unchanged, want the penalty applied")
	}
}

// TestApplyDeathExpKarmaLossSiegeZoneHalvesPercent matches the reference's
// `if (... || isInsideZone(ZoneId.SIEGE)) percentLost /= 4.0`
// (Player.java:2906-2907).
func TestApplyDeathExpKarmaLossSiegeZoneHalvesPercent(t *testing.T) {
	c := newDeathExpKarmaCharacter(t, 2.0, 10.0)
	c.KarmaPoints = 0 // isolate the siege-zone quarter from the karma-rate multiplier.
	c.SetInSiegeZone(true)
	killer := &Character{ID: 2}

	c.applyDeathExpKarmaLoss(killer)

	// percentLost = 10.0/4.0 = 2.5; lostExp = round(2000*2.5/100) = 50.
	if want := int64(1450); c.Exp != want {
		t.Fatalf("Exp after siege-zone death = %d, want %d", c.Exp, want)
	}
}

// TestDieAwardsKillerKarmaBeforeApplyingVictimsOwnDeathPenalty matches the
// reference's ordering: Playable.doDie (Playable.java:178-183) runs
// onKillUpdatePvPKarma — the source of awardKillerPKKarma/awardKillerPvPKill
// — while the victim's karma is still untouched, and only Player.doDie's
// later applyDeathPenalty call (Player.java:2650) reduces it
// (updateKarmaLoss, Player.java:2921-2925). Character.Die must therefore run
// the killer-reward checks before applyDeathExpKarmaLoss, not after: a
// small-positive-karma victim (3) must still block the "innocent, karma-free
// victim" PK-karma award even though this same death floors that karma to 0
// a moment later.
func TestDieAwardsKillerKarmaBeforeApplyingVictimsOwnDeathPenalty(t *testing.T) {
	victim := newDeathExpKarmaCharacter(t, 2.0, 10.0)
	victim.KarmaPoints = 3
	killer := &Character{ID: 2}

	if !victim.Die(killer) {
		t.Fatal("Die() = false, want true")
	}

	if killer.PKKills != 0 || killer.KarmaPoints != 0 {
		t.Fatalf("killer = (PKKills=%d, KarmaPoints=%d), want (0, 0): PK-karma award must read the victim's pre-penalty karma", killer.PKKills, killer.KarmaPoints)
	}
	if victim.KarmaPoints != 0 {
		t.Fatalf("victim.KarmaPoints after death = %d, want 0 (own death penalty still floors it)", victim.KarmaPoints)
	}
}
