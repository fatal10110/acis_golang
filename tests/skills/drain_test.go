package skills

import (
	"testing"
	"time"

	"github.com/fatal10110/acis_golang/internal/gameserver/model/location"
	modelskill "github.com/fatal10110/acis_golang/internal/gameserver/model/skill"
	"github.com/fatal10110/acis_golang/internal/gameserver/network/serverpackets"
	"github.com/fatal10110/acis_golang/internal/gameserver/skill/effect"
	"github.com/fatal10110/acis_golang/internal/gameserver/skill/stat"
	"github.com/fatal10110/acis_golang/internal/gameservertest"
)

// drainCasterHP returns the live caster's exact HP, fractions included.
func drainCasterHP(t *testing.T, srv *gameservertest.Server, objID int32) float64 {
	t.Helper()
	obj, ok := srv.State.Player(objID)
	if !ok {
		t.Fatalf("world.Player(%d) missing", objID)
	}
	hp, ok := obj.(interface{ HP() float64 })
	if !ok {
		t.Fatalf("world.Player(%d) = %T has no HP", objID, obj)
	}
	return hp.HP()
}

// lifeDrain is the shipped Life Drain (1090-1, absorbPart 0.8) with a short
// fixed cast time, so the hit lands well inside the driven clock's window.
func lifeDrain(t *testing.T) modelskill.Definition {
	t.Helper()
	def := shippedSkill(t, 1090, 1)
	if def.SkillType != "DRAIN" || def.AbsorbPart != 0.8 {
		t.Fatalf("shipped 1090-1 = %s absorbPart %v, want DRAIN 0.8", def.SkillType, def.AbsorbPart)
	}
	def.HitTime, def.StaticHitTime = 500, true
	return def
}

// TestLifeDrainFeedsTheCasterFromTheDrainedHP casts Life Drain at the
// fixture monster with every roll forced to a plain hit and the monster's
// M.Def pinned so it survives: the caster regains 80% of the damage in
// single precision, and hears its own full status (CUR_HP, CUR_MP, CUR_CP,
// MAX_CP) before the damage it dealt, then no second HP update.
func TestLifeDrainFeedsTheCasterFromTheDrainedHP(t *testing.T) {
	t.Parallel()
	def := lifeDrain(t)
	srv := gameservertest.Boot(t,
		gameservertest.WithCharacter("Mage", 5, 0),
		gameservertest.WithWantChars(1),
		gameservertest.WithSkills(skillPersistence(t, []modelskill.Definition{def})),
	)
	c, objID := srv.Client, srv.SoleObjectID(t)
	seedKnownSkill(t, srv, objID, 1090, 1)
	startInWorld(t, c)
	setCasterMagicRolls(t, srv, objID, func() int { return 9999 })
	srv.DamagePlayerHP(t, objID, srv.PlayerMaxHP(t, objID)/2)
	hostile := srv.SpawnHostileNPC(t)
	hostile.AddStatFuncs([]effect.Mod{{Stat: stat.MagicDefence, Op: effect.OpSet, Value: 80}})
	drainUntilQuiet(t, c)
	targetHostile(t, c, hostile.ObjectID())
	drainUntilQuiet(t, c)

	before := drainCasterHP(t, srv, objID)
	monsterBefore := hostile.HP()
	c.Send(encodeRequestMagicSkillUse(1090, false, false))
	srv.AdvanceUntil(t, "Life Drain hit", func() bool { return hostile.HP() < monsterBefore })

	damage := monsterBefore - hostile.HP()
	want := before + float64(float32(0.8)*float32(damage))
	if hostile.Dead() || want >= float64(srv.PlayerMaxHP(t, objID)) {
		t.Fatalf("drained %v: monster dead %v, caster HP %v of %d; want neither capped", damage, hostile.Dead(), want, srv.PlayerMaxHP(t, objID))
	}
	if got := drainCasterHP(t, srv, objID); got != want {
		t.Fatalf("caster HP after draining %v = %v, want %v", damage, got, want)
	}

	// The hit's final MP cost reports its own status first; the absorb
	// status is the last one ahead of the damage message.
	var absorbStatus map[int32]int32
	statusAttrs, sawDamage := 0, false
	for range 50 {
		frame := c.ReadWithTimeout(time.Second)
		if frame == nil {
			break
		}
		r := wireReader(frame[1:])
		switch frame[0] {
		case serverpackets.OpcodeSystemMessage:
			if r.ReadInt32() != int32(serverpackets.SystemMessageYouDidS1Dmg) {
				continue
			}
			r.ReadInt32() // parameter count
			r.ReadInt32() // number parameter type
			if got := r.ReadInt32(); float64(got) != damage {
				t.Fatalf("damage message = %d, want %v", got, damage)
			}
			sawDamage = true
		case serverpackets.OpcodeStatusUpdate:
			if r.ReadInt32() != objID {
				continue
			}
			n := int(r.ReadInt32())
			attrs := map[int32]int32{}
			for range n {
				typ := r.ReadInt32()
				attrs[typ] = r.ReadInt32()
			}
			if _, ok := attrs[int32(serverpackets.StatusCurrentHP)]; !ok {
				continue
			}
			if sawDamage {
				t.Fatalf("caster HP StatusUpdate %v after the damage message, want the absorb status only ahead of it", attrs)
			}
			absorbStatus, statusAttrs = attrs, n
		}
	}
	if !sawDamage {
		t.Fatal("damage message never arrived")
	}
	if statusAttrs != 4 || absorbStatus[int32(serverpackets.StatusCurrentHP)] != int32(want) {
		t.Fatalf("caster status ahead of the damage message = %v, want 4 attributes with CUR_HP %d", absorbStatus, int32(want))
	}
}

// TestLifeDrainHalfFailureSendsDrainHalfSuccessful forces the first magic
// success roll to fail and the second to pass, so a same-level drain takes
// the half-damage branch: the caster hears DRAIN_HALF_SUCCESFUL, never
// ATTACK_FAILED.
func TestLifeDrainHalfFailureSendsDrainHalfSuccessful(t *testing.T) {
	t.Parallel()
	srv := gameservertest.Boot(t,
		gameservertest.WithCharacter("Mage", 5, 0),
		gameservertest.WithWantChars(1),
		gameservertest.WithSkills(skillPersistence(t, []modelskill.Definition{lifeDrain(t)})),
	)
	c, objID := srv.Client, srv.SoleObjectID(t)
	seedKnownSkill(t, srv, objID, 1090, 1)
	startInWorld(t, c)
	rolls := 0
	setCasterMagicRolls(t, srv, objID, func() int {
		rolls++
		if rolls == 1 {
			return 0
		}
		return 9999
	})
	hostile := srv.SpawnHostileNPC(t)
	drainUntilQuiet(t, c)
	targetHostile(t, c, hostile.ObjectID())
	drainUntilQuiet(t, c)

	monsterBefore := hostile.HP()
	c.Send(encodeRequestMagicSkillUse(1090, false, false))
	srv.AdvanceUntil(t, "Life Drain half hit", func() bool { return hostile.HP() < monsterBefore })

	found := false
	for range 50 {
		frame := c.ReadWithTimeout(time.Second)
		if frame == nil {
			break
		}
		if frame[0] != serverpackets.OpcodeSystemMessage {
			continue
		}
		switch wireReader(frame[1:]).ReadInt32() {
		case int32(serverpackets.SystemMessageAttackFailed):
			t.Fatal("a drain half failure sent ATTACK_FAILED")
		case int32(serverpackets.SystemMessageDrainHalfSuccessful):
			found = true
		}
	}
	if !found {
		t.Fatal("DRAIN_HALF_SUCCESFUL never arrived")
	}
}

// TestReflectedDrainLandsItsEffectsFromTheReflector pins the DRAIN reflect
// swap: a monster that reflects magic bounces the drain's effects, which
// land on the caster with the monster as their effector, while the drain
// still damages the monster.
func TestReflectedDrainLandsItsEffectsFromTheReflector(t *testing.T) {
	t.Parallel()
	srv := gameservertest.Boot(t,
		gameservertest.WithCharacter("Mage", 5, 0),
		gameservertest.WithWantChars(1),
		gameservertest.WithSkills(skillPersistence(t, []modelskill.Definition{{
			ID: 90, Level: 1, Activation: modelskill.ActivationActive, Target: modelskill.TargetOne,
			CastRange: 900, HitTime: 500, ReuseDelay: 60_000, StaticHitTime: true, StaticReuse: true,
			SkillType: "DRAIN", Magic: true, Power: 20, AbsorbPart: 0.5, CanBeReflected: true,
			Effects: []modelskill.EffectTemplate{{Name: "Buff", Time: 9}},
		}})),
	)
	c, objID := srv.Client, srv.SoleObjectID(t)
	seedKnownSkill(t, srv, objID, 90, 1)
	startInWorld(t, c)
	setCasterMagicRolls(t, srv, objID, func() int { return 9999 })
	obj, ok := srv.State.Player(objID)
	if !ok {
		t.Fatalf("world.Player(%d) missing", objID)
	}
	caster, ok := obj.(interface {
		effect.Actor
		EffectList() *effect.List
	})
	if !ok {
		t.Fatalf("world.Player(%d) = %T is no effect participant", objID, obj)
	}
	x, y, z := caster.Position()
	hostile := srv.SpawnHostileNPCAt(t, location.Location{X: x + 100, Y: y, Z: z})
	hostile.AddStatFuncs([]effect.Mod{{Stat: stat.ReflectSkillMagic, Op: effect.OpAdd, Value: 100}})
	drainUntilQuiet(t, c)
	targetHostile(t, c, hostile.ObjectID())
	drainUntilQuiet(t, c)

	monsterBefore := hostile.HP()
	c.Send(encodeRequestMagicSkillUse(90, false, false))
	readCastStartFrames(t, c, objID, 90, 1, 500, 60_000, hostile.ObjectID())
	srv.AdvanceUntil(t, "reflected drain", func() bool { return len(caster.EffectList().All()) == 1 })

	if hostile.HP() >= monsterBefore {
		t.Fatalf("monster HP = %v, want the drain to damage it below %v", hostile.HP(), monsterBefore)
	}
	if held := hostile.EffectList().All(); len(held) != 0 {
		t.Fatalf("monster-held effects = %+v, want none", held)
	}
	e := caster.EffectList().All()[0]
	if e.Type != effect.TypeBuff || e.Effector.ObjectID() != hostile.ObjectID() || e.Effected.ObjectID() != objID {
		t.Fatalf("caster-held effect = %s by %d on %d, want Buff by monster %d on caster %d",
			e.Type, e.Effector.ObjectID(), e.Effected.ObjectID(), hostile.ObjectID(), objID)
	}
}
