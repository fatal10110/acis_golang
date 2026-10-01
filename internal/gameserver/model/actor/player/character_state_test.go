package player

import (
	"math"
	"testing"
	"time"

	"github.com/fatal10110/acis_golang/internal/gameserver/model/actor/event"
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

// TestCharacterStopFakeDeathBroadcastsAfterDeath pins the get-up on a dead
// character: the get-up and revive visuals still go out, once each, and it
// takes the standing posture with no stand-up transition.
func TestCharacterStopFakeDeathBroadcastsAfterDeath(t *testing.T) {
	c := attachIdleLive(t, liveCharacter(1, combatTemplate(), combatItems()))
	c.StartFakeDeath()
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
	if !c.Standing() || c.StandingNow() || c.FakeDead() {
		t.Fatalf("dead get-up Standing=%v StandingNow=%v FakeDead=%v, want standing at once, not faking",
			c.Standing(), c.StandingNow(), c.FakeDead())
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

// TestStopFakeDeathWithoutGetUpEndsFakeDeath pins the two StopFakeDeath
// branches that schedule no get-up and so must end fake death themselves: a
// character killed while playing dead, which must not stay fake-dead after
// a revive, and a character with no live runtime.
func TestStopFakeDeathWithoutGetUpEndsFakeDeath(t *testing.T) {
	t.Run("dead", func(t *testing.T) {
		c := attachIdleLive(t, liveCharacter(1, combatTemplate(), combatItems()))
		c.StartFakeDeath()
		if !c.FakeDead() {
			t.Fatal("FakeDead() = false after StartFakeDeath")
		}
		if !c.MarkDead() {
			t.Fatal("MarkDead() = false, want true")
		}
		c.StopFakeDeath()
		if c.FakeDead() {
			t.Fatal("FakeDead() = true after a dead character left fake death")
		}
		if !c.Revive() {
			t.Fatal("Revive() = false, want true")
		}
		if c.FakeDead() || c.AlikeDead() {
			t.Fatalf("revived character FakeDead=%v AlikeDead=%v, want both false", c.FakeDead(), c.AlikeDead())
		}
	})
	t.Run("no live runtime", func(t *testing.T) {
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
	})
}

// TestRepeatFakeDeathStopOnlyDuringGetUp pins a repeated get-up request
// (Player.stopFakeDeath during its own get-up): it re-sends the get-up and
// revive visuals and restarts the grace, and leaves the running get-up to
// end fake death. Outside the get-up, or once dead, it does nothing.
func TestRepeatFakeDeathStopOnlyDuringGetUp(t *testing.T) {
	c := attachIdleLive(t, liveCharacter(1, combatTemplate(), combatItems()))
	rec := recordEvents(c)
	c.StartFakeDeath()
	if c.RepeatFakeDeathStop() {
		t.Fatal("RepeatFakeDeathStop() = true while lying down")
	}
	c.StopFakeDeath()
	c.recentFakeDeathUntil = time.Time{}
	stances, revives := event.Count[event.StanceChanged](rec), event.Count[event.FakeDeathRevived](rec)
	c.stateMu.RLock()
	gen := c.postureGen
	c.stateMu.RUnlock()

	if !c.RepeatFakeDeathStop() {
		t.Fatal("RepeatFakeDeathStop() = false during the get-up")
	}
	if got, want := event.Count[event.StanceChanged](rec), stances+1; got != want {
		t.Fatalf("stance broadcasts = %d, want %d", got, want)
	}
	if got, want := event.Count[event.FakeDeathRevived](rec), revives+1; got != want {
		t.Fatalf("revive broadcasts = %d, want %d", got, want)
	}
	if !c.RecentFakeDeath() {
		t.Fatal("RecentFakeDeath() = false after a repeated get-up request")
	}
	c.stateMu.RLock()
	sameGetUp := c.postureGen == gen && c.standingNow
	c.stateMu.RUnlock()
	if !sameGetUp || !c.FakeDead() {
		t.Fatalf("repeated get-up request replaced the get-up (same=%v FakeDead=%v)", sameGetUp, c.FakeDead())
	}

	c.settlePosture(gen, true)
	if c.FakeDead() {
		t.Fatal("FakeDead() = true after the original get-up ended")
	}
	if c.RepeatFakeDeathStop() {
		t.Fatal("RepeatFakeDeathStop() = true after the get-up ended")
	}

	d := attachIdleLive(t, liveCharacter(2, combatTemplate(), combatItems()))
	d.StartFakeDeath()
	d.StopFakeDeath()
	d.MarkDead()
	if d.RepeatFakeDeathStop() {
		t.Fatal("RepeatFakeDeathStop() = true on a dead character")
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
