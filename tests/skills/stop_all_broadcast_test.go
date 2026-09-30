package skills

import (
	"testing"

	"github.com/fatal10110/acis_golang/internal/gameserver/model/actor/attackable"
	"github.com/fatal10110/acis_golang/internal/gameserver/model/location"
	modelskill "github.com/fatal10110/acis_golang/internal/gameserver/model/skill"
	"github.com/fatal10110/acis_golang/internal/gameserver/network/serverpackets"
	"github.com/fatal10110/acis_golang/internal/gameserver/skill/effect"
)

// Reference: a stop-all ends every effect with exit(true), which sets
// cantUpdateAnymore (EffectList.java:306-333, AbstractEffect.java:220-226),
// and Creature.removeStatsByOwner skips broadcastModifiedStats for such an
// effect (Creature.java:1198-1204): no per-effect UserInfo, StatusUpdate or
// NpcInfo. A player's stop-all then runs one updateAndBroadcastStatus(2)
// (Player.java:4953-4964): UserInfo to itself and CharInfo to its observers,
// ahead of the Die broadcast the dead AI event sends (Playable.java:141-169,
// CreatureAI.java:77-86). An NPC's stop-all sends nothing more.

// statBuffs are a Might-shaped P.Atk buff and a Haste-shaped attack-speed
// buff, whose ends each send a stat refresh outside a stop-all.
var statBuffs = []struct {
	meta effect.Skill
	tmpl modelskill.EffectTemplate
}{
	{effect.Skill{ID: 1068, Level: 1}, modelskill.EffectTemplate{
		Name: "Buff", Time: 60, Count: 1, Icon: true, StackType: "pa_up", StackOrder: 1,
		Funcs: []modelskill.FuncTemplate{{Op: modelskill.FuncMul, Stat: "pAtk", Value: 1.5}},
	}},
	{effect.Skill{ID: 1086, Level: 1}, modelskill.EffectTemplate{
		Name: "Buff", Time: 60, Count: 1, Icon: true, StackType: "attack_time_down", StackOrder: 1,
		Funcs: []modelskill.FuncTemplate{{Op: modelskill.FuncMul, Stat: "pAtkSpd", Value: 1.33}},
	}},
}

func speedStatusUpdatesOf(frames [][]byte, objectID int32) []speedStatusUpdate {
	var out []speedStatusUpdate
	for _, su := range speedStatusUpdates(frames) {
		if su.objectID == objectID {
			out = append(out, su)
		}
	}
	return out
}

// TestDeathStripSendsOneClosingUserInfo kills a player holding a P.Atk buff
// and an attack-speed buff while a Watcher sees it. The death strip sends no
// per-buff UserInfo or ATK_SPD StatusUpdate; the player gets exactly one
// UserInfo and the Watcher one CharInfo of it, both before its Die.
func TestDeathStripSendsOneClosingUserInfo(t *testing.T) {
	t.Parallel()
	p := bootAppearancePair(t)
	for _, b := range statBuffs {
		p.land(t, b.meta, b.tmpl)
	}
	drainUntilQuiet(t, p.tc)
	drainUntilQuiet(t, p.wc)

	dier, ok := p.target.(interface {
		Die(attackable.Combatant) bool
	})
	if !ok {
		t.Fatalf("Target %T cannot die", p.target)
	}
	died := false
	p.onQueue(t, func() { died = dier.Die(nil) })
	if !died {
		t.Fatal("Die() = false for a living player")
	}
	if held := p.target.EffectList().All(); len(held) != 0 {
		t.Fatalf("Target still holds %d effects after death, want none", len(held))
	}

	own := readQuiet(t, p.tc)
	if infos := framesWithOpcode(own, serverpackets.OpcodeUserInfo); len(infos) != 1 {
		t.Fatalf("Target got %d UserInfo frames, want 1 (opcodes % x)", len(infos), opcodeList(own))
	}
	if su := speedStatusUpdatesOf(own, p.targetID); len(su) != 0 {
		t.Fatalf("Target got speed StatusUpdates %+v, want none", su)
	}
	if ui, die := opcodeIndex(own, serverpackets.OpcodeUserInfo), opcodeIndex(own, serverpackets.OpcodeDie); die < 0 || ui > die {
		t.Fatalf("Target UserInfo at %d, Die at %d, want UserInfo first (opcodes % x)", ui, die, opcodeList(own))
	}

	seen := readQuiet(t, p.wc)
	var infos []int
	for i, f := range seen {
		if f[0] != serverpackets.OpcodeCharInfo {
			continue
		}
		if id, _ := charInfoAbnormal(t, f); id == p.targetID {
			infos = append(infos, i)
		}
	}
	if len(infos) != 1 {
		t.Fatalf("Watcher got %d CharInfo frames of the Target, want 1 (opcodes % x)", len(infos), opcodeList(seen))
	}
	if su := speedStatusUpdatesOf(seen, p.targetID); len(su) != 0 {
		t.Fatalf("Watcher got speed StatusUpdates of the Target %+v, want none", su)
	}
	if die := opcodeIndex(seen, serverpackets.OpcodeDie); die < 0 || infos[0] > die {
		t.Fatalf("Watcher CharInfo at %d, Die at %d, want CharInfo first (opcodes % x)", infos[0], die, opcodeList(seen))
	}
}

// TestNPCStopAllSendsNoPerEffectStatus strips a watched monster's P.Atk and
// attack-speed buffs the way leaving an active region does
// (Npc.onInactiveRegion -> stopAllEffects). The Watcher gets no StatusUpdate
// and no NpcInfo for it.
func TestNPCStopAllSendsNoPerEffectStatus(t *testing.T) {
	t.Parallel()
	p := bootAppearancePair(t)
	x, y, z := p.target.Position()
	at := location.Location{X: x + 100, Y: y, Z: z}
	// The moving fixture wires the monster as its effect list's stat owner,
	// as the production spawner does.
	mob := p.srv.SpawnMovingHostileNPCAt(t, "Monster", at, at)
	onMob := func(fn func()) {
		t.Helper()
		done := make(chan struct{})
		if !mob.Queue().Post(func() { fn(); close(done) }) {
			t.Fatal("post to monster queue: queue closed")
		}
		<-done
	}
	for _, b := range statBuffs {
		e, err := effect.New(b.meta, b.tmpl)
		if err != nil {
			t.Fatalf("effect.New: %v", err)
		}
		e.Effector, e.Effected = mob, mob
		onMob(func() { mob.EffectList().Add(e) })
	}
	landed := readQuiet(t, p.wc)
	if su := speedStatusUpdatesOf(landed, mob.ObjectID()); len(su) != 1 {
		t.Fatalf("Watcher got %d speed StatusUpdates of the monster when its buffs landed, want 1 (opcodes % x)", len(su), opcodeList(landed))
	}

	onMob(func() { mob.EffectList().StopAll() })
	if held := mob.EffectList().All(); len(held) != 0 {
		t.Fatalf("monster still holds %d effects after StopAll, want none", len(held))
	}
	seen := readQuiet(t, p.wc)
	for _, op := range []byte{serverpackets.OpcodeStatusUpdate, serverpackets.OpcodeNPCInfo} {
		if got := framesWithOpcode(seen, op); len(got) != 0 {
			t.Fatalf("Watcher got %d frames of opcode %#x after the monster's stop-all, want none (opcodes % x)", len(got), op, opcodeList(seen))
		}
	}
}
