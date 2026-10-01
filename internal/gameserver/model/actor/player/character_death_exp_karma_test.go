package player

import (
	"slices"
	"sync"
	"testing"

	"github.com/fatal10110/acis_golang/internal/gameserver/model/actor/event"
	modelskill "github.com/fatal10110/acis_golang/internal/gameserver/model/skill"
	"github.com/fatal10110/acis_golang/internal/gameserver/skill/effect"
)

// ---- from character_death_exp_karma_test.go ----
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
	c.levelTable = deathExpKarmaTable(t, karmaModifier, expLossAtDeath)
	c.allowDelevel = true
	c.rateKarmaExpLost = 2.0
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

	rec := recordEvents(c)

	c.applyDeathExpKarmaLoss(killer)

	var karmaNotified []int
	for _, e := range event.Of[event.KarmaChanged](rec) {
		karmaNotified = append(karmaNotified, e.Karma)
	}

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
	// The loss is a negative experience add (Player.java:2925), so it sends
	// UserInfo (PlayerStatus.java:478-485) and no EXP_DECREASED_BY_S1, which
	// only PlayerStatus.removeExpAndSp sends. The karma change sends its own.
	if lost := event.Count[event.ExpSPLost](rec); lost != 0 {
		t.Fatalf("exp-loss notifications = %d, want 0", lost)
	}
	if updates := event.Count[event.UserInfoChanged](rec); updates != 2 {
		t.Fatalf("UserInfo updates = %d, want 2 (karma, then exp)", updates)
	}
	if broadcasts := event.Count[event.RelationChanged](rec); broadcasts != 1 {
		t.Fatalf("relation broadcasts = %d, want 1", broadcasts)
	}
}

// TestApplyDeathExpKarmaLossDropsLossBelowZero pins the negative add's
// overflow branch (PlayableStatus.java:70-72): a loss larger than the
// character's whole experience is dropped, not floored, and UserInfo still
// goes out (PlayerStatus.java:478-485).
func TestApplyDeathExpKarmaLossDropsLossBelowZero(t *testing.T) {
	table, err := NewLevelTable(map[int]Level{
		1: {RequiredExpToLevelUp: 0, ExpLossAtDeath: 10},
		2: {RequiredExpToLevelUp: 3000},
	})
	if err != nil {
		t.Fatalf("NewLevelTable() error = %v", err)
	}
	c := &Character{ID: 1, CharLevel: 1, Exp: 100}
	c.levelTable = table
	c.allowDelevel = true
	rec := recordEvents(c)

	// lostExp = round(3000*10/100) = 300 > 100.
	c.applyDeathExpKarmaLoss(&Character{ID: 2})

	if c.Exp != 100 || c.CharLevel != 1 {
		t.Fatalf("after death Exp = %d, level %d; want unchanged 100 at level 1", c.Exp, c.CharLevel)
	}
	if c.ExpBeforeDeath != 100 {
		t.Fatalf("ExpBeforeDeath = %d, want 100", c.ExpBeforeDeath)
	}
	if got := progressionTrace(rec); !slices.Equal(got, []string{"userinfo"}) {
		t.Fatalf("progression events = %v, want [userinfo]", got)
	}
}

// TestUpdateKarmaLossFromKillExp matches updateKarmaLoss and setKarma
// (Player.java:2749-2757, 1067-1087) for kill exp: karma drops by
// floor(exp / karmaModifier / 15), announced once with UserInfo and a
// relation broadcast; a loss that floors to zero changes and announces
// nothing, and neither does a karma-free character.
func TestUpdateKarmaLossFromKillExp(t *testing.T) {
	c := newDeathExpKarmaCharacter(t, 2.0, 10.0)
	rec := recordEvents(c)

	// karmaLost = int(29/2.0/15) = 0.
	c.UpdateKarmaLoss(c.levelTable, 29)
	if c.KarmaPoints != 100 || len(rec.Events()) != 0 {
		t.Fatalf("sub-1 loss: karma = %d, events = %v; want 100 and none", c.KarmaPoints, rec.Events())
	}

	// karmaLost = int(600/2.0/15) = 20.
	c.UpdateKarmaLoss(c.levelTable, 600)
	if c.KarmaPoints != 80 {
		t.Fatalf("KarmaPoints = %d, want 80", c.KarmaPoints)
	}
	if got := event.Of[event.KarmaChanged](rec); len(got) != 1 || got[0].Karma != 80 {
		t.Fatalf("karma-change notifications = %v, want [80]", got)
	}
	if n := event.Count[event.UserInfoChanged](rec); n != 1 {
		t.Fatalf("UserInfo updates = %d, want 1", n)
	}
	if n := event.Count[event.RelationChanged](rec); n != 1 {
		t.Fatalf("relation broadcasts = %d, want 1", n)
	}

	c.KarmaPoints = 0
	before := len(rec.Events())
	c.UpdateKarmaLoss(c.levelTable, 600)
	if c.KarmaPoints != 0 || len(rec.Events()) != before {
		t.Fatalf("karma-free: karma = %d, events %d → %d; want 0 and unchanged", c.KarmaPoints, before, len(rec.Events()))
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
	rec := recordEvents(c)

	c.applyDeathExpKarmaLoss(killer)

	// percentLost = 10.0 (unscaled); lostExp = round(2000*10/100) = 200.
	if want := int64(1300); c.Exp != want {
		t.Fatalf("Exp after death = %d, want %d", c.Exp, want)
	}
	if c.KarmaPoints != 0 {
		t.Fatalf("KarmaPoints after karma-free death = %d, want 0", c.KarmaPoints)
	}
	if broadcasts := event.Count[event.RelationChanged](rec); broadcasts != 0 {
		t.Fatalf("relation broadcasts after karma-free death = %d, want 0", broadcasts)
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
// The previous death's exp snapshot is still cleared, since
// `setExpBeforeDeath(0)` (Player.java:2618) runs before that gate, and a held
// Charm of Courage is not used up, since a closed gate never reaches
// applyDeathPenalty's charm check.
func TestApplyDeathExpKarmaLossDelevelDisabledIsNoOp(t *testing.T) {
	c := newDeathExpKarmaCharacter(t, 2.0, 10.0)
	c.allowDelevel = false
	c.ExpBeforeDeath = 1800
	c.SetInPvPZone(true)
	c.SetInSiegeZone(true)
	giveCharmOfCourage(t, c)
	killer := &Character{ID: 2}

	c.applyDeathExpKarmaLoss(killer)

	if c.Exp != 1500 || c.KarmaPoints != 100 {
		t.Fatalf("delevel-disabled death changed state: Exp=%d KarmaPoints=%d, want unchanged (1500, 100)", c.Exp, c.KarmaPoints)
	}
	assertGateClosedDeathSideEffects(t, c)
}

// giveCharmOfCourage lands a Charm of Courage on c itself, so its start and
// exit hooks refresh c's status window.
func giveCharmOfCourage(t *testing.T, c *Character) {
	t.Helper()
	attachTestLive(t, c)
	landSelfEffect(t, c, "CharmOfCourage", 60)
	if !c.CharmOfCourage() {
		t.Fatal("CharmOfCourage() = false after the charm landed")
	}
}

// assertGateClosedDeathSideEffects checks what a killed death does while the
// delevel gate is closed: the stale exp snapshot is cleared and the Charm of
// Courage stays.
func assertGateClosedDeathSideEffects(t *testing.T, c *Character) {
	t.Helper()
	if c.ExpBeforeDeath != 0 {
		t.Fatalf("ExpBeforeDeath after gate-closed killed death = %d, want 0 (cleared before the gate)", c.ExpBeforeDeath)
	}
	if !c.EffectList().IsAffected(effect.FlagCharmOfCourage) || !c.CharmOfCourage() {
		t.Fatalf("Charm of Courage stopped by a gate-closed death, want it kept")
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
	c.ExpBeforeDeath = 1800
	c.SetInPvPZone(true)
	c.SetInSiegeZone(true)
	giveCharmOfCourage(t, c)
	killer := &Character{ID: 2}

	c.applyDeathExpKarmaLoss(killer)

	if c.Exp != 1500 || c.KarmaPoints != 100 {
		t.Fatalf("Lucky-skill sub-10 death changed state: Exp=%d KarmaPoints=%d, want unchanged (1500, 100)", c.Exp, c.KarmaPoints)
	}
	assertGateClosedDeathSideEffects(t, c)
}

// TestApplyDeathExpKarmaLossSiegeCharmSparesNonPlayableKill matches
// applyDeathPenalty's siege branch (Player.java:2881-2889): the charm exempts
// the death whoever the killer, here a monster, and is used up.
func TestApplyDeathExpKarmaLossSiegeCharmSparesNonPlayableKill(t *testing.T) {
	c := newDeathExpKarmaCharacter(t, 2.0, 10.0)
	c.ExpBeforeDeath = 1800
	c.SetInPvPZone(true)
	c.SetInSiegeZone(true)
	giveCharmOfCourage(t, c)
	rec := recordEvents(c)

	c.applyDeathExpKarmaLoss(deathPenaltyKiller{})

	if c.Exp != 1500 || c.KarmaPoints != 100 {
		t.Fatalf("siege charm death to a monster changed state: Exp=%d KarmaPoints=%d, want unchanged (1500, 100)", c.Exp, c.KarmaPoints)
	}
	if c.ExpBeforeDeath != 0 {
		t.Fatalf("ExpBeforeDeath after exempt death = %d, want 0", c.ExpBeforeDeath)
	}
	if c.EffectList().IsAffected(effect.FlagCharmOfCourage) {
		t.Fatalf("Charm of Courage still held after the siege death it spared, want it stopped")
	}
	if c.CharmOfCourage() || event.Count[event.EtcStatusBroadcast](rec) != 1 {
		t.Fatalf("charm end: CharmOfCourage()=%v EtcStatusBroadcast=%d, want false and 1 status refresh",
			c.CharmOfCourage(), event.Count[event.EtcStatusBroadcast](rec))
	}
}

// TestDeathLossExemption pins applyDeathPenalty's PvP-zone branches
// (Player.java:2881-2894): outside a PvP zone nothing is exempt; in a siege
// zone only the charm exempts, whoever the killer, and is used up; in any
// other PvP zone a playable killer exempts without touching the charm.
func TestDeathLossExemption(t *testing.T) {
	tests := []struct {
		name                          string
		inPvP, inSiege, charm, byPlay bool
		wantExempt, wantUseCharm      bool
	}{
		{name: "open world, charm, playable", charm: true, byPlay: true},
		{name: "siege, charm, monster", inPvP: true, inSiege: true, charm: true, wantExempt: true, wantUseCharm: true},
		{name: "siege, charm, playable", inPvP: true, inSiege: true, charm: true, byPlay: true, wantExempt: true, wantUseCharm: true},
		{name: "siege, no charm, playable", inPvP: true, inSiege: true, byPlay: true},
		{name: "siege, no charm, monster", inPvP: true, inSiege: true},
		{name: "arena, playable", inPvP: true, byPlay: true, wantExempt: true},
		{name: "arena, charm, playable", inPvP: true, charm: true, byPlay: true, wantExempt: true},
		{name: "arena, monster", inPvP: true, charm: true},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			exempt, useCharm := deathLossExemption(tt.inPvP, tt.inSiege, tt.charm, tt.byPlay)
			if exempt != tt.wantExempt || useCharm != tt.wantUseCharm {
				t.Fatalf("deathLossExemption(pvp=%v, siege=%v, charm=%v, playable=%v) = (%v, %v), want (%v, %v)",
					tt.inPvP, tt.inSiege, tt.charm, tt.byPlay, exempt, useCharm, tt.wantExempt, tt.wantUseCharm)
			}
		})
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

// TestApplyDeathExpKarmaLossSnapshotsExpBeforeDeath matches
// Player.applyDeathPenalty's `setExpBeforeDeath(getStatus().getExp())`
// (Player.java:2919): the pre-loss exp is recorded, not the post-loss exp.
func TestApplyDeathExpKarmaLossSnapshotsExpBeforeDeath(t *testing.T) {
	c := newDeathExpKarmaCharacter(t, 2.0, 10.0)
	killer := &Character{ID: 2}

	c.applyDeathExpKarmaLoss(killer)

	if c.ExpBeforeDeath != 1500 {
		t.Fatalf("ExpBeforeDeath = %d, want 1500 (the pre-loss exp)", c.ExpBeforeDeath)
	}
	if c.Exp != 1100 {
		t.Fatalf("Exp after death = %d, want 1100", c.Exp)
	}
}

// TestRestoreExpAddsPercentOfLostExpAndClearsSnapshot matches
// Player.restoreExp (Player.java:2865-2872), which adds the experience
// through PlayerStatus.addExp alone.
func TestRestoreExpAddsPercentOfLostExpAndClearsSnapshot(t *testing.T) {
	c := newDeathExpKarmaCharacter(t, 2.0, 10.0)
	killer := &Character{ID: 2}
	c.applyDeathExpKarmaLoss(killer) // Exp: 1500 -> 1100, ExpBeforeDeath = 1500.

	rec := recordEvents(c)

	c.RestoreExp(50)

	// restored = round((1500-1100)*50/100) = 200.
	if want := int64(1300); c.Exp != want {
		t.Fatalf("Exp after RestoreExp(50) = %d, want %d", c.Exp, want)
	}
	if c.ExpBeforeDeath != 0 {
		t.Fatalf("ExpBeforeDeath after RestoreExp = %d, want 0", c.ExpBeforeDeath)
	}
	// A bare experience add (Player.java:2869): UserInfo, and no reward
	// message, which only addExpAndSp sends.
	if got := progressionTrace(rec); !slices.Equal(got, []string{"userinfo"}) {
		t.Fatalf("progression events = %v, want [userinfo]", got)
	}
}

// TestRestoreExpNoDeathIsNoOp matches restoreExp's
// `if (getExpBeforeDeath() > 0)` guard (Player.java:2867): a character that
// never died has nothing to restore.
func TestRestoreExpNoDeathIsNoOp(t *testing.T) {
	c := newDeathExpKarmaCharacter(t, 2.0, 10.0)

	c.RestoreExp(100)

	if c.Exp != 1500 {
		t.Fatalf("Exp after no-op RestoreExp = %d, want unchanged 1500", c.Exp)
	}
}

// TestReviveRestoringExpSkipsLivingPlayer matches Player.reviveRequest's
// isDead gate (Player.java:6047): a player who revived at a restart point
// before a resurrection hit landed keeps the exp loss and its snapshot.
func TestReviveRestoringExpSkipsLivingPlayer(t *testing.T) {
	c := newDeathExpKarmaCharacter(t, 2.0, 10.0)
	c.applyDeathExpKarmaLoss(&Character{ID: 2}) // Exp: 1500 -> 1100, ExpBeforeDeath = 1500.
	c.MarkDead()
	if !c.Revive() {
		t.Fatal("restart-point Revive refused a dead player")
	}

	if c.ReviveRestoringExp(50) {
		t.Fatal("ReviveRestoringExp revived a living player")
	}
	if c.Exp != 1100 || c.ExpBeforeDeath != 1500 {
		t.Fatalf("Exp/ExpBeforeDeath = %d/%d, want unchanged 1100/1500", c.Exp, c.ExpBeforeDeath)
	}
}

// TestReviveRestoringExpRacingRestartGivesOneRevive runs a resurrection hit
// and a restart-point revive against the same death at once. Only one
// revive wins, and the exp comes back exactly when the resurrection wins.
func TestReviveRestoringExpRacingRestartGivesOneRevive(t *testing.T) {
	for range 200 {
		c := newDeathExpKarmaCharacter(t, 2.0, 10.0)
		c.applyDeathExpKarmaLoss(&Character{ID: 2}) // Exp: 1500 -> 1100.
		c.MarkDead()

		var restarted, resurrected bool
		var wg sync.WaitGroup
		start := make(chan struct{})
		wg.Add(2)
		go func() { defer wg.Done(); <-start; restarted = c.Revive() }()
		go func() { defer wg.Done(); <-start; resurrected = c.ReviveRestoringExp(50) }()
		close(start)
		wg.Wait()

		if restarted == resurrected {
			t.Fatalf("restart won = %v, resurrection won = %v, want exactly one", restarted, resurrected)
		}
		wantExp := int64(1100)
		if resurrected {
			wantExp = 1300
		}
		if c.Exp != wantExp {
			t.Fatalf("Exp = %d with resurrection won = %v, want %d", c.Exp, resurrected, wantExp)
		}
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
