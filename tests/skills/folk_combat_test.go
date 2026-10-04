package skills

import (
	"sync/atomic"
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
		IgnoreResists: true, EffectPower: 100,
		Effects: []modelskill.EffectTemplate{{Name: "Debuff", Time: 60}, {Name: "Stun", Time: 10}},
	}
}

// folkCaster boots a caster knowing def, in world, with a merchant spawned
// dx units east of it and selected. It returns the merchant's max HP as
// its selection reported it. opts apply after the defaults.
func folkCaster(t *testing.T, def modelskill.Definition, dx int, opts ...gameservertest.Option) (srv *gameservertest.Server, c *testsupport.ScriptedClient, objID int32, origin location.Location, folk *npc.Folk, maxHP int) {
	t.Helper()
	srv = gameservertest.Boot(t, append([]gameservertest.Option{
		gameservertest.WithCharacter("Newbie", 5, 0),
		gameservertest.WithWantChars(1),
		gameservertest.WithSkills(skillPersistence(t, []modelskill.Definition{def})),
		gameservertest.WithAttackStanceClock(time.Now),
	}, opts...)...)
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

	regen := task.NewNPCRegen(srv.State)
	srv.AdvanceUntil(t, "the regen tick", func() bool {
		regen.Tick()
		return folk.CurrentHP() > 1
	})
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

// TestFolkAttackStanceExpiryShowsAutoAttackStop pins the end of a hit
// civilian NPC's attack stance (AttackStanceTaskManager expiry ->
// Creature.broadcastPacket(AutoAttackStop)): once the stance period passes
// with no new hit, the player watching it reads AutoAttackStop for the NPC
// and the NPC leaves combat.
func TestFolkAttackStanceExpiryShowsAutoAttackStop(t *testing.T) {
	t.Parallel()
	var nowMS atomic.Int64
	nowMS.Store(time.Now().UnixMilli())
	clock := func() time.Time { return time.UnixMilli(nowMS.Load()) }
	srv, c, objID, _, folk, maxHP := folkCaster(t, folkNuke(1), 40, gameservertest.WithAttackStanceClock(clock))

	c.Send(encodeRequestMagicSkillUse(folkNukeID, true, false))
	readCastStartFrames(t, c, objID, folkNukeID, 1, 500, 60_000, folk.ObjectID())
	srv.AdvanceUntil(t, "the nuke landing", func() bool { return folk.CurrentHP() < maxHP })
	if log := readFrameLog(c); log.index(objectFrame(serverpackets.OpcodeAutoAttackStart, folk.ObjectID())) < 0 {
		t.Fatal("the hit merchant showed no AutoAttackStart")
	}
	if !folk.InCombat() {
		t.Fatal("the hit merchant is not in combat")
	}

	nowMS.Add((task.AttackStancePeriod + time.Second).Milliseconds())
	if err := srv.AttackStance.Tick(); err != nil {
		t.Fatalf("AttackStance.Tick() = %v", err)
	}
	srv.Settle(t)
	if log := readFrameLog(c); log.index(objectFrame(serverpackets.OpcodeAutoAttackStop, folk.ObjectID())) < 0 {
		t.Fatal("the merchant's attack stance expired without an AutoAttackStop")
	}
	if folk.InCombat() {
		t.Fatal("the merchant is still in combat after its stance expired")
	}
}

// landOnFolk lands an effect of skill id built from tmpl on folk, on the
// NPC's own queue.
func landOnFolk(t *testing.T, folk *npc.Folk, id int, tmpl modelskill.EffectTemplate) {
	t.Helper()
	e, err := effect.New(effect.Skill{ID: modelskill.ID(id), Level: 1}, tmpl)
	if err != nil {
		t.Fatalf("effect.New(%s): %v", tmpl.Name, err)
	}
	e.Effector, e.Effected = folk, folk
	done := make(chan struct{})
	if !folk.Queue().Post(func() { folk.EffectList().Add(e); close(done) }) {
		t.Fatal("post to merchant queue: queue closed")
	}
	select {
	case <-done:
	case <-time.After(10 * time.Second):
		t.Fatal("effect add on the merchant's queue did not complete")
	}
	held := false
	for _, h := range folk.EffectList().All() {
		held = held || h == e
	}
	if !held {
		t.Fatalf("merchant does not hold the %s effect of skill %d", tmpl.Name, id)
	}
}

// folkFrames returns the frames among log that name objID with opcode.
func folkFrames(log frameLog, opcode byte, objID int32) frameLog {
	var out frameLog
	match := objectFrame(opcode, objID)
	for _, frame := range log {
		if match(frame) {
			out = append(out, frame)
		}
	}
	return out
}

// TestFolkStatBuffBroadcast pins how a civilian NPC shows a buff's stat
// change (Creature.broadcastModifiedStats): attack and casting speed go out
// in one StatusUpdate with no MAX_HP, which only an Attackable sends; a
// max-HP change alone sends nothing; a run-speed change sends the NPC's
// whole info instead of a StatusUpdate, NpcInfo when it can move and
// ServerObjectInfo when its move speed is 0.
func TestFolkStatBuffBroadcast(t *testing.T) {
	t.Parallel()
	srv, c, objID, _, folk, _ := folkCaster(t, folkNuke(1), 40)
	x, y, z := srv.PlayerPosition(t, objID)
	still := gameservertest.FolkTemplate("Merchant", 30002)
	still.RunSpeed, still.WalkSpeed = 0, 0
	statue := srv.SpawnFolkNPCAt(t, still, location.Location{X: x - 40, Y: y, Z: z})
	drainUntilQuiet(t, c)
	baseAtk, baseCast := folk.AttackSpeed(), folk.MagicAttackSpeed()

	// Attack and casting speed: one StatusUpdate with both, and no MAX_HP.
	landOnFolk(t, folk, 1086, modelskill.EffectTemplate{
		Name: "Buff", Time: 60, Count: 1, Icon: true, StackType: "speed_both", StackOrder: 1,
		Funcs: []modelskill.FuncTemplate{
			{Op: modelskill.FuncMul, Stat: "pAtkSpd", Value: 1.2},
			{Op: modelskill.FuncMul, Stat: "mAtkSpd", Value: 1.2},
		},
	})
	log := readFrameLog(c)
	if folk.AttackSpeed() == baseAtk || folk.MagicAttackSpeed() == baseCast {
		t.Fatalf("speeds after the buff = %d/%d, want both raised from %d/%d", folk.AttackSpeed(), folk.MagicAttackSpeed(), baseAtk, baseCast)
	}
	su := speedStatusUpdatesOf(log, folk.ObjectID())
	if len(su) != 1 || su[0].attrs[statusAtkSpd] != int32(folk.AttackSpeed()) || su[0].attrs[statusCastSpd] != int32(folk.MagicAttackSpeed()) {
		t.Fatalf("speed StatusUpdates of the merchant = %+v, want one with ATK_SPD %d and CAST_SPD %d", su, folk.AttackSpeed(), folk.MagicAttackSpeed())
	}
	assertNoFolkMaxHP(t, log, folk.ObjectID())
	if got := folkFrames(log, serverpackets.OpcodeNPCInfo, folk.ObjectID()); len(got) != 0 {
		t.Fatalf("an attack-speed buff sent %d NpcInfo frames of the merchant, want none", len(got))
	}

	// Max HP alone: nothing.
	landOnFolk(t, folk, 1045, modelskill.EffectTemplate{
		Name: "Buff", Time: 60, Count: 1, Icon: true, StackType: "max_hp_up", StackOrder: 1,
		Funcs: []modelskill.FuncTemplate{{Op: modelskill.FuncAdd, Stat: "maxHp", Value: 500}},
	})
	log = readFrameLog(c)
	if got := folkFrames(log, serverpackets.OpcodeStatusUpdate, folk.ObjectID()); len(got) != 0 {
		t.Fatalf("a max-HP buff sent %d StatusUpdates of the merchant, want none", len(got))
	}
	if got := folkFrames(log, serverpackets.OpcodeNPCInfo, folk.ObjectID()); len(got) != 0 {
		t.Fatalf("a max-HP buff sent %d NpcInfo frames of the merchant, want none", len(got))
	}

	// Run speed on a moving NPC: NpcInfo, no StatusUpdate.
	runSpeed := modelskill.EffectTemplate{
		Name: "Buff", Time: 60, Count: 1, Icon: true, StackType: "speed_up", StackOrder: 1,
		Funcs: []modelskill.FuncTemplate{{Op: modelskill.FuncMul, Stat: "runSpd", Value: 1.5}},
	}
	landOnFolk(t, folk, 1204, runSpeed)
	log = readFrameLog(c)
	if got := folkFrames(log, serverpackets.OpcodeNPCInfo, folk.ObjectID()); len(got) != 1 {
		t.Fatalf("a run-speed buff sent %d NpcInfo frames of the merchant, want 1", len(got))
	}
	if got := folkFrames(log, serverpackets.OpcodeServerObjectInfo, folk.ObjectID()); len(got) != 0 {
		t.Fatalf("a run-speed buff on a moving merchant sent %d ServerObjectInfo frames, want none", len(got))
	}
	if got := folkFrames(log, serverpackets.OpcodeStatusUpdate, folk.ObjectID()); len(got) != 0 {
		t.Fatalf("a run-speed buff sent %d StatusUpdates of the merchant, want none", len(got))
	}

	// Run speed on an NPC whose move speed is 0: ServerObjectInfo.
	if statue.MoveSpeed() != 0 {
		t.Fatalf("still merchant move speed = %v, want 0", statue.MoveSpeed())
	}
	landOnFolk(t, statue, 1204, runSpeed)
	log = readFrameLog(c)
	if got := folkFrames(log, serverpackets.OpcodeServerObjectInfo, statue.ObjectID()); len(got) != 1 {
		t.Fatalf("a run-speed buff on a still merchant sent %d ServerObjectInfo frames, want 1", len(got))
	}
	if got := folkFrames(log, serverpackets.OpcodeNPCInfo, statue.ObjectID()); len(got) != 0 {
		t.Fatalf("a run-speed buff on a still merchant sent %d NpcInfo frames, want none", len(got))
	}
}

// assertNoFolkMaxHP fails when log carries a StatusUpdate of objID with
// MAX_HP.
func assertNoFolkMaxHP(t *testing.T, log frameLog, objID int32) {
	t.Helper()
	if i := log.index(func(frame []byte) bool {
		_, ok := statusValue(frame, objID, serverpackets.StatusMaxHP)
		return ok
	}); i >= 0 {
		t.Fatalf("frame %d is a StatusUpdate of the civilian NPC with MAX_HP, which only an Attackable sends", i)
	}
}
