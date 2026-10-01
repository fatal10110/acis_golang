package player

import (
	"testing"
	"time"

	"github.com/fatal10110/acis_golang/internal/gameserver/model/actor/creature"
	"github.com/fatal10110/acis_golang/internal/gameserver/model/location"
	modelskill "github.com/fatal10110/acis_golang/internal/gameserver/model/skill"
	"github.com/fatal10110/acis_golang/internal/gameserver/sim"
	"github.com/fatal10110/acis_golang/internal/gameserver/skill/effect"
)

func TestCharacterCrowdControlGettersTrackActiveEffectsAndClearOnRemoval(t *testing.T) {
	tests := []struct {
		name       string
		effectName string
		get        func(*Character) bool
	}{
		{"Stunned", "Stun", (*Character).Stunned},
		{"Rooted", "Root", (*Character).Rooted},
		{"Sleeping", "Sleep", (*Character).Sleeping},
		{"Afraid", "Fear", (*Character).Afraid},
		{"ImmobileUntilAttacked", "ImmobileUntilAttacked", (*Character).ImmobileUntilAttacked},
		{"FakeDead", "FakeDeath", (*Character).FakeDead},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			c := &Character{ID: 1}
			attachTestLive(t, c)

			if tt.get(c) {
				t.Fatalf("%s() = true before any effect is active", tt.name)
			}

			e := addCharacterEffect(t, c, tt.effectName)
			if !tt.get(c) {
				t.Fatalf("%s() = false with the effect active", tt.name)
			}

			c.EffectList().Remove(e)
			if tt.get(c) {
				t.Fatalf("%s() = true after the effect was removed", tt.name)
			}
		})
	}
}

func TestCharacterMovementDisabledTracksImmobilityNotFear(t *testing.T) {
	c := &Character{ID: 1}
	if c.MovementDisabled() {
		t.Fatal("MovementDisabled() = true on a fresh standing character")
	}

	attachTestLive(t, c)
	if c.MovementDisabled() {
		t.Fatal("MovementDisabled() = true after Live attach with no crowd-control")
	}

	c.SetStanding(false)
	if !c.MovementDisabled() {
		t.Fatal("MovementDisabled() = false while sitting")
	}
	c.SetStanding(true)

	if !c.SetTeleporting(true) {
		t.Fatal("SetTeleporting(true) reported no change")
	}
	if !c.MovementDisabled() {
		t.Fatal("MovementDisabled() = false while teleporting")
	}
	c.SetTeleporting(false)

	if !c.SetImmobilized(true) {
		t.Fatal("SetImmobilized(true) reported no change")
	}
	if !c.MovementDisabled() {
		t.Fatal("MovementDisabled() = false while immobilized")
	}
	c.SetImmobilized(false)

	root := addCharacterEffect(t, c, "Root")
	if !c.MovementDisabled() {
		t.Fatal("MovementDisabled() = false while rooted")
	}
	c.EffectList().Remove(root)
	if c.MovementDisabled() {
		t.Fatal("MovementDisabled() = true after root was removed")
	}

	addCharacterEffect(t, c, "Fear")
	if c.MovementDisabled() {
		t.Fatal("MovementDisabled() = true while afraid; fear does not disable movement")
	}

	if !c.MarkDead() {
		t.Fatal("MarkDead() = false, want true")
	}
	if !c.MovementDisabled() {
		t.Fatal("MovementDisabled() = false while dead")
	}
}

// TestCharacterAttackDisabledMatchesReferenceTerms pins the attack gate to
// the reference's seven-term union (Creature.isAttackingDisabled:
// flying, stunned, immobile-until-attacked, sleeping, paralyzed, alike-dead,
// afraid). Rooted, teleporting, immobilized and sitting gate movement only
// and must leave attacking enabled.
func TestCharacterAttackDisabledMatchesReferenceTerms(t *testing.T) {
	effects := []struct {
		name     string
		disables bool
	}{
		{"Stun", true},
		{"ImmobileUntilAttacked", true},
		{"Sleep", true},
		{"Paralyze", true},
		{"Fear", true},
		{"FakeDeath", true},
		{"Root", false},
	}
	for _, tt := range effects {
		t.Run(tt.name, func(t *testing.T) {
			c := &Character{ID: 1}
			attachTestLive(t, c)
			if c.AttackDisabled() {
				t.Fatal("AttackDisabled() = true with no effect active")
			}
			e := addCharacterEffect(t, c, tt.name)
			if got := c.AttackDisabled(); got != tt.disables {
				t.Fatalf("AttackDisabled() = %v with %s active, want %v", got, tt.name, tt.disables)
			}
			c.EffectList().Remove(e)
			if c.AttackDisabled() {
				t.Fatalf("AttackDisabled() = true after %s was removed", tt.name)
			}
		})
	}

	t.Run("flying", func(t *testing.T) {
		c := &Character{ID: 1}
		attachTestLive(t, c)
		c.SetFlying(true)
		if !c.AttackDisabled() {
			t.Fatal("AttackDisabled() = false while flying")
		}
	})

	t.Run("dead", func(t *testing.T) {
		c := &Character{ID: 1}
		attachTestLive(t, c)
		c.MarkDead()
		if !c.AttackDisabled() {
			t.Fatal("AttackDisabled() = false while dead")
		}
	})

	t.Run("movement-only states", func(t *testing.T) {
		c := &Character{ID: 1}
		attachTestLive(t, c)
		c.SetTeleporting(true)
		c.SetImmobilized(true)
		c.SetStanding(false)
		if !c.MovementDisabled() {
			t.Fatal("MovementDisabled() = false while teleporting, immobilized and sitting")
		}
		if c.AttackDisabled() {
			t.Fatal("AttackDisabled() = true while only teleporting, immobilized and sitting")
		}
	})
}

// TestCharacterDenyAIActionMatchesReferenceTerms pins the AI-action gate to
// the reference's seven-term union (Creature.denyAiAction: stunned,
// immobile-until-attacked, sleeping, paralyzed, teleporting, dead, afraid).
// Flying and fake death disable attacking but must not deny the AI action;
// rooted only gates movement.
func TestCharacterDenyAIActionMatchesReferenceTerms(t *testing.T) {
	effects := []struct {
		name   string
		denies bool
	}{
		{"Stun", true},
		{"ImmobileUntilAttacked", true},
		{"Sleep", true},
		{"Paralyze", true},
		{"Fear", true},
		{"FakeDeath", false},
		{"Root", false},
	}
	for _, tt := range effects {
		t.Run(tt.name, func(t *testing.T) {
			c := &Character{ID: 1}
			attachTestLive(t, c)
			if c.DenyAIAction() {
				t.Fatal("DenyAIAction() = true with no effect active")
			}
			e := addCharacterEffect(t, c, tt.name)
			if got := c.DenyAIAction(); got != tt.denies {
				t.Fatalf("DenyAIAction() = %v with %s active, want %v", got, tt.name, tt.denies)
			}
			c.EffectList().Remove(e)
			if c.DenyAIAction() {
				t.Fatalf("DenyAIAction() = true after %s was removed", tt.name)
			}
		})
	}

	t.Run("teleporting", func(t *testing.T) {
		c := &Character{ID: 1}
		attachTestLive(t, c)
		c.SetTeleporting(true)
		if !c.DenyAIAction() {
			t.Fatal("DenyAIAction() = false while teleporting")
		}
	})

	t.Run("dead", func(t *testing.T) {
		c := &Character{ID: 1}
		attachTestLive(t, c)
		c.MarkDead()
		if !c.DenyAIAction() {
			t.Fatal("DenyAIAction() = false while dead")
		}
	})

	t.Run("flying", func(t *testing.T) {
		c := &Character{ID: 1}
		attachTestLive(t, c)
		c.SetFlying(true)
		if c.DenyAIAction() {
			t.Fatal("DenyAIAction() = true while only flying")
		}
	})
}

// mountBodyTable is a one-mount NPC template lookup.
type mountBodyTable struct {
	npcID          int32
	radius, height float64
}

func (m mountBodyTable) CollisionBody(npcID int32) (float64, float64, bool) {
	if npcID != m.npcID {
		return 0, 0, false
	}
	return m.radius, m.height, true
}

// TestCharacterCollisionBodyFollowsMount pins Player.getCollisionRadius /
// getCollisionHeight: while mounted the mount NPC template's footprint
// (wyvern 12621: radius 60, height 80 in the shipped npc data) replaces the
// class template's.
func TestCharacterCollisionBodyFollowsMount(t *testing.T) {
	c := liveCharacter(1, combatTemplate(), combatItems())
	c.Configure(Runtime{Mounts: mountBodyTable{npcID: 12621, radius: 60, height: 80}})

	if got, want := c.CollisionRadius(), 9.0; got != want {
		t.Fatalf("unmounted CollisionRadius() = %v, want %v", got, want)
	}
	if got, want := c.CollisionHeight(), 23.0; got != want {
		t.Fatalf("unmounted CollisionHeight() = %v, want %v", got, want)
	}

	if !c.Mount(12621, 77) {
		t.Fatal("Mount() = false, want true")
	}
	if got, want := c.CollisionRadius(), 60.0; got != want {
		t.Fatalf("mounted CollisionRadius() = %v, want %v", got, want)
	}
	if got, want := c.CollisionHeight(), 80.0; got != want {
		t.Fatalf("mounted CollisionHeight() = %v, want %v", got, want)
	}
}

func TestCharacterThrowUpEffectActivatesAndMovesToLanding(t *testing.T) {
	effector := &Character{ID: 1}
	effector.SetLastKnownPosition(location.Location{X: 100, Y: 0, Z: 0}, 0)
	attachThrowUpTestLive(t, effector)

	effected := &Character{ID: 2}
	effected.SetLastKnownPosition(location.Location{}, 0)
	attachThrowUpTestLive(t, effected)

	e, err := effect.New(effect.Skill{ID: 1, FlyRadius: 600}, modelskill.EffectTemplate{Name: "ThrowUp"})
	if err != nil {
		t.Fatal(err)
	}
	e.Effector, e.Effected = effector, effected
	effected.EffectList().Add(e)
	if !effected.Stunned() {
		t.Fatal("ThrowUp was rejected instead of applying its stunned state")
	}

	effected.EffectList().Remove(e)
	if got := effected.CurrentLocation(); got != (location.Location{X: -600, Y: 0, Z: 0}) {
		t.Fatalf("landing = %+v, want {-600 0 0}", got)
	}
}

func attachThrowUpTestLive(t *testing.T, c *Character) {
	t.Helper()
	live, err := creature.NewLive(location.Location{}, 100, permissiveGeo{}, c)
	if err != nil {
		t.Fatal(err)
	}
	live.SetQueue(idleQueue())
	c.Live = live
}

func TestCharacterParalyzedUnionsManualLockAndActiveEffect(t *testing.T) {
	c := &Character{ID: 1}
	attachTestLive(t, c)

	if c.Paralyzed() {
		t.Fatal("Paralyzed() = true on a fresh character")
	}
	if !c.SetParalyzed(true) {
		t.Fatal("SetParalyzed(true) reported no change")
	}
	if !c.Paralyzed() {
		t.Fatal("Paralyzed() = false with only the manual lock set, want true (OR-union)")
	}

	c.SetParalyzed(false)
	if c.Paralyzed() {
		t.Fatal("Paralyzed() = true after the manual lock was cleared and no effect is active")
	}

	e := addCharacterEffect(t, c, "Paralyze")
	if !c.Paralyzed() {
		t.Fatal("Paralyzed() = false with an active paralyze effect and no manual lock")
	}
	c.EffectList().Remove(e)
	if c.Paralyzed() {
		t.Fatal("Paralyzed() = true after the paralyze effect was removed")
	}
}

func TestCharacterAlikeDeadUnionsRealDeathAndFakeDeath(t *testing.T) {
	c := &Character{ID: 1}
	attachTestLive(t, c)
	c.maxHP = 100
	c.curHP = 100

	if c.AlikeDead() {
		t.Fatal("AlikeDead() = true on a fresh character")
	}

	e := addCharacterEffect(t, c, "FakeDeath")
	if !c.AlikeDead() {
		t.Fatal("AlikeDead() = false with an active fake-death effect, want true")
	}

	c.EffectList().Remove(e)
	if c.AlikeDead() {
		t.Fatal("AlikeDead() = true after the fake-death effect was removed")
	}

	c.MarkDead()
	if !c.AlikeDead() {
		t.Fatal("AlikeDead() = false on a really-dead character, want true")
	}
}

func TestCharacterRecentFakeDeathTracksGracePeriodAfterMarking(t *testing.T) {
	c := &Character{ID: 1}
	attachTestLive(t, c)
	clock := sim.NewInline(time.Unix(0, 0))
	c.Live.SetQueue(clock.NewQueue("test"))

	if c.RecentFakeDeath() {
		t.Fatal("RecentFakeDeath() = true before MarkRecentFakeDeath was ever called")
	}

	c.MarkRecentFakeDeath()
	if !c.RecentFakeDeath() {
		t.Fatal("RecentFakeDeath() = false right after MarkRecentFakeDeath, want true")
	}

	// The grace runs on the character's queue clock, not the wall clock.
	clock.Advance(recentFakeDeathGrace - time.Millisecond)
	if !c.RecentFakeDeath() {
		t.Fatal("RecentFakeDeath() = false before the grace period elapsed on the queue clock")
	}
	clock.Advance(time.Millisecond)
	if c.RecentFakeDeath() {
		t.Fatal("RecentFakeDeath() = true after the grace period elapsed on the queue clock")
	}
}

// TestCharacterClearRecentFakeDeathCancelsGrace matches
// Player.clearRecentFakeDeath() zeroing `_recentFakeDeathEndTime`
// (Player.java:2130-2133), called unconditionally from
// PlayerAttack.doAttack (PlayerAttack.java:23) and PlayerCast.doCast
// (PlayerCast.java:184): an attack or completed cast cancels the grace
// immediately instead of letting it run out on its own.
func TestCharacterClearRecentFakeDeathCancelsGrace(t *testing.T) {
	c := &Character{ID: 1}
	attachTestLive(t, c)

	c.MarkRecentFakeDeath()
	if !c.RecentFakeDeath() {
		t.Fatal("RecentFakeDeath() = false right after MarkRecentFakeDeath, want true")
	}

	c.ClearRecentFakeDeath()
	if c.RecentFakeDeath() {
		t.Fatal("RecentFakeDeath() = true after ClearRecentFakeDeath, want false")
	}
}

func TestCharacterEffectListAndCrowdControlGettersAreSafeBeforeLiveIsAttached(t *testing.T) {
	c := &Character{ID: 1}
	if c.EffectList() != nil {
		t.Fatal("EffectList() = non-nil before Live is attached")
	}
	if c.Stunned() || c.Rooted() || c.Sleeping() || c.Afraid() || c.ImmobileUntilAttacked() || c.Paralyzed() || c.Teleporting() {
		t.Fatal("a crowd-control getter reported true before Live is attached")
	}
}

func TestCharacterTeleportingReportsLiveState(t *testing.T) {
	c := &Character{ID: 1}
	attachTestLive(t, c)

	if c.Teleporting() {
		t.Fatal("Teleporting() = true on a fresh character")
	}
	if !c.SetTeleporting(true) {
		t.Fatal("SetTeleporting(true) reported no change")
	}
	if !c.Teleporting() {
		t.Fatal("Teleporting() = false after SetTeleporting(true)")
	}
	if !c.SetTeleporting(false) {
		t.Fatal("SetTeleporting(false) reported no change")
	}
	if c.Teleporting() {
		t.Fatal("Teleporting() = true after SetTeleporting(false)")
	}
}
