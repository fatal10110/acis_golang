package player

import (
	"fmt"
	"math"
	"testing"
	"time"

	"github.com/fatal10110/acis_golang/internal/gameserver/model/actor/creature"
	"github.com/fatal10110/acis_golang/internal/gameserver/model/actor/event"
	"github.com/fatal10110/acis_golang/internal/gameserver/model/location"
	modelskill "github.com/fatal10110/acis_golang/internal/gameserver/model/skill"
	"github.com/fatal10110/acis_golang/internal/gameserver/sim"
	"github.com/fatal10110/acis_golang/internal/gameserver/skill/effect"
	"github.com/fatal10110/acis_golang/internal/gameserver/skill/statbonus"
)

// ---- from character_mount_test.go ----
func TestCharacterMountWyvernTracksControlItemAndNotifies(t *testing.T) {
	c := &Character{}
	if !c.Mount(12621, 77) {
		t.Fatal("Mount() = false, want true")
	}
	if got := c.MountType(); got != 2 {
		t.Fatalf("MountType() = %d, want 2", got)
	}
	if got := c.MountNPCID(); got != 12621 {
		t.Fatalf("MountNPCID() = %d, want 12621", got)
	}
	if got := c.MountObjectID(); got != 77 {
		t.Fatalf("MountObjectID() = %d, want 77", got)
	}
}

// ---- from character_state_test.go ----
func TestCharacterSpawnProtectionMakesItInvulnerable(t *testing.T) {
	c := &Character{}
	if c.Invul() {
		t.Fatal("Invul() = true without protection")
	}
	c.SetSpawnProtection(true)
	if !c.SpawnProtected() || !c.Invul() {
		t.Fatal("spawn protection did not make the character invulnerable")
	}
	c.SetSpawnProtection(false)
	if c.SpawnProtected() || c.Invul() {
		t.Fatal("cleared spawn protection left the character invulnerable")
	}
}

// Reference fake-death timeline (Player.startFakeDeath and
// Player.stopFakeDeath, Player.java:7017-7056): the lie-down task sets
// _isSitting after (int)(3000 / mult) ms and the get-up task clears
// _isStandingNow and _isFakeDeath after (int)(2500 / mult) ms, dead or
// alive. Neither task is ever cancelled, and stopFakeDeath does not touch
// the lie-down's, so a get-up during the lie-down still ends seated and a
// second stopFakeDeath does not move the first get-up's end.

// fakeDeathTimingCharacter is a character whose run speed makes the
// movement speed multiplier its DEX bonus, on a queue whose clock the test
// advances. It returns the clock and the reference lie-down and get-up
// lengths, (int)(3000 / mult) and (int)(2500 / mult) ms.
func fakeDeathTimingCharacter(t *testing.T) (c *Character, clock *sim.Inline, lieDown, getUp time.Duration) {
	t.Helper()
	tmpl := combatTemplate()
	tmpl.RunSpeed, tmpl.WalkSpeed = 120, 80
	c = liveCharacter(1, tmpl, combatItems())
	live, err := creature.NewLive(location.Location{}, 0, permissiveGeo{}, c)
	if err != nil {
		t.Fatal(err)
	}
	clock = sim.NewInline(time.Unix(0, 0))
	live.SetQueue(clock.NewQueue("test"))
	c.Live = live
	mult := float32(statbonus.DEXBonus[tmpl.DEX])
	if mult == 1 {
		t.Fatalf("DEX %d gives multiplier 1; the lengths would not tell the multiplier apart", tmpl.DEX)
	}
	return c, clock, time.Duration(int32(3000/mult)) * time.Millisecond, time.Duration(int32(2500/mult)) * time.Millisecond
}

// TestStopFakeDeathOnCorpseHoldsFakeDeathForGetUp pins the get-up on a dead
// character: the get-up and revive visuals go out once each, it takes the
// standing posture and gets up, and it plays dead until the get-up ends,
// through a revive in between.
func TestStopFakeDeathOnCorpseHoldsFakeDeathForGetUp(t *testing.T) {
	c, clock, lieDown, getUp := fakeDeathTimingCharacter(t)
	c.StartFakeDeath()
	clock.Advance(lieDown)
	rec := recordEvents(c)
	if !c.MarkDead() {
		t.Fatal("MarkDead() = false, want true")
	}

	if !c.StopFakeDeath() {
		t.Fatal("StopFakeDeath() = false for a dead character lying down, want the posture changed")
	}
	if stances, revives := event.Count[event.StanceChanged](rec), event.Count[event.FakeDeathRevived](rec); stances != 1 || revives != 1 {
		t.Fatalf("dead fake-death exit broadcasts = stances:%d revives:%d, want one each", stances, revives)
	}
	if got := event.Of[event.StanceChanged](rec)[0]; got.Stance != event.StanceFakeDeathStop {
		t.Fatalf("stance = %v, want StanceFakeDeathStop", got.Stance)
	}
	if !c.Standing() || !c.StandingNow() || !c.FakeDead() {
		t.Fatalf("dead get-up Standing=%v StandingNow=%v FakeDead=%v, want standing, getting up, playing dead",
			c.Standing(), c.StandingNow(), c.FakeDead())
	}

	if !c.Revive() {
		t.Fatal("Revive() = false, want true")
	}
	clock.Advance(getUp - time.Millisecond)
	if !c.FakeDead() || !c.AlikeDead() {
		t.Fatalf("1ms before the get-up ends FakeDead=%v AlikeDead=%v, want both true", c.FakeDead(), c.AlikeDead())
	}
	clock.Advance(time.Millisecond)
	if c.FakeDead() || c.AlikeDead() || c.StandingNow() || !c.Standing() {
		t.Fatalf("after the get-up FakeDead=%v AlikeDead=%v StandingNow=%v Standing=%v, want up and not playing dead",
			c.FakeDead(), c.AlikeDead(), c.StandingNow(), c.Standing())
	}
	if got := event.Count[event.PostureSettled](rec); got != 1 {
		t.Fatalf("PostureSettled events = %d, want 1 at the get-up's end", got)
	}
}

// TestStopFakeDeathWithoutLiveRuntimeEndsFakeDeath pins a character with no
// live runtime: it has no queue to end a get-up on, so it leaves fake death
// at once.
func TestStopFakeDeathWithoutLiveRuntimeEndsFakeDeath(t *testing.T) {
	c := liveCharacter(1, combatTemplate(), combatItems())
	c.Live = nil
	c.StartFakeDeath()
	if !c.FakeDead() {
		t.Fatal("FakeDead() = false after StartFakeDeath")
	}
	c.StopFakeDeath()
	if c.FakeDead() {
		t.Fatal("FakeDead() = true after StopFakeDeath with no live runtime")
	}
}

// TestRepeatedGetUpKeepsFirstEnd pins a repeated get-up request: it
// re-sends the get-up and revive visuals and restarts the grace, and the
// first get-up still ends fake death on time, dead or alive. The repeat's
// own get-up ends later on its own.
func TestRepeatedGetUpKeepsFirstEnd(t *testing.T) {
	for _, dead := range []bool{false, true} {
		t.Run(fmt.Sprintf("dead=%v", dead), func(t *testing.T) {
			c, clock, lieDown, getUp := fakeDeathTimingCharacter(t)
			c.StartFakeDeath()
			clock.Advance(lieDown)
			c.StopFakeDeath()
			if dead && !c.MarkDead() {
				t.Fatal("MarkDead() = false, want true")
			}
			clock.Advance(getUp / 2)
			c.stateMu.Lock()
			c.recentFakeDeathUntil = time.Time{}
			c.stateMu.Unlock()
			rec := recordEvents(c)

			c.GetUpFromFakeDeath()
			if stances, revives := event.Count[event.StanceChanged](rec), event.Count[event.FakeDeathRevived](rec); stances != 1 || revives != 1 {
				t.Fatalf("repeated get-up broadcasts = stances:%d revives:%d, want one each", stances, revives)
			}
			if !c.RecentFakeDeath() {
				t.Fatal("RecentFakeDeath() = false after a repeated get-up request")
			}
			clock.Advance(getUp - getUp/2 - time.Millisecond)
			if !c.FakeDead() {
				t.Fatal("FakeDead() = false 1ms before the first get-up ends")
			}
			clock.Advance(time.Millisecond)
			if c.FakeDead() || c.StandingNow() {
				t.Fatalf("at the first get-up's end FakeDead=%v StandingNow=%v, want the repeat not to move it", c.FakeDead(), c.StandingNow())
			}
			if got := event.Count[event.PostureSettled](rec); got != 1 {
				t.Fatalf("PostureSettled events at the first get-up's end = %d, want 1", got)
			}
			clock.Advance(getUp / 2)
			if got := event.Count[event.PostureSettled](rec); got != 2 {
				t.Fatalf("PostureSettled events after the repeat's get-up = %d, want 2", got)
			}
		})
	}
}

// TestLaterGetUpEndsFakeDeathBegunMeanwhile pins the repeat's own get-up:
// a Fake Death cast again after the first get-up ended has its fake death
// ended by it, while the new lie-down and the effect go on
// (Player.isFakeDeath is the flag alone, Player.java:2141-2144).
func TestLaterGetUpEndsFakeDeathBegunMeanwhile(t *testing.T) {
	c, clock, lieDown, getUp := fakeDeathTimingCharacter(t)
	c.StartFakeDeath()
	clock.Advance(lieDown)
	c.StopFakeDeath()
	clock.Advance(getUp / 2)
	c.GetUpFromFakeDeath()
	clock.Advance(getUp - getUp/2)
	if c.FakeDead() {
		t.Fatal("FakeDead() = true after the first get-up ended")
	}

	e, err := effect.New(effect.Skill{ID: 60}, modelskill.EffectTemplate{Name: "FakeDeath"})
	if err != nil {
		t.Fatal(err)
	}
	e.Effected = c
	c.EffectList().Add(e)
	clock.Advance(0)
	if !c.FakeDead() || !c.SittingNow() {
		t.Fatalf("Fake Death cast again FakeDead=%v SittingNow=%v, want lying down", c.FakeDead(), c.SittingNow())
	}
	clock.Advance(getUp / 2)
	if !c.EffectList().IsAffected(effect.FlagFakeDeath) {
		t.Fatal("the repeat's get-up ended the Fake Death effect")
	}
	if c.FakeDead() || c.AlikeDead() {
		t.Fatalf("after the repeat's get-up FakeDead=%v AlikeDead=%v with the effect on, want both false", c.FakeDead(), c.AlikeDead())
	}
	if !c.SittingNow() {
		t.Fatal("the repeat's get-up ended the new lie-down")
	}
}

// TestShorterLaterGetUpEndsFakeDeathFirst pins two get-ups of different
// lengths: a later get-up made shorter by a higher movement speed
// multiplier ends fake death before the first one would.
func TestShorterLaterGetUpEndsFakeDeathFirst(t *testing.T) {
	c, clock, lieDown, getUp := fakeDeathTimingCharacter(t)
	c.StartFakeDeath()
	clock.Advance(lieDown)
	c.armorGradePenalty = 2
	c.StopFakeDeath()
	c.armorGradePenalty = 0
	clock.Advance(time.Millisecond)
	c.GetUpFromFakeDeath()

	clock.Advance(getUp - time.Millisecond)
	if !c.FakeDead() {
		t.Fatal("FakeDead() = false 1ms before the shorter get-up ends")
	}
	clock.Advance(time.Millisecond)
	if c.FakeDead() {
		t.Fatal("FakeDead() = true at the shorter get-up's end, want it to end fake death before the longer one")
	}
}

// TestFakeDeadFollowsFakeDeathNotItsEffect pins isFakeDeath() to the
// player's own flag (Player.isFakeDeath, Player.java:2141-2144): the Fake
// Death effect's start sets it, and it holds after the effect ends until
// the get-up does. Being alike dead, the player cannot attack
// (Creature.isAttackingDisabled, Creature.java:628-631).
func TestFakeDeadFollowsFakeDeathNotItsEffect(t *testing.T) {
	c, clock, _, getUp := fakeDeathTimingCharacter(t)
	e, err := effect.New(effect.Skill{ID: 60}, modelskill.EffectTemplate{Name: "FakeDeath"})
	if err != nil {
		t.Fatal(err)
	}
	e.Effected = c
	c.EffectList().Add(e)
	clock.Advance(0)
	if !c.FakeDead() || !c.AlikeDead() || !c.AttackDisabled() {
		t.Fatalf("with Fake Death on FakeDead=%v AlikeDead=%v AttackDisabled=%v, want all true", c.FakeDead(), c.AlikeDead(), c.AttackDisabled())
	}

	c.EffectList().Remove(e)
	clock.Advance(0)
	if !c.FakeDead() || !c.AlikeDead() || !c.AttackDisabled() {
		t.Fatalf("during the get-up FakeDead=%v AlikeDead=%v AttackDisabled=%v, want all true", c.FakeDead(), c.AlikeDead(), c.AttackDisabled())
	}
	clock.Advance(getUp)
	if c.FakeDead() || c.AlikeDead() || c.AttackDisabled() {
		t.Fatalf("after the get-up FakeDead=%v AlikeDead=%v AttackDisabled=%v, want all false", c.FakeDead(), c.AlikeDead(), c.AttackDisabled())
	}
}

// TestStopFakeDeathDuringLieDownEndsSeated pins a get-up during the
// lie-down: the lie-down runs on beside it and seats the character when it
// ends, whichever ends first; the get-up still ends fake death.
func TestStopFakeDeathDuringLieDownEndsSeated(t *testing.T) {
	for _, tc := range []struct {
		name string
		// stopAfter is how far into the lie-down the get-up starts.
		stopAfter func(lieDown, getUp time.Duration) time.Duration
	}{
		{"get-up ends first", func(time.Duration, time.Duration) time.Duration { return 0 }},
		{"lie-down ends first", func(lieDown, _ time.Duration) time.Duration { return lieDown - time.Millisecond }},
	} {
		t.Run(tc.name, func(t *testing.T) {
			c, clock, lieDown, getUp := fakeDeathTimingCharacter(t)
			c.StartFakeDeath()
			stopAt := tc.stopAfter(lieDown, getUp)
			clock.Advance(stopAt)
			rec := recordEvents(c)
			c.StopFakeDeath()
			if !c.Standing() || !c.SittingNow() || !c.StandingNow() {
				t.Fatalf("get-up during the lie-down Standing=%v SittingNow=%v StandingNow=%v, want standing with both under way",
					c.Standing(), c.SittingNow(), c.StandingNow())
			}

			clock.Advance(max(lieDown, stopAt+getUp))
			if c.Standing() || !c.Seated() || c.FakeDead() || c.SittingNow() || c.StandingNow() {
				t.Fatalf("after both ended Standing=%v Seated=%v FakeDead=%v SittingNow=%v StandingNow=%v, want seated, not playing dead",
					c.Standing(), c.Seated(), c.FakeDead(), c.SittingNow(), c.StandingNow())
			}
			if got := event.Count[event.PostureSettled](rec); got != 2 {
				t.Fatalf("PostureSettled events = %d, want 2 (lie-down and get-up)", got)
			}
		})
	}
}

// TestDieDuringFakeDeathGetUpGetsUpAgain pins the death of a player still
// getting up out of fake death (Player.doDie, Player.java:2609-2613): the
// flag still holds, so the death sends one more get-up and revive and
// restarts the recent-fake-death grace (Player.stopFakeDeath,
// Player.java:7035-7056).
func TestDieDuringFakeDeathGetUpGetsUpAgain(t *testing.T) {
	c := attachIdleLive(t, liveCharacter(1, combatTemplate(), combatItems()))
	c.StartFakeDeath()
	c.StopFakeDeath()
	c.stateMu.Lock()
	c.recentFakeDeathUntil = time.Time{}
	c.stateMu.Unlock()
	rec := recordEvents(c)

	if !c.Die(nil) {
		t.Fatal("Die() = false on a living character")
	}
	if !c.RecentFakeDeath() {
		t.Fatal("RecentFakeDeath() = false after dying during the get-up, want the grace restarted")
	}
	stops := 0
	for _, e := range event.Of[event.StanceChanged](rec) {
		if e.Stance == event.StanceFakeDeathStop {
			stops++
		}
	}
	if revives := event.Count[event.FakeDeathRevived](rec); stops != 1 || revives != 1 {
		t.Fatalf("death during the get-up sent stops:%d revives:%d, want one each", stops, revives)
	}
}

func TestCharacterAllSkillsDisabledUnionsCrowdControlStates(t *testing.T) {
	tests := []struct {
		name       string
		effectName string
	}{
		{"Stunned", "Stun"},
		{"Sleeping", "Sleep"},
		{"Afraid", "Fear"},
		{"ImmobileUntilAttacked", "ImmobileUntilAttacked"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			c := &Character{ID: 1}
			attachTestLive(t, c)

			if c.AllSkillsDisabled() {
				t.Fatal("AllSkillsDisabled() = true before any lock is active")
			}

			e := addCharacterEffect(t, c, tt.effectName)
			if !c.AllSkillsDisabled() {
				t.Fatalf("AllSkillsDisabled() = false with %s active, want true", tt.effectName)
			}

			c.EffectList().Remove(e)
			if c.AllSkillsDisabled() {
				t.Fatalf("AllSkillsDisabled() = true after %s was removed", tt.effectName)
			}
		})
	}

	t.Run("Paralyzed", func(t *testing.T) {
		c := &Character{ID: 1}
		attachTestLive(t, c)

		c.SetParalyzed(true)
		if !c.AllSkillsDisabled() {
			t.Fatal("AllSkillsDisabled() = false with Paralyzed lock set, want true")
		}
		c.SetParalyzed(false)
		if c.AllSkillsDisabled() {
			t.Fatal("AllSkillsDisabled() = true after the paralyze lock was cleared")
		}
	})
}

func TestCharacterItemDisabledConsultsAllSkillsDisabledOnlyWhenAnItemIsAlreadyTracked(t *testing.T) {
	c := &Character{ID: 1}
	attachTestLive(t, c)

	e := addCharacterEffect(t, c, "Stun")
	if c.ItemDisabled(1) {
		t.Fatal("ItemDisabled() = true while stunned but no item is tracked as disabled, want false (matches Java's isItemDisabled emptiness short-circuit)")
	}

	c.DisableItem(2, time.Minute)
	if !c.ItemDisabled(1) {
		t.Fatal("ItemDisabled() = false for an untracked id while stunned and another item is disabled, want true")
	}

	c.EffectList().Remove(e)
	if c.ItemDisabled(1) {
		t.Fatal("ItemDisabled() = true for an untracked id once the stun lock clears")
	}
	if !c.ItemDisabled(2) {
		t.Fatal("ItemDisabled() = false for the item still inside its own disable window")
	}
}

func TestCharacterSkillDisabledConsultsAllSkillsDisabledOnlyWhenASkillIsAlreadyTracked(t *testing.T) {
	c := &Character{ID: 1}
	attachTestLive(t, c)

	e := addCharacterEffect(t, c, "Stun")
	if c.SkillDisabled(1) {
		t.Fatal("SkillDisabled() = true while stunned but no skill is on cooldown, want false (matches Java's isSkillDisabled emptiness short-circuit)")
	}

	c.DisableSkill(2, time.Minute)
	if !c.SkillDisabled(1) {
		t.Fatal("SkillDisabled() = false for an untracked key while stunned and another skill is on cooldown, want true")
	}

	c.EffectList().Remove(e)
	if c.SkillDisabled(1) {
		t.Fatal("SkillDisabled() = true for an untracked key once the stun lock clears")
	}
	if !c.SkillDisabled(2) {
		t.Fatal("SkillDisabled() = false for the skill still inside its own reuse window")
	}
}

// TestFakeDeathDelay pins the fake-death transition times,
// (int) (millis / multiplier) ms (Player.java:7029,7052): truncated, and a
// zero multiplier's infinity saturating at Integer.MAX_VALUE.
func TestFakeDeathDelay(t *testing.T) {
	for _, tc := range []struct {
		millis, mult float32
		want         time.Duration
	}{
		{3000, 1, 3000 * time.Millisecond},
		{2500, 1, 2500 * time.Millisecond},
		{3000, 1.25, 2400 * time.Millisecond},
		{2500, 0.84, 2976 * time.Millisecond},
		{3000, 0.84, 3571 * time.Millisecond},
		{2500, 3, 833 * time.Millisecond},
		{3000, 0, math.MaxInt32 * time.Millisecond},
	} {
		if got := fakeDeathDelay(tc.millis, tc.mult); got != tc.want {
			t.Errorf("fakeDeathDelay(%v, %v) = %v, want %v", tc.millis, tc.mult, got, tc.want)
		}
	}
}
