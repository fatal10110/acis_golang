package skills

import (
	"testing"

	"github.com/fatal10110/acis_golang/internal/gameserver/model/actor/npc"
	"github.com/fatal10110/acis_golang/internal/gameserver/model/actor/player"
	modelskill "github.com/fatal10110/acis_golang/internal/gameserver/model/skill"
	"github.com/fatal10110/acis_golang/internal/gameserver/network/serverpackets"
	"github.com/fatal10110/acis_golang/internal/gameserver/skill/effect"
	"github.com/fatal10110/acis_golang/internal/gameservertest"
	"github.com/fatal10110/acis_golang/internal/testsupport"
)

// deniedStrikeSkill is the damage-denied caster's single-target skill.
const deniedStrikeSkill = 47

// deniedStrike is deniedStrikeSkill as skillType, strong enough to kill
// the fixture monster were its damage let through.
func deniedStrike(skillType string) modelskill.Definition {
	return modelskill.Definition{
		ID: deniedStrikeSkill, Level: 1, Activation: modelskill.ActivationActive, Target: modelskill.TargetOne,
		Offensive: true, CastRange: 900, HitTime: 500, ReuseDelay: 60_000, StaticHitTime: true, StaticReuse: true,
		SkillType: skillType, Power: 1_000_000,
	}
}

// deniedCaster is a player whose access level forbids dealing damage,
// targeting the fixture monster with def.
type deniedCaster struct {
	srv     *gameservertest.Server
	c       *testsupport.ScriptedClient
	objID   int32
	hostile *npc.Hostile
	maxHP   int
}

func bootDeniedCaster(t *testing.T, def modelskill.Definition) *deniedCaster {
	t.Helper()
	srv := gameservertest.Boot(t,
		gameservertest.WithCharacter("Newbie", 5, 0),
		gameservertest.WithWantChars(1),
		gameservertest.WithSkills(skillPersistence(t, []modelskill.Definition{def})),
	)
	c, objID := srv.Client, srv.SoleObjectID(t)
	seedKnownSkill(t, srv, objID, int(def.ID), def.Level)
	startInWorld(t, c)
	denyCasterDamage(t, srv, objID)
	hostile := srv.SpawnHostileNPC(t)
	drainUntilQuiet(t, c)
	maxHP := targetHostile(t, c, hostile.ObjectID())
	drainUntilQuiet(t, c)
	return &deniedCaster{srv: srv, c: c, objID: objID, hostile: hostile, maxHP: maxHP}
}

// cast casts the skill at the monster, lets the cast finish and returns
// every frame the caster read.
func (d *deniedCaster) cast(t *testing.T, def modelskill.Definition) frameLog {
	t.Helper()
	d.c.Send(encodeRequestMagicSkillUse(int32(def.ID), false, false))
	readCastStartFrames(t, d.c, d.objID, int32(def.ID), int32(def.Level), int32(def.HitTime), int32(def.ReuseDelay), d.hostile.ObjectID())
	obj, ok := d.srv.State.Player(d.objID)
	if !ok {
		t.Fatalf("world.Player(%d) missing", d.objID)
	}
	caster := obj.(interface{ CastingNow() bool })
	d.srv.AdvanceUntil(t, "the cast ending", func() bool { return !caster.CastingNow() })
	d.srv.Settle(t)
	return readFrameLog(d.c)
}

func (d *deniedCaster) assertMonsterUnharmed(t *testing.T) {
	t.Helper()
	if hp := d.hostile.CurrentHP(); hp != d.maxHP {
		t.Fatalf("monster HP after the damage-denied cast = %d, want unchanged %d", hp, d.maxHP)
	}
}

// TestDamageDeniedPDamReportsAttackFailed casts a PDAM carrying an
// offensive effect from a caster whose access level forbids dealing damage
// (issue #2740): the hit still resolves and works out to no damage, so the
// caster reads ATTACK_FAILED rather than nothing, and the monster keeps its
// HP. The offensive effect is refused, as every offensive landing is for a
// damage-denied effector.
func TestDamageDeniedPDamReportsAttackFailed(t *testing.T) {
	t.Parallel()
	def := deniedStrike("PDAM")
	def.Effects = []modelskill.EffectTemplate{{Name: "Debuff", Time: 60}}
	d := bootDeniedCaster(t, def)

	log := d.cast(t, def)
	if log.index(isSystemMessage(serverpackets.SystemMessageAttackFailed)) < 0 {
		t.Fatal("damage-denied caster never read ATTACK_FAILED for its PDAM")
	}
	if at := log.index(isSystemMessage(serverpackets.SystemMessageYouDidS1Dmg)); at >= 0 {
		t.Fatalf("damage-denied PDAM reported damage at frame %d, want none", at)
	}
	d.assertMonsterUnharmed(t)
	if got := len(d.hostile.EffectList().All()); got != 0 {
		t.Fatalf("monster effects after the damage-denied PDAM = %d, want 0", got)
	}
}

// TestDamageDeniedChargeDamReportsZeroDamage casts a CHARGEDAM from a
// damage-denied caster (issue #2740): a CHARGEDAM hit that works out to no
// damage still goes through the damage path, so the caster reads
// YOU_DID_S1_DMG 0, and the monster keeps its HP.
func TestDamageDeniedChargeDamReportsZeroDamage(t *testing.T) {
	t.Parallel()
	def := deniedStrike("CHARGEDAM")
	d := bootDeniedCaster(t, def)

	if got := youDidDamage(t, d.cast(t, def)); got != 0 {
		t.Fatalf("damage-denied CHARGEDAM reported %d damage, want 0", got)
	}
	d.assertMonsterUnharmed(t)
}

// TestDamageDeniedBlowReportsItsDamageButLeavesHP lands a BLOW from a
// damage-denied caster (issue #2740): the blow formula carries no damage
// permission gate, so the caster reads YOU_DID_S1_DMG with the blow's full
// damage, while the monster's HP reduction refuses it.
func TestDamageDeniedBlowReportsItsDamageButLeavesHP(t *testing.T) {
	t.Parallel()
	def := deniedStrike("BLOW")
	def.Power, def.BaseLandRate = 100, 1
	d := bootDeniedCaster(t, def)
	// Every caster roll comes up zero: the blow always lands, never crits.
	onPlayerQueue(t, d.srv, d.objID, func(pc *player.Character) { pc.SetRollSource(func(int) int { return 0 }) })

	if got := youDidDamage(t, d.cast(t, def)); got <= 0 {
		t.Fatalf("damage-denied BLOW reported %d damage, want the blow's positive damage", got)
	}
	d.assertMonsterUnharmed(t)
}

// TestDamageDeniedChargeDamWakesSleepingPlayer casts a CHARGEDAM from a
// damage-denied player at a sleeping one (issue #2740): the zero-damage hit
// still runs the victim's hit side effects ahead of the damage-permission
// gate, so the victim's sleep ends, and its HP is untouched.
func TestDamageDeniedChargeDamWakesSleepingPlayer(t *testing.T) {
	t.Parallel()
	def := deniedStrike("CHARGEDAM")
	def.ID = feedbackStrikeSkill
	p := bootPVPPair(t, def, 60)
	denyCasterDamage(t, p.srv, p.mageID)
	sleeping := func() (asleep bool) {
		onPlayerQueue(t, p.srv, p.victimID, func(pc *player.Character) {
			for _, e := range pc.EffectList().All() {
				asleep = asleep || e.Type == effect.TypeSleep
			}
		})
		return asleep
	}
	onPlayerQueue(t, p.srv, p.victimID, func(pc *player.Character) {
		e, err := effect.New(effect.Skill{ID: 101, Level: 1, Debuff: true}, modelskill.EffectTemplate{Name: "Sleep", Time: 30})
		if err != nil {
			t.Errorf("effect.New(Sleep): %v", err)
			return
		}
		e.Effector, e.Effected = pc, pc
		pc.EffectList().Add(e)
	})
	if !sleeping() {
		t.Fatal("the Victim is not asleep before the strike")
	}
	drainUntilQuiet(t, p.vc)
	drainUntilQuiet(t, p.c)
	before := p.srv.PlayerCurrentHP(t, p.victimID)

	mage, ok := p.srv.State.Player(p.mageID)
	if !ok {
		t.Fatalf("world.Player(%d) missing", p.mageID)
	}
	casting := mage.(interface{ CastingNow() bool })
	// Forced, since the Victim is an unflagged innocent.
	p.c.Send(encodeRequestMagicSkillUse(feedbackStrikeSkill, true, false))
	readCastStartFrames(t, p.c, p.mageID, feedbackStrikeSkill, 1, p.strikeHit, p.strikeR, p.victimID)
	p.srv.AdvanceUntil(t, "the Mage's CHARGEDAM ending", func() bool { return !casting.CastingNow() })

	if sleeping() {
		t.Fatal("the Victim still sleeps after the damage-denied CHARGEDAM, want it woken")
	}
	if got := youDidDamage(t, readFrameLog(p.c)); got != 0 {
		t.Fatalf("damage-denied CHARGEDAM reported %d damage, want 0", got)
	}
	if got := p.srv.PlayerCurrentHP(t, p.victimID); got != before {
		t.Fatalf("Victim HP after the damage-denied CHARGEDAM = %d, want unchanged %d", got, before)
	}
}

// TestDamageDeniedSignetMDamStillShowsItsStrike ticks a SignetMDam from a
// damage-denied caster beside the fixture monster (issue #2740): the tick
// still resolves the monster as a target, so its per-target MagicSkillUse
// reaches the caster, with no damage report and no HP lost.
func TestDamageDeniedSignetMDamStillShowsItsStrike(t *testing.T) {
	t.Parallel()
	def := signetMDamSkill()
	srv := gameservertest.Boot(t,
		gameservertest.WithCharacter("Mage", 5, 0),
		gameservertest.WithWantChars(1),
		gameservertest.WithNPCs(npc.NewTable([]*npc.Template{{ID: 13018, Type: "EffectPoint", CollisionRadius: 8}})),
		gameservertest.WithSkills(skillPersistence(t, []modelskill.Definition{def})),
	)
	c, objID := srv.Client, srv.SoleObjectID(t)
	seedKnownSkill(t, srv, objID, int(def.ID), def.Level)
	startInWorld(t, c)
	denyCasterDamage(t, srv, objID)
	hostile := srv.SpawnHostileNPC(t)
	drainUntilQuiet(t, c)
	maxHP := hostile.CurrentHP()

	c.Send(encodeRequestMagicSkillUse(int32(def.ID), false, false))
	readSignetCastStartFrames(t, c, objID, int32(def.ID), 1, int32(def.HitTime), int32(def.ReuseDelay), objID)
	tickSignetMDamLive(t, srv)

	log := readFrameLog(c)
	strike := log.index(func(frame []byte) bool {
		if frame[0] != serverpackets.OpcodeMagicSkillUse {
			return false
		}
		r := wireReader(frame[1:])
		caster, target := r.ReadInt32(), r.ReadInt32()
		return caster != objID && target == hostile.ObjectID()
	})
	if strike < 0 {
		t.Fatal("caster never read the signet's MagicSkillUse on the monster")
	}
	if at := log.index(isSystemMessage(serverpackets.SystemMessageYouDidS1Dmg)); at >= 0 {
		t.Fatalf("damage-denied signet reported damage at frame %d, want none", at)
	}
	if hp := hostile.CurrentHP(); hp != maxHP {
		t.Fatalf("monster HP after the damage-denied signet = %d, want unchanged %d", hp, maxHP)
	}
}
