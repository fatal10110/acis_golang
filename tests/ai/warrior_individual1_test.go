package ai

import (
	"fmt"
	"maps"
	"slices"
	"sync"
	"testing"
	"time"

	"github.com/fatal10110/acis_golang/internal/commons/wire"
	gamesql "github.com/fatal10110/acis_golang/internal/gameserver/data/sql"
	"github.com/fatal10110/acis_golang/internal/gameserver/data/sql/sqltest"
	actorai "github.com/fatal10110/acis_golang/internal/gameserver/model/actor/ai"
	"github.com/fatal10110/acis_golang/internal/gameserver/model/actor/npc"
	modelskill "github.com/fatal10110/acis_golang/internal/gameserver/model/skill"
	"github.com/fatal10110/acis_golang/internal/gameserver/network/clientpackets"
	"github.com/fatal10110/acis_golang/internal/gameserver/network/serverpackets"
	"github.com/fatal10110/acis_golang/internal/gameserver/script"
	"github.com/fatal10110/acis_golang/internal/gameserver/script/ai/warrior"
	skillstate "github.com/fatal10110/acis_golang/internal/gameserver/skill"
	"github.com/fatal10110/acis_golang/internal/gameservertest"
)

// The first individual behavior batch's NPC ids the scenarios spawn (#186
// PR 1), each a fixture monster of level 1 unless a scenario says
// otherwise, and the id the scenarios' script-event listener is bound to.
const (
	premoID         = int32(29013) // CorePrivatePremo
	ramphoID        = int32(22199) // RamphoPrimeval
	royalGuardID    = int32(18135) // RoyalRushWarriorPhysicalSpecial2
	royalGuardTwoID = int32(18146) // RoyalRushWarriorPhysicalSpecial2
	immediateID     = int32(27178) // WarriorAggressiveImmediate
	debufferID      = int32(21379) // WarriorCast2DebuffsAggressive
	ind1ListenerID  = unboundID
)

// The skills the batch's fixture NPCs list by type or cast by id.
const (
	premoMuteID       = modelskill.ID(4215) // CorePrivatePremo's raid mute
	ind1Debuff1ID     = modelskill.ID(9401)
	ind1Debuff2ID     = modelskill.ID(9402)
	ramphoSelfBuffID  = modelskill.ID(9411)
	ramphoLongRangeID = modelskill.ID(9412)
	ramphoSpecial1ID  = modelskill.ID(9413)
	ramphoSpecial2ID  = modelskill.ID(9414)
	royalSpecial1ID   = modelskill.ID(9421)
	royalSpecial2ID   = modelskill.ID(9422)
	ind1AggroBuffID   = modelskill.ID(9431) // a self buff the character knows, adding hate
	royalWeaponID     = int32(6593)         // the weapon a royal guard draws
)

// ind1Batch are the batch's classes, in scripts.xml order.
var ind1Batch = []struct {
	path string
	ctor func() script.Script
}{
	{"script.ai.individual.Monster.WarriorBase.Warrior.CorePrivatePremo", warrior.CorePrivatePremo},
	{"script.ai.individual.Monster.WarriorBase.Warrior.RamphoPrimeval", warrior.RamphoPrimeval},
	{"script.ai.individual.Monster.WarriorBase.Warrior.RoyalRushWarriorPhysicalSpecial2", warrior.RoyalRushWarriorPhysicalSpecial2},
	{"script.ai.individual.Monster.WarriorBase.Warrior.WarriorAggressiveImmediate", warrior.AggressiveImmediate},
	{"script.ai.individual.Monster.WarriorBase.Warrior.WarriorCast2DebuffsAggressive", warrior.Cast2DebuffsAggressive},
}

// ind1Draws is the script random source of a scenario: the queued draws in
// order, then fallback, each held inside [0, n) of its call.
type ind1Draws struct {
	mu       sync.Mutex
	queue    []int
	fallback int
}

func (d *ind1Draws) rand(n int) int {
	d.mu.Lock()
	defer d.mu.Unlock()
	v := d.fallback
	if len(d.queue) > 0 {
		v, d.queue = d.queue[0], d.queue[1:]
	}
	return min(v, n-1)
}

// set queues draws, dropping any still queued, and sets the fallback.
func (d *ind1Draws) set(fallback int, draws ...int) {
	d.mu.Lock()
	defer d.mu.Unlock()
	d.queue, d.fallback = draws, fallback
}

// ind1Heard records the script events the listener NPC receives.
type ind1Heard struct {
	mu     sync.Mutex
	events []string
}

func (h *ind1Heard) take() []string {
	h.mu.Lock()
	defer h.mu.Unlock()
	out := h.events
	h.events = nil
	return out
}

// ind1World is warriorWorld with the batch registered after the Warrior
// chain, a director script to send script events through, a listener
// behavior on ind1ListenerID, and the script draws d hands out.
type ind1World struct {
	*warriorWorld
	draws *ind1Draws
	heard *ind1Heard
}

// bootIndividual1 boots the batch beside templates, with extra, every draw
// 0 until a scenario queues others.
func bootIndividual1(t *testing.T, extra raidBoot, templates ...*npc.Template) *ind1World {
	t.Helper()
	list, catalog := warriorScripts()
	for _, c := range ind1Batch {
		list = append(list, script.Listing{Path: c.path})
		catalog[c.path] = c.ctor
	}
	heard := &ind1Heard{}
	const listener, director = "test.Ind1Listener", "test.Ind1Director"
	catalog[listener] = func() script.Script {
		return script.Script{Behavior: true, NPCs: []int32{ind1ListenerID}, Hooks: script.Hooks{
			OnScriptEvent: func(_ *script.Script, e script.ScriptEvent) {
				heard.mu.Lock()
				defer heard.mu.Unlock()
				heard.events = append(heard.events, fmt.Sprintf("event=%d arg1=%d arg2=%d", e.EventID, e.Arg1, e.Arg2))
			},
		}}
	}
	catalog[director] = func() script.Script { return script.Script{} }
	list = append(list, script.Listing{Path: listener}, script.Listing{Path: director})
	kinds := map[int32]script.NPCKind{}
	for _, tmpl := range templates {
		kinds[int32(tmpl.ID)] = script.KindHostile
	}
	draws := &ind1Draws{}
	opts := append([]gameservertest.Option{
		gameservertest.WithScriptRand(draws.rand),
		gameservertest.WithNPCScripts(kinds, list, catalog),
	}, extra.opts...)
	w := bootRaid(t, raidBoot{opts: opts, ready: extra.ready}, templates...)
	return &ind1World{warriorWorld: w, draws: draws, heard: heard}
}

// sendEvent sends h the script event eventID with arg1 and arg2 from the
// director script, on the character's queue.
func (w *ind1World) sendEvent(t *testing.T, h *npc.Hostile, eventID, arg1, arg2 int32) {
	t.Helper()
	if !w.srv.RunScript(t, w.player.ObjectID(), "Ind1Director", func(s *script.Script, _ *script.Player) {
		s.SendScriptEvent(script.NPCOf(h), eventID, arg1, arg2)
	}) {
		t.Fatal("the script event panicked")
	}
	w.srv.Settle(t)
}

// ind1Slot reads slot of h's script memory.
func ind1Slot(h *npc.Hostile, slot script.IntSlot) int32 { return script.NPCOf(h).Int(slot) }

// ind1Casts returns h's queued cast desires by skill id.
func ind1Casts(h *npc.Hostile) map[modelskill.ID]actorai.Desire {
	out := map[modelskill.ID]actorai.Desire{}
	for _, d := range h.AI().Desires().Snapshot() {
		if d.Kind == actorai.IntentionCast {
			out[d.Skill.ID] = d
		}
	}
	return out
}

// wantCasts fails unless h's queued cast desires are exactly want: each
// skill id on the creature of the object id, weight as given.
func (w *ind1World) wantCasts(t *testing.T, when string, h *npc.Hostile, want map[modelskill.ID]ind1Cast) {
	t.Helper()
	got := ind1Casts(h)
	for id, c := range want {
		d, ok := got[id]
		if !ok || d.Weight != c.weight || d.FinalTarget == nil || d.FinalTarget.ObjectID() != c.target {
			t.Errorf("%s: cast desire of skill %d = %+v (%v), want weight %v on %d", when, id, d, ok, c.weight, c.target)
		}
	}
	for id, d := range got {
		if _, ok := want[id]; !ok {
			t.Errorf("%s: unexpected cast desire %+v", when, d)
		}
	}
}

// ind1Cast is an expected cast desire's target object id and weight.
type ind1Cast struct {
	target int32
	weight float64
}

// ind1Skill is a fixture NPC skill of id reaching castRange: a one-target
// magic attack, or a self buff when self is set.
func ind1Skill(id modelskill.ID, castRange int, self bool) modelskill.Definition {
	d := modelskill.Definition{
		ID: id, Level: 1, Activation: modelskill.ActivationActive, Target: modelskill.TargetOne,
		Offensive: true, Magic: true, SkillType: "MDAM", CastRange: castRange, HitTime: 1000,
		StaticHitTime: true, StaticReuse: true, Power: 1,
	}
	if self {
		d.Target, d.Offensive, d.Magic, d.SkillType, d.Power = modelskill.TargetSelf, false, false, "BUFF", 0
	}
	return d
}

// TestCorePrivatePremoCursesAndMutesOverLevelled ports CorePrivatePremo's
// attacked and see-spell hooks. Against the level 20 character, a level 11
// Premo (9 levels below, more than 8) wants to cast its raid curse on the
// character when hit and its raid mute when it sees the character cast,
// both with weight 1000000; a level 12 one (8 levels below) wants neither. Its attacked
// hook never calls its parent, so a hit adds no hate and no attack desire
// at either level.
func TestCorePrivatePremoCursesAndMutesOverLevelled(t *testing.T) {
	t.Parallel()
	for _, row := range []struct {
		level int
		curse bool
	}{{11, true}, {12, false}} {
		t.Run(fmt.Sprintf("level %d", row.level), func(t *testing.T) {
			t.Parallel()
			tmpl := warriorTemplate(premoID, nil)
			tmpl.Level, tmpl.CanMove = row.level, false
			w := bootIndividual1(t, raidSpellBoot(t), tmpl)
			premo := w.spawn(t, premoID, 400, 0)
			w.srv.InstallNPCCastSeam(t, premo, modelskill.NewTable([]modelskill.Definition{raidCurseDefinition(), ind1Skill(premoMuteID, 600, false)}))
			me := w.player.ObjectID()

			w.hit(t, premo, 10)
			want := map[modelskill.ID]ind1Cast{}
			if row.curse {
				want[raidCurseID] = ind1Cast{me, 1000000}
			}
			w.wantCasts(t, "after the hit", premo, want)
			if got := premo.AggroHate(w.player); got != 0 {
				t.Errorf("hate after the hit = %v, want none", got)
			}
			if d, ok := desire(premo, actorai.IntentionAttack); ok {
				t.Errorf("attack desire after the hit = %+v, want none", d)
			}

			w.castSelfBuff()
			if row.curse {
				w.srv.AdvanceUntil(t, "the raid mute desire", func() bool {
					_, ok := ind1Casts(premo)[premoMuteID]
					return ok
				})
				want[premoMuteID] = ind1Cast{me, 1000000}
			} else {
				w.srv.Advance(t, 2*time.Second)
			}
			w.wantCasts(t, "after the spell", premo, want)
		})
	}
}

// TestWarriorAggressiveImmediateAttacksOnFirstSight ports
// WarriorAggressiveImmediate's see-creature hook: with SetAggressiveTime -1,
// which makes a WarriorAggressive NPC wait at least 3 AI cycles before it
// turns on what it sees, the immediate NPC wants to attack the character it
// sees in its territory at once, with weight 200, and the WarriorAggressive
// beside it does not yet.
func TestWarriorAggressiveImmediateAttacksOnFirstSight(t *testing.T) {
	t.Parallel()
	params := npc.AIParams{"SetAggressiveTime": "-1"}
	w := bootIndividual1(t, raidBoot{}, warriorTemplate(immediateID, params), warriorTemplate(aggressiveID, params))
	immediate := w.spawn(t, immediateID, 150, 0)
	aggressive := w.spawn(t, aggressiveID, 0, 150)

	w.ticks(t, 1)
	if got := immediate.AggroHate(w.player); got != 200 {
		t.Fatalf("immediate hate of the character it sees = %v, want 200", got)
	}
	if got := aggressive.AggroHate(w.player); got != 0 {
		t.Fatalf("WarriorAggressive hate at life time %d = %v, want none yet", aggressive.LifeTime(), got)
	}
	w.ticks(t, 1)
	if got := immediate.AI().CurrentIntention(); got != actorai.IntentionAttack {
		t.Fatalf("immediate intention = %v, want attack", got)
	}
}

// TestWarriorAggressiveImmediateSparesWhatItSeesOutOfItsTerritory ports
// the territory gate of WarriorAggressiveImmediate's see-creature hook: an
// NPC standing out of its territory (its drift range around its spawn
// point) does not turn on the character it sees.
func TestWarriorAggressiveImmediateSparesWhatItSeesOutOfItsTerritory(t *testing.T) {
	t.Parallel()
	w := bootIndividual1(t, raidBoot{}, warriorTemplate(immediateID, nil))
	immediate := w.spawn(t, immediateID, 1500, 0)
	w.teleport(t, immediate, -1350, 0)
	if script.NPCOf(immediate).InMyTerritory() {
		t.Fatal("setup: the NPC is still in its territory")
	}
	w.ticks(t, 2)
	if got := immediate.AggroHate(w.player); got != 0 {
		t.Fatalf("hate of the character seen out of its territory = %v, want none", got)
	}
}

// debufferTemplate is the fixture WarriorCast2DebuffsAggressive NPC: two
// debuffs by type, in the clan the scenario's NPCs share.
func debufferTemplate() *npc.Template {
	tmpl := warriorTemplate(debufferID, nil)
	tmpl.SkillsByType = map[npc.SkillType]modelskill.Ref{
		npc.SkillTypeDebuff1: {ID: ind1Debuff1ID, Level: 1},
		npc.SkillTypeDebuff2: {ID: ind1Debuff2ID, Level: 1},
	}
	tmpl.Clans, tmpl.ClanRange = []string{"scorpions"}, 500
	return tmpl
}

// debufferSkills are the debuffer's two debuffs.
func debufferSkills() *modelskill.Table {
	return modelskill.NewTable([]modelskill.Definition{ind1Skill(ind1Debuff1ID, 600, false), ind1Skill(ind1Debuff2ID, 600, false)})
}

// TestWarriorCast2DebuffsAggressiveDebuffsWhatItFights ports
// WarriorCast2DebuffsAggressive's attacked and timer hooks, every draw 0
// and the character holding no effect of either debuff. The first hit
// arms the 5 s timer, but the character is not yet the most hated, so no
// debuff is wanted. Once the NPC fights, the timer wants debuff 1 alone on
// the most hated, weight 1000000, and re-arms. A second hit by the most
// hated wants both debuffs and marks the timer to skip its next debuff.
// Once the NPC no longer fights, the timer clears its state and stops, and
// the next hit arms it again.
func TestWarriorCast2DebuffsAggressiveDebuffsWhatItFights(t *testing.T) {
	t.Parallel()
	w := bootIndividual1(t, raidBoot{}, debufferTemplate())
	n := w.spawn(t, debufferID, 700, 0)
	w.srv.InstallNPCCastSeam(t, n, debufferSkills())
	me := w.player.ObjectID()

	w.hit(t, n, 10)
	w.wantCasts(t, "after the first hit", n, nil)
	if got := ind1Slot(n, script.SlotIntAI1); got != 1 {
		t.Fatalf("i_ai1 after the first hit = %d, want 1", got)
	}
	w.ticks(t, 1)
	if got := n.AI().CurrentIntention(); got != actorai.IntentionAttack {
		t.Fatalf("intention after the first hit = %v, want attack", got)
	}
	w.srv.Advance(t, 5*time.Second)
	w.wantCasts(t, "after the first timer", n, map[modelskill.ID]ind1Cast{ind1Debuff1ID: {me, 1000000}})

	w.onQueue(t, n, func() { n.AI().Desires().Clear() })
	w.hit(t, n, 10)
	w.wantCasts(t, "after the second hit", n, map[modelskill.ID]ind1Cast{ind1Debuff1ID: {me, 1000000}, ind1Debuff2ID: {me, 1000000}})
	if got := ind1Slot(n, script.SlotIntAI2); got != 1 {
		t.Fatalf("i_ai2 after a hit by the most hated = %d, want 1", got)
	}
	w.onQueue(t, n, func() { n.AI().Desires().Clear() })
	w.ticks(t, 1)
	w.srv.Advance(t, 5*time.Second)
	if got := ind1Slot(n, script.SlotIntAI2); got != 0 {
		t.Fatalf("i_ai2 after the timer it skipped = %d, want 0", got)
	}
	if casts := ind1Casts(n); len(casts) != 0 {
		t.Fatalf("cast desires after the skipped timer = %+v, want none", casts)
	}

	w.onQueue(t, n, func() { script.NPCOf(n).RemoveAllAttackDesire(); n.AI().Desires().Clear() })
	w.ticks(t, 1)
	if got := n.AI().CurrentIntention(); got == actorai.IntentionAttack || got == actorai.IntentionCast {
		t.Fatalf("intention after the desires were dropped = %v, want neither attack nor cast", got)
	}
	w.srv.Advance(t, 5*time.Second)
	if a1, a2 := ind1Slot(n, script.SlotIntAI1), ind1Slot(n, script.SlotIntAI2); a1 != 0 || a2 != 0 {
		t.Fatalf("i_ai1, i_ai2 after the timer out of a fight = %d, %d, want 0, 0", a1, a2)
	}
	w.srv.Advance(t, 10*time.Second)
	if casts := ind1Casts(n); len(casts) != 0 {
		t.Fatalf("cast desires once the timer stopped = %+v, want none", casts)
	}
	w.hit(t, n, 10)
	if got := ind1Slot(n, script.SlotIntAI1); got != 1 {
		t.Fatalf("i_ai1 after the next hit = %d, want 1", got)
	}
}

// TestWarriorCast2DebuffsAggressiveDebuffsOnSightAndForItsClan ports
// WarriorCast2DebuffsAggressive's see-creature and clan-attacked hooks,
// every draw 0. Settled (life time past 7) and wandering, it wants both
// debuffs on the character it sees, then attacks it; young, it only
// attacks. A settled clan member wants both debuffs on the attacker of
// another member.
func TestWarriorCast2DebuffsAggressiveDebuffsOnSightAndForItsClan(t *testing.T) {
	t.Parallel()
	t.Run("see", func(t *testing.T) {
		t.Parallel()
		w := bootIndividual1(t, raidBoot{}, debufferTemplate())
		n := w.spawn(t, debufferID, 1500, 0)
		w.srv.InstallNPCCastSeam(t, n, debufferSkills())
		w.ticks(t, 9)
		if n.LifeTime() <= 7 || n.AI().CurrentIntention() != actorai.IntentionWander {
			t.Fatalf("setup: life time %d, intention %v, want past 7 and wander", n.LifeTime(), n.AI().CurrentIntention())
		}
		if got := n.AggroHate(w.player); got != 0 {
			t.Fatalf("setup: the debuffer saw the character from afar: hate %v", got)
		}
		obj, ok := w.srv.State.Player(w.player.ObjectID())
		if !ok {
			t.Fatal("the character is not in the world")
		}
		x, y, z := n.Position()
		if err := w.srv.State.Move(obj, x-100, y, z); err != nil {
			t.Fatalf("approach: %v", err)
		}
		w.ticks(t, 1)
		me := w.player.ObjectID()
		w.wantCasts(t, "after it saw the character", n, map[modelskill.ID]ind1Cast{ind1Debuff1ID: {me, 1000000}, ind1Debuff2ID: {me, 1000000}})
		if got := n.AggroHate(w.player); got != 200 {
			t.Fatalf("hate of the character it sees = %v, want 200", got)
		}
	})
	t.Run("young", func(t *testing.T) {
		t.Parallel()
		w := bootIndividual1(t, raidBoot{}, debufferTemplate())
		n := w.spawn(t, debufferID, 150, 0)
		w.srv.InstallNPCCastSeam(t, n, debufferSkills())
		w.ticks(t, 1)
		if n.LifeTime() > 7 {
			t.Fatalf("setup: life time %d, want at most 7", n.LifeTime())
		}
		if got := n.AggroHate(w.player); got != 200 {
			t.Fatalf("hate of the character it sees = %v, want 200", got)
		}
		w.wantCasts(t, "after a young debuffer saw the character", n, nil)
	})
	t.Run("clan", func(t *testing.T) {
		t.Parallel()
		w := bootIndividual1(t, raidBoot{}, debufferTemplate())
		victim := w.spawn(t, debufferID, 1500, 0)
		member := w.spawn(t, debufferID, 1500, 100)
		w.srv.InstallNPCCastSeam(t, member, debufferSkills())
		w.ticks(t, 8)
		if member.LifeTime() <= 7 {
			t.Fatalf("member life time = %d, want past 7", member.LifeTime())
		}
		w.hit(t, victim, 10)
		me := w.player.ObjectID()
		w.wantCasts(t, "member after the hit", member, map[modelskill.ID]ind1Cast{ind1Debuff1ID: {me, 1000000}, ind1Debuff2ID: {me, 1000000}})
	})
}

// TestWarriorCast2DebuffsAggressiveSkipsAHeldDebuff ports the abnormal
// level gate of WarriorCast2DebuffsAggressive's debuffs, every draw 0: hit
// by its most hated while the character holds an effect of debuff 1's
// stack, it wants debuff 2 alone.
func TestWarriorCast2DebuffsAggressiveSkipsAHeldDebuff(t *testing.T) {
	t.Parallel()
	held := ind1Skill(ind1Debuff1ID, 600, false)
	held.Effects = []modelskill.EffectTemplate{{Name: "Buff", Time: 60, Icon: true, StackType: "ind1_debuff1", StackOrder: 1}}
	w := bootIndividual1(t, raidBoot{opts: []gameservertest.Option{gameservertest.WithScriptSkills(held)}}, debufferTemplate())
	n := w.spawn(t, debufferID, 700, 0)
	w.srv.InstallNPCCastSeam(t, n, debufferSkills())
	me := w.player.ObjectID()
	if !w.srv.RunScript(t, me, "Ind1Director", func(s *script.Script, p *script.Player) {
		s.ApplyEffects(modelskill.Ref{ID: ind1Debuff1ID, Level: 1}, script.NPCOf(n), p)
	}) {
		t.Fatal("the effect landing panicked")
	}
	w.srv.AdvanceUntil(t, "debuff 1 landed on the character", func() bool {
		var level int32
		w.srv.RunScript(t, me, "Ind1Director", func(s *script.Script, p *script.Player) {
			level = s.AbnormalLevel(p, modelskill.Ref{ID: ind1Debuff1ID, Level: 1})
		})
		return level > 0
	})
	w.hit(t, n, 10)
	w.hit(t, n, 10)
	w.wantCasts(t, "after a hit by the most hated", n, map[modelskill.ID]ind1Cast{ind1Debuff2ID: {me, 1000000}})
}

// ramphoTemplate is the fixture RamphoPrimeval NPC of AI parameters ai,
// listing its four skills by type.
func ramphoTemplate(ai npc.AIParams) *npc.Template {
	tmpl := warriorTemplate(ramphoID, ai)
	tmpl.SkillsByType = map[npc.SkillType]modelskill.Ref{
		npc.SkillTypeSelfBuff1:         {ID: ramphoSelfBuffID, Level: 1},
		npc.SkillTypeLongRangeDDMagic1: {ID: ramphoLongRangeID, Level: 1},
		npc.SkillTypePhysicalSpecial1:  {ID: ramphoSpecial1ID, Level: 1},
		npc.SkillTypePhysicalSpecial2:  {ID: ramphoSpecial2ID, Level: 1},
	}
	return tmpl
}

// ramphoSkills are the fixture RamphoPrimeval's skills.
func ramphoSkills() *modelskill.Table {
	return modelskill.NewTable([]modelskill.Definition{
		ind1Skill(ramphoSelfBuffID, 0, true), ind1Skill(ramphoLongRangeID, 600, false),
		ind1Skill(ramphoSpecial1ID, 600, false), ind1Skill(ramphoSpecial2ID, 600, false),
	})
}

// TestRamphoPrimevalStrikesByItsChances ports RamphoPrimeval's created and
// attacked hooks. It is created with a chance multiplier of 1 and its last
// fight tick set. Hit from closer than 100 with draw 0, it wants its long
// range DD magic on the attacker (draw 0 at most 15 x 1). Before the hit
// that makes the attacker its top desire target there is none, so a losing
// first draw wants nothing; once there is one, a losing first draw wants
// the DD magic and both physical specials on it, each draw 0 at most its
// chance. Below HpChkRate5 it wants its self buff, weight 10000000, once,
// and below HpChkRate4 its chances take ProbMultiplier1. Hit from farther
// than 100 it wants nothing.
func TestRamphoPrimevalStrikesByItsChances(t *testing.T) {
	t.Parallel()
	t.Run("near", func(t *testing.T) {
		t.Parallel()
		w := bootIndividual1(t, raidBoot{}, ramphoTemplate(nil))
		n := w.spawn(t, ramphoID, 50, 0)
		w.srv.InstallNPCCastSeam(t, n, ramphoSkills())
		if a3, a4 := ind1Slot(n, script.SlotIntAI3), ind1Slot(n, script.SlotIntAI4); a3 != 1 || a4 < 0 {
			t.Fatalf("created i_ai3, i_ai4 = %d, %d, want 1 and the current tick", a3, a4)
		}
		me := w.player.ObjectID()
		w.draws.set(0, 99)
		w.hit(t, n, 10)
		w.wantCasts(t, "after a losing first hit", n, nil)
		w.draws.set(0, 99, 0, 0, 0)
		w.hit(t, n, 10)
		w.wantCasts(t, "after a losing second hit", n, map[modelskill.ID]ind1Cast{
			ramphoLongRangeID: {me, 10000000}, ramphoSpecial1ID: {me, 10000000}, ramphoSpecial2ID: {me, 10000000},
		})
		w.onQueue(t, n, func() { n.AI().Desires().Clear() })
		w.draws.set(0)
		w.hit(t, n, 10)
		w.wantCasts(t, "after a winning hit", n, map[modelskill.ID]ind1Cast{ramphoLongRangeID: {me, 10000000}})
		if got := ind1Slot(n, script.SlotIntAI3); got != 1 {
			t.Fatalf("i_ai3 at full hit points = %d, want 1", got)
		}
	})
	t.Run("hurt", func(t *testing.T) {
		t.Parallel()
		w := bootIndividual1(t, raidBoot{}, ramphoTemplate(npc.AIParams{"HpChkRate4": "100", "HpChkRate5": "100", "ProbMultiplier1": "3"}))
		n := w.spawn(t, ramphoID, 300, 0)
		w.srv.InstallNPCCastSeam(t, n, ramphoSkills())
		w.hit(t, n, 10)
		w.wantCasts(t, "after the first hit", n, map[modelskill.ID]ind1Cast{ramphoSelfBuffID: {n.ObjectID(), 10000000}})
		if a2, a3 := ind1Slot(n, script.SlotIntAI2), ind1Slot(n, script.SlotIntAI3); a2 != 1 || a3 != 3 {
			t.Fatalf("i_ai2, i_ai3 after the hit = %d, %d, want 1, 3", a2, a3)
		}
		w.onQueue(t, n, func() { n.AI().Desires().Clear() })
		w.hit(t, n, 10)
		w.wantCasts(t, "after the second hit", n, nil)
	})
}

// TestRamphoPrimevalWalksHomeAndCallsTheLair ports RamphoPrimeval's
// no-desire and script-event hooks. With nothing to do out of combat it
// wants to walk back to its spawn point, weight 30. Event 11049 makes it
// report: once more than 600 seconds have passed since the tick of its last
// fight or sighting (i_ai4), its next no-desire hook broadcasts event 11051
// to the NPCs within 8000. The harness's uptime runs on the wall clock, so
// the scenario moves that tick 601 seconds back rather than waiting.
func TestRamphoPrimevalWalksHomeAndCallsTheLair(t *testing.T) {
	t.Parallel()
	w := bootIndividual1(t, raidBoot{}, ramphoTemplate(nil), warriorTemplate(ind1ListenerID, nil))
	n := w.spawn(t, ramphoID, 1500, 0)
	w.spawn(t, ind1ListenerID, 1500, 3000)
	spawnX, spawnY, _ := n.Position()
	w.teleport(t, n, 200, 0)
	w.ticks(t, 2)
	if d, ok := desire(n, actorai.IntentionMoveTo); !ok || d.Weight != 30 || d.Location.X != spawnX || d.Location.Y != spawnY {
		t.Fatalf("move desire = %+v (%v), want weight 30 to %d,%d", d, ok, spawnX, spawnY)
	}
	if got := w.heard.take(); len(got) != 0 {
		t.Fatalf("the listener heard %q before event 11049", got)
	}

	w.sendEvent(t, n, 11049, 0, 0)
	if got := ind1Slot(n, npc.IntQuest0); got != 1 {
		t.Fatalf("i_quest0 after event 11049 = %d, want 1", got)
	}
	w.ticks(t, 6)
	if got := w.heard.take(); len(got) != 0 {
		t.Fatalf("the listener heard %q within 600 seconds", got)
	}
	w.onQueue(t, n, func() {
		script.NPCOf(n).SetInt(script.SlotIntAI4, ind1Slot(n, script.SlotIntAI4)-601)
		n.AI().Desires().Clear()
	})
	w.ticks(t, 2)
	if got := w.heard.take(); !slices.Equal(got, []string{"event=11051 arg1=0 arg2=0"}) {
		t.Fatalf("the listener heard %q after 600 seconds, want event 11051 once", got)
	}
}

// TestRamphoPrimevalAnswersAShoutedTarget ports RamphoPrimeval's script
// event 10016: naming a player with BroadCastReception 1, it drops its
// attack desires and runs its own attacked hook as a hit of 1000 by that
// player, which adds the Warrior hate, 1000 / (level + 7) x 100. Its parent
// then handles the same event: on a draw under 50 it drops its attack
// desires again and wants the player with weight 100. Without
// BroadCastReception only the parent reacts.
func TestRamphoPrimevalAnswersAShoutedTarget(t *testing.T) {
	t.Parallel()
	for _, row := range []struct {
		name      string
		reception string
		draw      int
		wantHate  float64
	}{
		{"reception, parent loses", "1", 99, 12500},
		{"reception, parent wins", "1", 0, 100},
		{"no reception, parent loses", "0", 99, 0},
		{"no reception, parent wins", "0", 0, 100},
	} {
		t.Run(row.name, func(t *testing.T) {
			t.Parallel()
			w := bootIndividual1(t, raidBoot{}, ramphoTemplate(npc.AIParams{"BroadCastReception": row.reception}))
			n := w.spawn(t, ramphoID, 1500, 0)
			w.srv.InstallNPCCastSeam(t, n, ramphoSkills())
			w.draws.set(row.draw)
			w.sendEvent(t, n, 10016, w.player.ObjectID(), 0)
			if got := n.AggroHate(w.player); got != row.wantHate {
				t.Fatalf("hate after event 10016 = %v, want %v", got, row.wantHate)
			}
		})
	}
}

// TestRamphoPrimevalSeesSpellsButNotPlayers ports RamphoPrimeval's
// see-spell and see-creature hooks. Seeing the character cast a skill that
// adds hate on a player, on a draw at most ProbCond4 (30), it wants its
// second physical special on the caster, and its first too on a draw of 1
// of 2. It never turns on a player or summon it sees: it neither attacks
// the character beside it nor casts at it, but notes the tick.
func TestRamphoPrimevalSeesSpellsButNotPlayers(t *testing.T) {
	t.Parallel()
	boot := ind1AggroSpellBoot(t)
	w := bootIndividual1(t, boot, ramphoTemplate(nil))
	n := w.spawn(t, ramphoID, 150, 0)
	w.srv.InstallNPCCastSeam(t, n, ramphoSkills())
	w.ticks(t, 3)
	if got := n.AggroHate(w.player); got != 0 {
		t.Fatalf("hate of the character it sees = %v, want none", got)
	}
	w.wantCasts(t, "after it saw the character", n, nil)

	me := w.player.ObjectID()
	w.draws.set(0, 0, 1)
	w.castAggroBuff()
	w.srv.AdvanceUntil(t, "the physical special desires", func() bool { return len(ind1Casts(n)) == 2 })
	w.wantCasts(t, "after the spell", n, map[modelskill.ID]ind1Cast{ramphoSpecial1ID: {me, 10000000}, ramphoSpecial2ID: {me, 10000000}})
}

// ind1AggroSpellBoot boots the character knowing a self buff that adds
// hate, in the scripts' skill table too.
func ind1AggroSpellBoot(t *testing.T) raidBoot {
	t.Helper()
	db := sqltest.SharedDB(t)
	def := modelskill.Definition{
		ID: ind1AggroBuffID, Level: 1, Activation: modelskill.ActivationActive, Target: modelskill.TargetSelf,
		SkillType: "BUFF", HitTime: 500, StaticHitTime: true, StaticReuse: true, AggroPoints: 10,
	}
	table := modelskill.NewTable([]modelskill.Definition{{ID: 248, Level: 3}, {ID: 294, Level: 1}, def})
	return raidBoot{
		opts: []gameservertest.Option{
			gameservertest.WithSkills(skillstate.NewPersistence(gamesql.NewSkillSaveStore(db), table, gamesql.NewCharacterSkillStore(db))),
			gameservertest.WithScriptSkills(def),
		},
		ready: func(srv *gameservertest.Server, objID int32) {
			if err := srv.KnownSkills.SetKnownSkill(t.Context(), objID, 0, int(ind1AggroBuffID), 1); err != nil {
				t.Fatalf("seed the hate buff: %v", err)
			}
		},
	}
}

// castAggroBuff has the character cast its self buff that adds hate.
func (w *ind1World) castAggroBuff() {
	p := wire.NewPacketWriter(clientpackets.OpcodeRequestMagicSkillUse)
	p.WriteInt32(int32(ind1AggroBuffID))
	p.WriteInt32(0)
	p.WriteUint8(0)
	w.srv.Client.Send(p.Bytes())
}

// royalTemplate is a fixture RoyalRushWarriorPhysicalSpecial2 NPC of id,
// with ai, ten soulshots (AI parameter SoulShot) and its two physical
// specials by type, in the clan the scenario's NPCs share.
func royalTemplate(id int32, ai npc.AIParams) *npc.Template {
	params := npc.AIParams{"SoulShot": "10"}
	maps.Copy(params, ai)
	tmpl := warriorTemplate(id, params)
	tmpl.SkillsByType = map[npc.SkillType]modelskill.Ref{
		npc.SkillTypePhysicalSpecial1: {ID: royalSpecial1ID, Level: 1},
		npc.SkillTypePhysicalSpecial2: {ID: royalSpecial2ID, Level: 1},
	}
	tmpl.Clans, tmpl.ClanRange = []string{"sepulcher"}, 600
	return tmpl
}

// royalSkills are the royal guards' skills.
func royalSkills() *modelskill.Table {
	return modelskill.NewTable([]modelskill.Definition{ind1Skill(royalSpecial1ID, 600, false), ind1Skill(royalSpecial2ID, 600, false)})
}

// TestRoyalRushWarriorDrawsItsWeapon ports RoyalRushWarriorPhysicalSpecial2's
// created and attacked hooks, every draw 0. Its created hook does not call
// its parent, so the Warrior's created shout is not heard. Hit from closer
// than 100 by its top desire target it wants its second physical special,
// weight 1000000. Once a hit finds it below half of its hit points it
// draws its WeaponID weapon glowing at +15 (an NpcInfo to the character)
// and from then on charges a soulshot (MagicSkillUse 2154) rather than
// striking.
func TestRoyalRushWarriorDrawsItsWeapon(t *testing.T) {
	t.Parallel()
	ai := npc.AIParams{"WeaponID": fmt.Sprint(royalWeaponID), "ShoutMsg1": "1000286"}
	w := bootIndividual1(t, raidBoot{}, royalTemplate(royalGuardID, ai), warriorTemplate(warriorID, npc.AIParams{"ShoutMsg1": "1000286"}))
	plain := w.spawnFrames(t, warriorID, 1500, 300)
	if got := says(plain.frames, plain.h.ObjectID()); len(got) != 1 {
		t.Fatalf("setup: a plain Warrior's created shouts = %q, want one", got)
	}
	g := w.spawnFrames(t, royalGuardID, 50, 0)
	if got := says(g.frames, g.h.ObjectID()); len(got) != 0 {
		t.Fatalf("royal guard created shouts = %q, want none", got)
	}
	n := g.h
	if got := ind1Slot(n, script.SlotIntAI0); got != 0 {
		t.Fatalf("created i_ai0 = %d, want 0", got)
	}
	w.srv.InstallNPCCastSeam(t, n, royalSkills())
	me := w.player.ObjectID()

	w.hit(t, n, 10)
	w.wantCasts(t, "after the first hit", n, nil)
	w.hit(t, n, 10)
	w.wantCasts(t, "after a hit by its top desire target", n, map[modelskill.ID]ind1Cast{royalSpecial2ID: {me, 1000000}})
	w.onQueue(t, n, func() { n.AI().Desires().Clear() })
	w.srv.ReadQueued(t, w.srv.Client)

	w.hit(t, n, int(n.HP())-n.MaxHP()*45/100)
	if s := n.NPCInfoSnapshot(); s.RightHand == int(royalWeaponID) {
		t.Fatal("drew its weapon above half of its hit points")
	}
	w.onQueue(t, n, func() { n.AI().Desires().Clear() })
	w.srv.ReadQueued(t, w.srv.Client)
	w.hit(t, n, 10)
	frames := w.srv.ReadQueued(t, w.srv.Client)
	if s := n.NPCInfoSnapshot(); s.RightHand != int(royalWeaponID) || s.EnchantEffect != 15 {
		t.Fatalf("hands after a hit below half = %d glow %d, want %d glowing at 15", s.RightHand, s.EnchantEffect, royalWeaponID)
	}
	if got := ind1Slot(n, script.SlotIntAI0); got != 1 {
		t.Fatalf("i_ai0 after it drew its weapon = %d, want 1", got)
	}
	if infos := framesOf(frames, serverpackets.OpcodeNPCInfo, n.ObjectID()); len(infos) == 0 {
		t.Fatal("the character was sent no NpcInfo of the drawn weapon")
	}
	if got := castsOf(frames, n, 2154); len(got) != 1 {
		t.Fatalf("soulshot MagicSkillUse frames = %d, want 1", len(got))
	}
	w.wantCasts(t, "after it drew its weapon", n, nil)
}

// TestRoyalRushWarriorAssistsItsClan ports RoyalRushWarriorPhysicalSpecial2's
// clan-attacked hook, every draw 0: a settled clan member that is not
// attacking wants its first physical special on an attacker farther than
// 100 from it, and its second on one closer than 100, weight 1000000. A
// member that drew its weapon charges a soulshot (MagicSkillUse 2154)
// instead of the close strike, and a member already attacking does
// nothing.
func TestRoyalRushWarriorAssistsItsClan(t *testing.T) {
	t.Parallel()
	w := bootIndividual1(t, raidBoot{}, royalTemplate(royalGuardID, nil), royalTemplate(royalGuardTwoID, nil))
	victim := w.spawn(t, royalGuardID, 0, 400)
	far := w.spawn(t, royalGuardTwoID, 300, 0)
	near := w.spawn(t, royalGuardTwoID, 50, 0)
	drawn := w.spawn(t, royalGuardTwoID, 0, -60)
	attacking := w.spawn(t, royalGuardTwoID, -60, 0)
	members := []*npc.Hostile{far, near, drawn, attacking}
	for _, h := range members {
		w.srv.InstallNPCCastSeam(t, h, royalSkills())
	}
	// The hit lands before any member has settled, so none calls for help.
	w.hit(t, attacking, 10)
	w.onQueue(t, drawn, func() { script.NPCOf(drawn).SetInt(script.SlotIntAI0, 1) })
	w.ticks(t, 8)
	for _, h := range members {
		if h.LifeTime() <= 7 {
			t.Fatalf("member life time = %d, want past 7", h.LifeTime())
		}
		if casts := ind1Casts(h); len(casts) != 0 {
			t.Fatalf("setup: member %d wants %+v before the hit", h.ObjectID(), casts)
		}
	}
	if got := attacking.AI().CurrentIntention(); got != actorai.IntentionAttack {
		t.Fatalf("setup: the hit member's intention = %v, want attack", got)
	}
	w.srv.ReadQueued(t, w.srv.Client)
	w.hit(t, victim, 10)
	frames := w.srv.ReadQueued(t, w.srv.Client)
	me := w.player.ObjectID()
	w.wantCasts(t, "far member", far, map[modelskill.ID]ind1Cast{royalSpecial1ID: {me, 1000000}})
	w.wantCasts(t, "near member", near, map[modelskill.ID]ind1Cast{royalSpecial2ID: {me, 1000000}})
	w.wantCasts(t, "member with its weapon drawn", drawn, nil)
	w.wantCasts(t, "attacking member", attacking, nil)
	if got := castsOf(frames, drawn, 2154); len(got) != 1 {
		t.Fatalf("soulshot MagicSkillUse frames of the member with its weapon drawn = %d, want 1", len(got))
	}
	for _, h := range []*npc.Hostile{far, near, attacking} {
		if got := castsOf(frames, h, 2154); len(got) != 0 {
			t.Fatalf("member %d charged a soulshot", h.ObjectID())
		}
	}
}

// TestRoyalRushWarriorStrikesFromRange ports the ranged branch of
// RoyalRushWarriorPhysicalSpecial2's attacked hook, every draw 0: hit by
// its top desire target from farther than 100, it wants its second
// physical special on the attacker, weight 1000000.
func TestRoyalRushWarriorStrikesFromRange(t *testing.T) {
	t.Parallel()
	w := bootIndividual1(t, raidBoot{}, royalTemplate(royalGuardID, nil))
	n := w.spawn(t, royalGuardID, 300, 0)
	w.srv.InstallNPCCastSeam(t, n, royalSkills())
	w.hit(t, n, 10)
	w.wantCasts(t, "after the first hit", n, nil)
	w.hit(t, n, 10)
	w.wantCasts(t, "after a hit by its top desire target", n, map[modelskill.ID]ind1Cast{royalSpecial2ID: {w.player.ObjectID(), 1000000}})
}

// TestRoyalRushWarriorAttacksTheNamedCreature ports
// RoyalRushWarriorPhysicalSpecial2's script event 1234: on a draw under 80
// it wants the creature the event names with weight 300; otherwise it
// drops its attack desires first and wants it with weight 1000. Another
// event does nothing.
func TestRoyalRushWarriorAttacksTheNamedCreature(t *testing.T) {
	t.Parallel()
	w := bootIndividual1(t, raidBoot{}, royalTemplate(royalGuardID, nil))
	n := w.spawn(t, royalGuardID, 1500, 0)
	me := w.player.ObjectID()
	w.sendEvent(t, n, 1235, me, 0)
	if got := n.AggroHate(w.player); got != 0 {
		t.Fatalf("hate after event 1235 = %v, want none", got)
	}
	w.sendEvent(t, n, 1234, me, 0)
	if got := n.AggroHate(w.player); got != 300 {
		t.Fatalf("hate after event 1234, draw 0 = %v, want 300", got)
	}
	w.draws.set(80)
	w.sendEvent(t, n, 1234, me, 0)
	if got := n.AggroHate(w.player); got != 1000 {
		t.Fatalf("hate after event 1234, draw 80 = %v, want 1000", got)
	}
}

// ind1Spawn is an NPC spawned by spawnFrames and the frames its spawn sent.
type ind1Spawn struct {
	h      *npc.Hostile
	frames [][]byte
}

// spawnFrames is spawn, keeping the frames the spawn sent.
func (w *ind1World) spawnFrames(t *testing.T, id int32, dx, dy int) ind1Spawn {
	t.Helper()
	n, err := w.srv.NpcSpawns.AddSpawn(id, w.at.X+dx, w.at.Y+dy, w.at.Z, 0, false, 0)
	if err != nil {
		t.Fatalf("spawn %d: %v", id, err)
	}
	w.srv.Settle(t)
	return ind1Spawn{h: n.(*npc.Hostile), frames: w.srv.ReadQueued(t, w.srv.Client)}
}
