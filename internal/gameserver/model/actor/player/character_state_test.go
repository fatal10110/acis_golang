package player

import (
	"testing"
	"time"
)

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

func TestCharacterStopFakeDeathDoesNotBroadcastAfterDeath(t *testing.T) {
	c := &Character{ID: 1}
	c.SetStanding(false)
	stances, revives := 0, 0
	c.SetStanceBroadcaster(func(Stance) { stances++ })
	c.SetFakeDeathReviveBroadcaster(func() { revives++ })
	if !c.MarkDead() {
		t.Fatal("MarkDead() = false, want true")
	}

	if c.StopFakeDeath() {
		t.Fatal("StopFakeDeath() = true for a dead character, want false")
	}
	if stances != 0 || revives != 0 {
		t.Fatalf("dead fake-death exit broadcasts = stances:%d revives:%d, want none", stances, revives)
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
