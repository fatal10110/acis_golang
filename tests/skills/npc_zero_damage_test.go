package skills

import (
	"testing"
	"time"

	"github.com/fatal10110/acis_golang/internal/gameserver/model/actor/attackable"
	"github.com/fatal10110/acis_golang/internal/gameserver/model/actor/npc"
	modelskill "github.com/fatal10110/acis_golang/internal/gameserver/model/skill"
	"github.com/fatal10110/acis_golang/internal/gameserver/skill/effect"
	"github.com/fatal10110/acis_golang/internal/gameservertest"
)

// sleepHostile puts the monster to sleep with a long Sleep effect.
func sleepHostile(t *testing.T, hostile *npc.Hostile) {
	t.Helper()
	onNPCQueue(t, hostile, func() {
		e, err := effect.New(effect.Skill{ID: 101, Level: 1, Debuff: true}, modelskill.EffectTemplate{Name: "Sleep", Time: 60})
		if err != nil {
			t.Errorf("effect.New(Sleep): %v", err)
			return
		}
		e.Effector, e.Effected = hostile, hostile
		hostile.EffectList().Add(e)
	})
	if !hostileAsleep(t, hostile) {
		t.Fatal("the monster is not asleep after the Sleep effect was added")
	}
}

func hostileAsleep(t *testing.T, hostile *npc.Hostile) (asleep bool) {
	t.Helper()
	onNPCQueue(t, hostile, func() {
		for _, e := range hostile.EffectList().All() {
			asleep = asleep || e.Type == effect.TypeSleep
		}
	})
	return asleep
}

// playerCombatant returns the in-world player objID as a combat actor.
func playerCombatant(t *testing.T, srv *gameservertest.Server, objID int32) attackable.Combatant {
	t.Helper()
	obj, ok := srv.State.Player(objID)
	if !ok {
		t.Fatalf("world.Player(%d) missing", objID)
	}
	combatant, ok := obj.(attackable.Combatant)
	if !ok {
		t.Fatalf("world.Player(%d) = %T is not a combatant", objID, obj)
	}
	return combatant
}

// threatOf reports the monster's aggro entry for attacker.
func threatOf(t *testing.T, hostile *npc.Hostile, attacker attackable.Combatant) (threat attackable.Threat, ok bool) {
	t.Helper()
	onNPCQueue(t, hostile, func() { threat, ok = hostile.AI().Threats().Get(attacker) })
	return threat, ok
}

// TestDamageDeniedChargeDamAggroesSleepingMonsterWithoutWakingIt casts a
// CHARGEDAM from a damage-denied player at a sleeping monster (issue
// #2837): the hit works out to zero damage, yet the monster still registers
// it, holding an aggro entry for the caster. The caster is not permitted to
// deal damage, so the hit stops before the wake-up side effects: the
// monster sleeps on, at full HP.
func TestDamageDeniedChargeDamAggroesSleepingMonsterWithoutWakingIt(t *testing.T) {
	t.Parallel()
	def := deniedStrike("CHARGEDAM")
	d := bootDeniedCaster(t, def)
	caster := playerCombatant(t, d.srv, d.objID)
	sleepHostile(t, d.hostile)
	drainUntilQuiet(t, d.c)
	if _, ok := threatOf(t, d.hostile, caster); ok {
		t.Fatal("the monster holds an aggro entry for the caster before the strike")
	}

	if got := youDidDamage(t, d.cast(t, def)); got != 0 {
		t.Fatalf("damage-denied CHARGEDAM reported %d damage, want 0", got)
	}
	d.assertMonsterUnharmed(t)
	threat, ok := threatOf(t, d.hostile, caster)
	if !ok {
		t.Fatal("the monster holds no aggro entry for the damage-denied caster after its zero-damage CHARGEDAM")
	}
	if threat.Damage != 0 {
		t.Fatalf("aggro entry damage = %v, want 0", threat.Damage)
	}
	if !hostileAsleep(t, d.hostile) {
		t.Fatal("the damage-denied zero-damage CHARGEDAM woke the monster, want it still asleep")
	}
}

// TestPermittedZeroDamageSkillHitWakesMonster lands a zero-damage skill hit
// from a player allowed to deal damage on a sleeping monster (issue #2837),
// the hit a non-player CHARGEDAM or a zero countered share delivers: the
// monster registers the hit and, since the attacker is permitted, the hit
// ends its sleep, while its HP stays full.
func TestPermittedZeroDamageSkillHitWakesMonster(t *testing.T) {
	t.Parallel()
	srv := gameservertest.Boot(t,
		gameservertest.WithCharacter("Newbie", 5, 0),
		gameservertest.WithWantChars(1),
	)
	startInWorld(t, srv.Client)
	attacker := playerCombatant(t, srv, srv.SoleObjectID(t))
	hostile := srv.SpawnHostileNPC(t)
	drainUntilQuiet(t, srv.Client)
	sleepHostile(t, hostile)
	full := hostile.CurrentHP()

	onNPCQueue(t, hostile, func() {
		hostile.ReduceHP(0, attacker, modelskill.Definition{ID: 1, Level: 1, SkillType: "CHARGEDAM"})
	})

	if hostileAsleep(t, hostile) {
		t.Fatal("the monster still sleeps after a permitted zero-damage skill hit, want it woken")
	}
	if _, ok := threatOf(t, hostile, attacker); !ok {
		t.Fatal("the monster holds no aggro entry for the attacker after its zero-damage skill hit")
	}
	if hp := hostile.CurrentHP(); hp != full {
		t.Fatalf("monster HP after a zero-damage skill hit = %d, want unchanged %d", hp, full)
	}
}

// TestZeroDamageDOTTickRegistersOnMonster lands a damage-over-time debuff
// whose tick deals no damage (the datapack's Item Skill: Bleed carries such
// a DamOverTime) on a sleeping monster (issue #2837): every tick still
// reaches the monster's HP reduction, refreshing the caster's aggro entry,
// but a damage-over-time tick never wakes the monster or costs it HP.
func TestZeroDamageDOTTickRegistersOnMonster(t *testing.T) {
	t.Parallel()
	const bleed = 48
	srv := gameservertest.Boot(t,
		gameservertest.WithCharacter("Newbie", 5, 0),
		gameservertest.WithWantChars(1),
		gameservertest.WithSkills(skillPersistence(t, []modelskill.Definition{{
			ID: bleed, Level: 1, Activation: modelskill.ActivationActive, Target: modelskill.TargetOne,
			CastRange: 900, HitTime: 0, StaticHitTime: true,
			SkillType: "DEBUFF", EffectType: "DEBUFF", Debuff: true,
			BaseLandRate: 100, IgnoreResists: true,
			Effects: []modelskill.EffectTemplate{{Name: "DamOverTime", Value: 0, Count: 5, Time: 1}},
		}})),
	)
	c, objID := srv.Client, srv.SoleObjectID(t)
	seedKnownSkill(t, srv, objID, bleed, 1)
	startInWorld(t, c)
	caster := playerCombatant(t, srv, objID)
	hostile := srv.SpawnHostileNPC(t)
	drainUntilQuiet(t, c)
	full := targetHostile(t, c, hostile.ObjectID())
	drainUntilQuiet(t, c)

	c.Send(encodeRequestMagicSkillUse(bleed, false, false))
	drainUntilQuiet(t, c)
	sleepHostile(t, hostile)
	landed, _ := threatOf(t, hostile, caster)

	srv.Advance(t, 1100*time.Millisecond)
	srv.TickEffects()

	threat, ok := threatOf(t, hostile, caster)
	if !ok {
		t.Fatal("the monster holds no aggro entry for the caster after a zero-damage DOT tick")
	}
	if !threat.Timestamp.After(landed.Timestamp) {
		t.Fatalf("aggro entry timestamp = %v after the tick, want it refreshed past %v", threat.Timestamp, landed.Timestamp)
	}
	if threat.Damage != 0 {
		t.Fatalf("aggro entry damage = %v, want 0", threat.Damage)
	}
	if !hostileAsleep(t, hostile) {
		t.Fatal("a zero-damage DOT tick woke the monster, want it still asleep")
	}
	if hp := hostile.CurrentHP(); hp != full {
		t.Fatalf("monster HP after a zero-damage DOT tick = %d, want unchanged %d", hp, full)
	}
}
