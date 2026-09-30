package skills

import (
	"testing"
	"time"

	"github.com/fatal10110/acis_golang/internal/gameserver/model/actor/attackable"
	"github.com/fatal10110/acis_golang/internal/gameserver/model/actor/npc"
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
	mob, onMob := spawnBuffedMonster(t, p)

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

// Reference: Creature.doDie ends every effect of a dying NPC that does not
// last through death with exit(true) (Creature.java:495-510,
// EffectList.java:322-333), so a DoT stops ticking on the corpse; the
// removal sends no per-effect StatusUpdate or NpcInfo. Npc.deleteMe stops
// whatever is left (Npc.java:621-636), as the respawn reset does
// (ASpawn.java:429, MinionSpawn.java:221).

// TestNPCDeathStripsEffectsWithoutPerEffectStatus kills a watched monster
// holding a P.Atk buff, an attack-speed buff, a DoT and a buff that lasts
// through death. Right after Die only the last one is held and the Watcher
// got no speed StatusUpdate or NpcInfo of it; the DoT never ticks on the
// corpse; decay stops the surviving buff.
func TestNPCDeathStripsEffectsWithoutPerEffectStatus(t *testing.T) {
	t.Parallel()
	p := bootAppearancePair(t)
	mob, onMob := spawnBuffedMonster(t, p)
	dot, err := effect.New(effect.Skill{ID: 1164, Level: 1, Debuff: true}, modelskill.EffectTemplate{
		Name: "DamOverTime", Value: 50, Count: 10, Time: 1,
	})
	if err != nil {
		t.Fatalf("effect.New(DamOverTime): %v", err)
	}
	lasting, err := effect.New(effect.Skill{ID: 1323, Level: 1, StayAfterDeath: true}, modelskill.EffectTemplate{
		Name: "Buff", Time: 60, Count: 1, Icon: true, StackType: "stay_after_death", StackOrder: 1,
	})
	if err != nil {
		t.Fatalf("effect.New(lasting Buff): %v", err)
	}
	for _, e := range []*effect.Effect{dot, lasting} {
		e.Effector, e.Effected = mob, mob
		onMob(func() { mob.EffectList().Add(e) })
	}
	readQuiet(t, p.wc)

	died := false
	onMob(func() { died = mob.Die(nil, nil) })
	if !died {
		t.Fatal("Die() = false for a living monster")
	}
	held := mob.EffectList().All()
	if len(held) != 1 || held[0] != lasting {
		t.Fatalf("monster holds %v after death, want only the effect that lasts through death", held)
	}
	// The death's own HP StatusUpdates carry no speed attribute.
	seen := readQuiet(t, p.wc)
	if su := speedStatusUpdates(seen); len(su) != 0 {
		t.Fatalf("Watcher got speed StatusUpdates %+v after the monster's death strip, want none", su)
	}
	if got := framesWithOpcode(seen, serverpackets.OpcodeNPCInfo); len(got) != 0 {
		t.Fatalf("Watcher got %d NpcInfo frames after the monster's death strip, want none (opcodes % x)", len(got), opcodeList(seen))
	}
	if opcodeIndex(seen, serverpackets.OpcodeDie) < 0 {
		t.Fatalf("Watcher got no Die of the monster (opcodes % x)", opcodeList(seen))
	}

	left := dot.Remaining()
	for range 3 {
		p.srv.Advance(t, 1100*time.Millisecond)
		p.srv.TickEffects()
	}
	// Stopping an effect zeroes its schedule; a DoT still held would count
	// its ticks down on the corpse instead.
	if got := dot.Remaining(); left != 0 || got != 0 {
		t.Fatalf("DoT remaining ticks = %d after death, %d after three sweeps, want 0: stopped at death", left, got)
	}
	if after := readQuiet(t, p.wc); len(framesWithOpcode(after, serverpackets.OpcodeStatusUpdate)) != 0 {
		t.Fatalf("Watcher got StatusUpdates of the corpse, want none (opcodes % x)", opcodeList(after))
	}

	decayed := false
	onMob(func() { decayed = mob.Decay(p.srv.State, nil) })
	if !decayed {
		t.Fatal("Decay() = false for a fresh corpse")
	}
	if held := mob.EffectList().All(); len(held) != 0 {
		t.Fatalf("monster still holds %d effects after decay, want none", len(held))
	}
}

// spawnBuffedMonster spawns a monster next to the Target that the Watcher
// sees, lands the P.Atk and attack-speed buffs on it, and returns it with a
// runner for its queue.
func spawnBuffedMonster(t *testing.T, p *appearancePair) (*npc.Hostile, func(func())) {
	t.Helper()
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
	return mob, onMob
}
