package skills

import (
	"testing"
	"time"

	"github.com/fatal10110/acis_golang/internal/gameserver/model/actor/npc"
	"github.com/fatal10110/acis_golang/internal/gameserver/model/location"
	modelskill "github.com/fatal10110/acis_golang/internal/gameserver/model/skill"
	"github.com/fatal10110/acis_golang/internal/gameserver/network/serverpackets"
	"github.com/fatal10110/acis_golang/internal/gameserver/skill/effect"
	"github.com/fatal10110/acis_golang/internal/gameserver/task"
	"github.com/fatal10110/acis_golang/internal/gameservertest"
	"github.com/fatal10110/acis_golang/internal/testsupport"
)

const (
	folkNukeID    = 47
	folkNukeRange = 300
)

// folkNuke is a ONE-target physical nuke that always lands its debuff and
// its stun when it hits.
func folkNuke(power int) modelskill.Definition {
	return modelskill.Definition{
		ID: folkNukeID, Level: 1, Activation: modelskill.ActivationActive, Target: modelskill.TargetOne,
		Offensive: true, SkillType: "PDAM", Power: float32(power), CastRange: folkNukeRange,
		HitTime: 500, ReuseDelay: 60_000, StaticHitTime: true, StaticReuse: true,
		IgnoreResists: true, BaseLandRate: 100,
		Effects: []modelskill.EffectTemplate{{Name: "Debuff", Time: 60}, {Name: "Stun", Time: 10}},
	}
}

// folkCaster boots a caster knowing def, in world, with a merchant spawned
// dx units east of it and selected. It returns the merchant's max HP as
// its selection reported it.
func folkCaster(t *testing.T, def modelskill.Definition, dx int) (srv *gameservertest.Server, c *testsupport.ScriptedClient, objID int32, origin location.Location, folk *npc.Folk, maxHP int) {
	t.Helper()
	srv = gameservertest.Boot(t,
		gameservertest.WithCharacter("Newbie", 5, 0),
		gameservertest.WithWantChars(1),
		gameservertest.WithSkills(skillPersistence(t, []modelskill.Definition{def})),
		gameservertest.WithAttackStanceClock(time.Now),
	)
	c, objID = srv.Client, srv.SoleObjectID(t)
	seedKnownSkill(t, srv, objID, int(def.ID), def.Level)
	startInWorld(t, c)
	x, y, z := srv.PlayerPosition(t, objID)
	origin = location.Location{X: x, Y: y, Z: z}
	at := location.Location{X: x + dx, Y: y, Z: z}
	folk = srv.SpawnFolkNPCAt(t, gameservertest.FolkTemplate("Merchant", 30001), at)
	drainUntilQuiet(t, c)
	c.Send(encodeAction(folk.ObjectID(), int32(at.X), int32(at.Y), int32(at.Z), false))
	log := readFrameLog(c)
	status := log.index(func(frame []byte) bool {
		_, ok := statusValue(frame, folk.ObjectID(), serverpackets.StatusMaxHP)
		return ok
	})
	if status < 0 {
		t.Fatal("selecting the merchant sent no StatusUpdate with its max HP")
	}
	v, _ := statusValue(log[status], folk.ObjectID(), serverpackets.StatusMaxHP)
	return srv, c, objID, origin, folk, int(v)
}

// TestCtrlDamageSkillOnFolkStopsAtOneHP pins a CTRL damage skill on a
// civilian NPC (TargetOne.java:78-86 lets it through): the hit takes its
// damage, but an undying NPC keeps 1 HP (CreatureStatus.reduceHp,
// Npc.java:195 setMortal(!isUndying())) and does not die. The player
// targeting it reads the 1 HP, and it enters its attack stance. Of the
// skill's effects only the debuff lands (Folk.addEffect keeps EffectBuff
// and EffectDebuff only).
func TestCtrlDamageSkillOnFolkStopsAtOneHP(t *testing.T) {
	t.Parallel()
	srv, c, objID, _, folk, maxHP := folkCaster(t, folkNuke(1_000_000), 40)
	if maxHP <= 1 || folk.CurrentHP() != maxHP {
		t.Fatalf("merchant HP before the hit = %d of %d, want full", folk.CurrentHP(), maxHP)
	}

	c.Send(encodeRequestMagicSkillUse(folkNukeID, true, false))
	readCastStartFrames(t, c, objID, folkNukeID, 1, 500, 60_000, folk.ObjectID())
	srv.AdvanceUntil(t, "the nuke landing", func() bool { return folk.CurrentHP() < maxHP })

	if hp := folk.CurrentHP(); hp != 1 || folk.Dead() {
		t.Fatalf("merchant after a lethal nuke: HP %d dead %v, want 1 HP and alive", hp, folk.Dead())
	}
	log := readFrameLog(c)
	if log.index(func(frame []byte) bool {
		hp, ok := statusValue(frame, folk.ObjectID(), serverpackets.StatusCurrentHP)
		return ok && hp == 1
	}) < 0 {
		t.Fatal("the targeting player read no StatusUpdate with the merchant at 1 HP")
	}
	if log.index(objectFrame(serverpackets.OpcodeAutoAttackStart, folk.ObjectID())) < 0 {
		t.Fatal("the hit merchant showed no AutoAttackStart")
	}
	if log.index(objectFrame(serverpackets.OpcodeDie, folk.ObjectID())) >= 0 {
		t.Fatal("the merchant was shown dying")
	}
	if _, ok := srv.State.Object(folk.ObjectID()); !ok {
		t.Fatal("the merchant left the world")
	}

	var debuff, stun bool
	for _, e := range folk.EffectList().All() {
		debuff = debuff || e.Type == effect.TypeDebuff
		stun = stun || e.Type == effect.TypeStun
	}
	if !debuff || stun {
		t.Fatalf("merchant effects: debuff %v stun %v, want the debuff only", debuff, stun)
	}
}

// TestDamagedFolkRegenerates pins a hit civilian NPC's regeneration: the
// NPC regen task restores its HP, and the player targeting it reads the
// new value.
func TestDamagedFolkRegenerates(t *testing.T) {
	t.Parallel()
	srv, c, objID, _, folk, maxHP := folkCaster(t, folkNuke(1_000_000), 40)
	c.Send(encodeRequestMagicSkillUse(folkNukeID, true, false))
	readCastStartFrames(t, c, objID, folkNukeID, 1, 500, 60_000, folk.ObjectID())
	srv.AdvanceUntil(t, "the nuke landing", func() bool { return folk.CurrentHP() < maxHP })
	readFrameLog(c)

	task.NewNPCRegen(srv.State).Tick()
	srv.AdvanceUntil(t, "the regen tick", func() bool { return folk.CurrentHP() > 1 })
	hp := folk.CurrentHP()
	if log := readFrameLog(c); log.index(func(frame []byte) bool {
		v, ok := statusValue(frame, folk.ObjectID(), serverpackets.StatusCurrentHP)
		return ok && int(v) == hp
	}) < 0 {
		t.Fatalf("the targeting player read no StatusUpdate with the regenerated %d HP", hp)
	}
}

// TestCtrlCastWalksToFolk pins the cast approach on a civilian NPC beyond
// the cast range (PlayerAI.thinkCast): the player walks to it with
// MoveToPawn at the cast range instead of being refused.
func TestCtrlCastWalksToFolk(t *testing.T) {
	t.Parallel()
	_, c, objID, origin, folk, _ := folkCaster(t, folkNuke(1), 800)

	c.Send(encodeRequestMagicSkillUse(folkNukeID, true, false))
	assertMoveToPawn(t, c.Read(), objID, folk.ObjectID(), folkNukeRange, origin)
}

// TestHelpfulSkillOnFolkLandsWithoutPvPFlag pins a buff cast on a civilian
// NPC: the buff lands (Folk.addEffect keeps EffectBuff), and, the NPC being
// no attackable NPC, the caster is not PvP-flagged (CreatureCast.callSkill
// flags a helpful skill only on a playable or a non-guard Attackable).
func TestHelpfulSkillOnFolkLandsWithoutPvPFlag(t *testing.T) {
	t.Parallel()
	const buffID = 48
	srv, c, objID, _, folk, _ := folkCaster(t, modelskill.Definition{
		ID: buffID, Level: 1, Activation: modelskill.ActivationActive, Target: modelskill.TargetOne,
		SkillType: "BUFF", CastRange: folkNukeRange, HitTime: 500, ReuseDelay: 60_000, StaticHitTime: true, StaticReuse: true,
		Effects: []modelskill.EffectTemplate{{Name: "Buff", Time: 60, Icon: true}},
	}, 40)

	c.Send(encodeRequestMagicSkillUse(buffID, false, false))
	readCastStartFrames(t, c, objID, buffID, 1, 500, 60_000, folk.ObjectID())
	srv.AdvanceUntil(t, "the buff landing", func() bool { return len(folk.EffectList().All()) > 0 })
	if got := folk.EffectList().All()[0].Type; got != effect.TypeBuff {
		t.Fatalf("merchant effect = %s, want the buff", got)
	}
	obj, ok := srv.State.Player(objID)
	if !ok {
		t.Fatal("player missing from world state")
	}
	if flag := obj.(interface{ PvPFlagState() task.PvPFlagState }).PvPFlagState(); flag != task.PvPFlagNone {
		t.Fatalf("caster PvP flag after buffing a civilian NPC = %v, want none", flag)
	}
}
