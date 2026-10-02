package skills

import (
	"testing"

	"github.com/fatal10110/acis_golang/internal/gameserver/model/actor/npc"
	"github.com/fatal10110/acis_golang/internal/gameserver/model/actor/player"
	modelskill "github.com/fatal10110/acis_golang/internal/gameserver/model/skill"
	"github.com/fatal10110/acis_golang/internal/gameserver/network/serverpackets"
	"github.com/fatal10110/acis_golang/internal/gameservertest"
	"github.com/fatal10110/acis_golang/internal/testsupport"
)

// orderSkill is the caster's single-target skill in the message-order
// scenarios below.
const orderSkill = 46

// objectFrame matches a frame of opcode whose first field is objID.
func objectFrame(opcode byte, objID int32) func([]byte) bool {
	return func(frame []byte) bool {
		return frame[0] == opcode && wireReader(frame[1:]).ReadInt32() == objID
	}
}

// killFrames is where a killing skill's frames sit in the caster's log: the
// damage report, the monster's zero-HP status, the exp message and the
// monster's Die.
type killFrames struct{ dealt, status, reward, die int }

// castKillingSkill boots a Mage with def, has it kill the fixture monster
// with one cast under magic rolls that never fail or crit, and returns
// where the kill's frames sit in the Mage's log.
func castKillingSkill(t *testing.T, def modelskill.Definition) killFrames {
	t.Helper()
	srv := gameservertest.Boot(t,
		gameservertest.WithCharacter("Mage", 5, 0),
		gameservertest.WithWantChars(1),
		gameservertest.WithSkills(skillPersistence(t, []modelskill.Definition{def})),
	)
	c, objID := srv.Client, srv.SoleObjectID(t)
	seedKnownSkill(t, srv, objID, int(def.ID), def.Level)
	startInWorld(t, c)
	hostile := srv.SpawnHostileNPC(t)
	drainUntilQuiet(t, c)
	targetHostile(t, c, hostile.ObjectID())
	drainUntilQuiet(t, c)
	setCasterMagicRolls(t, srv, objID, func() int { return 500 })

	c.Send(encodeRequestMagicSkillUse(int32(def.ID), false, false))
	readCastStartFrames(t, c, objID, int32(def.ID), int32(def.Level), int32(def.HitTime), int32(def.ReuseDelay), hostile.ObjectID())
	srv.AdvanceUntil(t, "the killing hit", hostile.Dead)

	log := readFrameLog(c)
	got := killFrames{
		dealt: log.index(isSystemMessage(serverpackets.SystemMessageYouDidS1Dmg)),
		status: log.index(func(frame []byte) bool {
			hp, ok := statusValue(frame, hostile.ObjectID(), serverpackets.StatusCurrentHP)
			return ok && hp == 0
		}),
		reward: log.index(isSystemMessage(serverpackets.SystemMessageYouEarnedS1ExpAndS2SP)),
		die:    log.index(objectFrame(serverpackets.OpcodeDie, hostile.ObjectID())),
	}
	if got.dealt < 0 || got.status < 0 || got.reward < 0 || got.die < 0 {
		t.Fatalf("caster frames: YOU_DID_S1_DMG at %d, zero-HP StatusUpdate at %d, exp message at %d, Die at %d; want all present",
			got.dealt, got.status, got.reward, got.die)
	}
	return got
}

// TestMDamKillReportsTheDamageBeforeTheKill kills the fixture monster with
// an MDAM: the caster reads YOU_DID_S1_DMG before the monster's zero-HP
// status, the exp message and its Die, since MDAM reports the damage
// before applying it (issue #2589).
func TestMDamKillReportsTheDamageBeforeTheKill(t *testing.T) {
	t.Parallel()
	got := castKillingSkill(t, modelskill.Definition{
		ID: orderSkill, Level: 1, Activation: modelskill.ActivationActive, Target: modelskill.TargetOne,
		CastRange: 900, HitTime: 500, ReuseDelay: 60_000, StaticHitTime: true, StaticReuse: true,
		SkillType: "MDAM", Power: 1_000_000, Magic: true,
	})
	if !(got.dealt < got.status && got.status < got.reward && got.reward < got.die) {
		t.Fatalf("caster frame order: YOU_DID_S1_DMG %d, zero-HP StatusUpdate %d, exp message %d, Die %d; want that order",
			got.dealt, got.status, got.reward, got.die)
	}
}

// TestPDamKillReportsTheDamageAfterTheKill kills the fixture monster with a
// PDAM: a physical skill reports the damage after applying it, so the
// caster reads YOU_DID_S1_DMG after the monster's status, the exp message
// and its Die (issue #2589).
func TestPDamKillReportsTheDamageAfterTheKill(t *testing.T) {
	t.Parallel()
	got := castKillingSkill(t, modelskill.Definition{
		ID: orderSkill, Level: 1, Activation: modelskill.ActivationActive, Target: modelskill.TargetOne,
		CastRange: 900, HitTime: 500, ReuseDelay: 60_000, StaticHitTime: true, StaticReuse: true,
		SkillType: "PDAM", Power: 1_000_000,
	})
	if !(got.status < got.reward && got.reward < got.die && got.die < got.dealt) {
		t.Fatalf("caster frame order: zero-HP StatusUpdate %d, exp message %d, Die %d, YOU_DID_S1_DMG %d; want that order",
			got.status, got.reward, got.die, got.dealt)
	}
}

// TestBlowResistReportsBeforeTheTargetStatus lands a BLOW whose stun the
// fixture monster resists: the caster reads S1_RESISTED_YOUR_S2 before the
// monster's StatusUpdate with the HP the blow left, since the blow lands its
// effects before its damage (issue #2589). The stun's landing roll always
// fails.
func TestBlowResistReportsBeforeTheTargetStatus(t *testing.T) {
	t.Parallel()
	srv := gameservertest.Boot(t,
		gameservertest.WithCharacter("Rogue", 5, 0),
		gameservertest.WithWantChars(1),
		gameservertest.WithSkills(skillPersistence(t, []modelskill.Definition{{
			ID: orderSkill, Level: 1, Activation: modelskill.ActivationActive, Target: modelskill.TargetOne,
			CastRange: 900, HitTime: 500, StaticHitTime: true, StaticReuse: true,
			SkillType: "BLOW", Power: 1, Offensive: true, IgnoreResists: true, BaseLandRate: 1,
			Effects: []modelskill.EffectTemplate{{Name: "Stun", Time: 10}},
		}})),
	)
	c, objID := srv.Client, srv.SoleObjectID(t)
	seedKnownSkill(t, srv, objID, orderSkill, 1)
	startInWorld(t, c)
	hostile := srv.SpawnHostileNPC(t)
	drainUntilQuiet(t, c)
	targetHostile(t, c, hostile.ObjectID())
	drainUntilQuiet(t, c)
	// Every other caster roll comes up zero: the blow always lands, never
	// crits.
	resistLandingRolls(t, srv, objID, func(int) int { return 0 })

	for range 5 {
		hp := hostile.CurrentHP()
		c.Send(encodeRequestMagicSkillUse(orderSkill, false, false))
		readCastStartFrames(t, c, objID, orderSkill, 1, 500, 0, hostile.ObjectID())
		srv.AdvanceUntil(t, "the blow landing", func() bool { return hostile.CurrentHP() < hp })
		if hostile.Dead() {
			t.Fatal("monster died; the blow must leave it alive to report its status")
		}
		left := int32(hostile.CurrentHP())

		log := readFrameLog(c)
		resisted := log.index(isSystemMessage(serverpackets.SystemMessageS1ResistedYourS2))
		status := log.index(func(frame []byte) bool {
			got, ok := statusValue(frame, hostile.ObjectID(), serverpackets.StatusCurrentHP)
			return ok && got == left
		})
		if status < 0 {
			t.Fatalf("caster never read the monster's StatusUpdate with HP %d", left)
		}
		if resisted < 0 {
			continue // the stun landed; roll again
		}
		if resisted > status {
			t.Fatalf("caster frames: S1_RESISTED_YOUR_S2 at %d, monster StatusUpdate at %d; want the resist first", resisted, status)
		}
		return
	}
	t.Fatal("the monster never resisted the stun in five blows")
}

// TestMDamBreaksTheTargetCastBeforeTheDamageReport drives an MDAM into a
// monster mid-way through a magic cast under a breaking roll: the cast break
// rolls before the damage report, so the caster reads the monster's
// MagicSkillCanceled before YOU_DID_S1_DMG (issue #2589).
func TestMDamBreaksTheTargetCastBeforeTheDamageReport(t *testing.T) {
	t.Parallel()
	srv := gameservertest.Boot(t,
		gameservertest.WithCharacter("Mage", 5, 0),
		gameservertest.WithWantChars(1),
		gameservertest.WithSkills(skillPersistence(t, []modelskill.Definition{mdamStrike(10)})),
	)
	c, objID := srv.Client, srv.SoleObjectID(t)
	seedKnownSkill(t, srv, objID, feedbackStrikeSkill, 1)
	startInWorld(t, c)
	hostile, aiCtl := srv.SpawnCastingHostileNPC(t, &npc.Template{
		ID: 100, TemplateID: 100, Type: "Monster", Level: 1, HPMax: 1_000_000,
		AtkSpd: 300, RunSpeed: 120, WalkSpeed: 60, CollisionRadius: 8, CollisionHeight: 20,
	}, modelskill.NewTable([]modelskill.Definition{{
		ID: npcBreakSkill, Level: 1, Activation: modelskill.ActivationActive, Target: modelskill.TargetSelf,
		Magic: true, SkillType: "BUFF", HitTime: 10_000, StaticHitTime: true, StaticReuse: true,
	}}))
	onNPCQueue(t, hostile, func() { hostile.SetRollSource(func(int) int { return 0 }) })
	drainUntilQuiet(t, c)
	maxHP := targetHostile(t, c, hostile.ObjectID())
	drainUntilQuiet(t, c)
	setCasterMagicRolls(t, srv, objID, func() int { return 500 })
	startNPCCast(t, c, hostile, aiCtl)
	drainUntilQuiet(t, c)

	c.Send(encodeRequestMagicSkillUse(feedbackStrikeSkill, false, false))
	readCastStartFrames(t, c, objID, feedbackStrikeSkill, 1, 500, 60_000, hostile.ObjectID())
	srv.AdvanceUntil(t, "the MDAM landing", func() bool { return hostile.CurrentHP() < maxHP })

	log := readFrameLog(c)
	canceled := log.index(objectFrame(serverpackets.OpcodeMagicSkillCanceled, hostile.ObjectID()))
	dealt := log.index(isSystemMessage(serverpackets.SystemMessageYouDidS1Dmg))
	if canceled < 0 || dealt < 0 || canceled > dealt {
		t.Fatalf("caster frames: monster MagicSkillCanceled at %d, YOU_DID_S1_DMG at %d; want the cancel first", canceled, dealt)
	}
}

// breakingMonster is a caster beside a monster mid-way through a magic cast
// whose every combat roll comes up zero, so any damaging hit breaks it.
type breakingMonster struct {
	srv     *gameservertest.Server
	c       *testsupport.ScriptedClient
	objID   int32
	hostile *npc.Hostile
	maxHP   int
}

// bootBreakingMonster boots the caster knowing def, with the SignetMDam
// effect point on file, targets the monster and starts its cast.
func bootBreakingMonster(t *testing.T, def modelskill.Definition) *breakingMonster {
	t.Helper()
	srv := gameservertest.Boot(t,
		gameservertest.WithCharacter("Mage", 5, 0),
		gameservertest.WithWantChars(1),
		gameservertest.WithNPCs(npc.NewTable([]*npc.Template{{ID: 13018, Type: "EffectPoint", CollisionRadius: 8}})),
		gameservertest.WithSkills(skillPersistence(t, []modelskill.Definition{def})),
	)
	c, objID := srv.Client, srv.SoleObjectID(t)
	seedKnownSkill(t, srv, objID, int(def.ID), def.Level)
	startInWorld(t, c)
	// The caster stands inside its own signet: invulnerability keeps it
	// alive and its report of that strike a blocked one.
	onPlayerQueue(t, srv, objID, func(pc *player.Character) { pc.Live.SetInvul(true) })
	hostile, aiCtl := srv.SpawnCastingHostileNPC(t, &npc.Template{
		ID: 100, TemplateID: 100, Type: "Monster", Level: 1, HPMax: 1_000_000,
		AtkSpd: 300, RunSpeed: 120, WalkSpeed: 60, CollisionRadius: 8, CollisionHeight: 20,
	}, modelskill.NewTable([]modelskill.Definition{{
		ID: npcBreakSkill, Level: 1, Activation: modelskill.ActivationActive, Target: modelskill.TargetSelf,
		Magic: true, SkillType: "BUFF", HitTime: 10_000, StaticHitTime: true, StaticReuse: true,
	}}))
	onNPCQueue(t, hostile, func() { hostile.SetRollSource(func(int) int { return 0 }) })
	drainUntilQuiet(t, c)
	maxHP := targetHostile(t, c, hostile.ObjectID())
	drainUntilQuiet(t, c)
	setCasterMagicRolls(t, srv, objID, func() int { return 500 })
	startNPCCast(t, c, hostile, aiCtl)
	drainUntilQuiet(t, c)
	return &breakingMonster{srv: srv, c: c, objID: objID, hostile: hostile, maxHP: maxHP}
}

// TestSignetMDamBreaksTheMonsterCastBeforeTheDamageReport ticks a SignetMDam
// beside a casting monster under a breaking roll: the tick rolls the cast
// break before it reports the damage, so the caster reads the monster's
// MagicSkillCanceled before YOU_DID_S1_DMG (issue #2738).
func TestSignetMDamBreaksTheMonsterCastBeforeTheDamageReport(t *testing.T) {
	t.Parallel()
	def := signetMDamSkill()
	def.Power = 1
	w := bootBreakingMonster(t, def)

	w.c.Send(encodeRequestMagicSkillUse(int32(def.ID), false, false))
	readSignetCastStartFrames(t, w.c, w.objID, int32(def.ID), 1, int32(def.HitTime), int32(def.ReuseDelay), w.objID)
	tickSignetMDamLive(t, w.srv)
	if w.hostile.CurrentHP() >= w.maxHP {
		t.Fatal("the signet never damaged the monster")
	}

	// The tick reports each target's damage before its MagicSkillUse on
	// that target, so the monster's damage report is the last one ahead of
	// its MagicSkillUse; the caster's own report may come earlier.
	log := readFrameLog(w.c)
	use := log.index(func(frame []byte) bool {
		if frame[0] != serverpackets.OpcodeMagicSkillUse {
			return false
		}
		r := wireReader(frame[1:])
		caster, target := r.ReadInt32(), r.ReadInt32()
		return caster != w.objID && target == w.hostile.ObjectID()
	})
	dealt := -1
	if use > 0 {
		dealt = log[:use].lastIndex(isSystemMessage(serverpackets.SystemMessageYouDidS1Dmg))
	}
	canceled := log.index(objectFrame(serverpackets.OpcodeMagicSkillCanceled, w.hostile.ObjectID()))
	if canceled < 0 || dealt < 0 || canceled > dealt {
		t.Fatalf("caster frames: monster MagicSkillCanceled at %d, its YOU_DID_S1_DMG at %d, signet MagicSkillUse on it at %d; want the cancel first",
			canceled, dealt, use)
	}
}

// TestDrainBreaksTheMonsterCastBeforeItsEffects drains a casting monster
// under a breaking roll with an effect it always resists: DRAIN rolls the
// cast break, reports the damage, then lands its effects, so the caster
// reads the monster's MagicSkillCanceled, then YOU_DID_S1_DMG, then
// S1_RESISTED_YOUR_S2 (issue #2738).
func TestDrainBreaksTheMonsterCastBeforeItsEffects(t *testing.T) {
	t.Parallel()
	def := modelskill.Definition{
		ID: orderSkill, Level: 1, Activation: modelskill.ActivationActive, Target: modelskill.TargetOne,
		Offensive: true, CastRange: 900, HitTime: 500, ReuseDelay: 60_000, StaticHitTime: true, StaticReuse: true,
		SkillType: "DRAIN", Power: 10, Magic: true, IgnoreResists: true,
		Effects: []modelskill.EffectTemplate{{Name: "Debuff", Time: 60}},
	}
	w := bootBreakingMonster(t, def)

	w.c.Send(encodeRequestMagicSkillUse(int32(def.ID), false, false))
	readCastStartFrames(t, w.c, w.objID, int32(def.ID), 1, int32(def.HitTime), int32(def.ReuseDelay), w.hostile.ObjectID())
	w.srv.AdvanceUntil(t, "the DRAIN landing", func() bool { return w.hostile.CurrentHP() < w.maxHP })

	log := readFrameLog(w.c)
	canceled := log.index(objectFrame(serverpackets.OpcodeMagicSkillCanceled, w.hostile.ObjectID()))
	dealt := log.index(isSystemMessage(serverpackets.SystemMessageYouDidS1Dmg))
	resisted := log.index(isSystemMessage(serverpackets.SystemMessageS1ResistedYourS2))
	if canceled < 0 || dealt < 0 || resisted < 0 || canceled > dealt || dealt > resisted {
		t.Fatalf("caster frames: monster MagicSkillCanceled at %d, YOU_DID_S1_DMG at %d, S1_RESISTED_YOUR_S2 at %d; want that order",
			canceled, dealt, resisted)
	}
}
