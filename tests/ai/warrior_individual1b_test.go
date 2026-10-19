package ai

import (
	"encoding/binary"
	"fmt"
	"slices"
	"testing"
	"time"

	actorai "github.com/fatal10110/acis_golang/internal/gameserver/model/actor/ai"
	"github.com/fatal10110/acis_golang/internal/gameserver/model/actor/npc"
	"github.com/fatal10110/acis_golang/internal/gameserver/model/actor/player"
	"github.com/fatal10110/acis_golang/internal/gameserver/model/item"
	modelskill "github.com/fatal10110/acis_golang/internal/gameserver/model/skill"
	"github.com/fatal10110/acis_golang/internal/gameserver/network/serverpackets"
	"github.com/fatal10110/acis_golang/internal/gameserver/script"
	"github.com/fatal10110/acis_golang/internal/gameserver/script/ai/warrior"
	"github.com/fatal10110/acis_golang/internal/gameservertest"
)

// The NPC ids the second PR of the first individual behavior batch (#186
// PR 2) spawns, each a fixture monster of level 1.
const (
	hallCaptainID   = int32(18329) // WarriorCast2SkillsApproachAggressive, counting captains
	hallGuardID     = int32(18331) // WarriorCast2SkillsApproachAggressive, counting guards
	hallPatrolID    = int32(18332) // WarriorCast2SkillsApproachAggressive, counting nothing
	selfRangeID     = int32(21310) // WarriorCastSelfRangeDDMagicAggressive
	selfRangeBuffID = int32(21390) // WarriorCastSelfRangeDDMagicBuffAggressive
	necroID         = int32(21574) // WarriorCorpseNecroLongAggressive
	disguiseID      = int32(21266) // WarriorDisguise
	fleeID          = int32(20002) // WarriorFlee
)

// The skills the batch's fixture NPCs list by type, and the root whose
// stack the self range caster checks on itself.
const (
	ind1bSelfBuffID    = modelskill.ID(9441) // SELF_BUFF
	ind1bSpecialID     = modelskill.ID(9442) // PHYSICAL_SPECIAL
	ind1bLongDDID      = modelskill.ID(9443) // W_LONG_RANGE_DD_MAGIC
	ind1bSelfRangeDDID = modelskill.ID(9444) // SELF_RANGE_DD_MAGIC
	ind1bBuffID        = modelskill.ID(9445) // BUFF
	ind1bDD1ID         = modelskill.ID(9446) // DD_MAGIC1
	ind1bDD2ID         = modelskill.ID(9447) // DD_MAGIC2
	dryadRootID        = modelskill.ID(1201)
	dewdropID          = int32(8556) // Dewdrop of Destruction
)

// ind1bBatch are the second PR's classes, in scripts.xml order.
var ind1bBatch = []struct {
	path string
	ctor func() script.Script
}{
	{"script.ai.individual.Monster.WarriorBase.Warrior.WarriorCast2SkillsApproachAggressive", warrior.Cast2SkillsApproachAggressive},
	{"script.ai.individual.Monster.WarriorBase.Warrior.WarriorCastSelfRangeDDMagicAggressive", warrior.CastSelfRangeDDMagicAggressive},
	{"script.ai.individual.Monster.WarriorBase.Warrior.WarriorCastSelfRangeDDMagicBuffAggressive", warrior.CastSelfRangeDDMagicBuffAggressive},
	{"script.ai.individual.Monster.WarriorBase.Warrior.WarriorCorpseNecroLongAggressive", warrior.CorpseNecroLongAggressive},
	{"script.ai.individual.Monster.WarriorBase.Warrior.WarriorDisguise", warrior.Disguise},
	{"script.ai.individual.Monster.WarriorBase.Warrior.WarriorFlee", warrior.Flee},
}

// bootIndividual1b is bootIndividual1 with the second PR's classes in
// place of the first's.
func bootIndividual1b(t *testing.T, extra raidBoot, templates ...*npc.Template) *ind1World {
	t.Helper()
	list, catalog := warriorScripts()
	for _, c := range ind1bBatch {
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

// ind1bTemplate is a fixture NPC of id with ai, listing skills by type, in
// the clan the scenario's NPCs share.
func ind1bTemplate(id int32, ai npc.AIParams, skills map[npc.SkillType]modelskill.ID) *npc.Template {
	tmpl := warriorTemplate(id, ai)
	tmpl.SkillsByType = map[npc.SkillType]modelskill.Ref{}
	for typ, sk := range skills {
		tmpl.SkillsByType[typ] = modelskill.Ref{ID: sk, Level: 1}
	}
	tmpl.Clans, tmpl.ClanRange = []string{"ind1b"}, 600
	return tmpl
}

// ind1bSkills are every skill the batch's fixture NPCs list: one-target
// magic of reach 600, and the buffs, cast on the caster itself.
func ind1bSkills() *modelskill.Table {
	return modelskill.NewTable([]modelskill.Definition{
		ind1Skill(ind1bSelfBuffID, 0, true), ind1Skill(ind1bSpecialID, 600, false), ind1Skill(ind1bLongDDID, 600, false),
		ind1Skill(ind1bSelfRangeDDID, 600, false), ind1bHeldBuff(), ind1Skill(ind1bDD1ID, 600, false), ind1Skill(ind1bDD2ID, 600, false),
	})
}

// ind1bHeldBuff is the self range buffer's buff: a one-target buff whose
// effect its target holds for a minute once it lands.
func ind1bHeldBuff() modelskill.Definition {
	d := ind1Skill(ind1bBuffID, 600, true)
	d.Target = modelskill.TargetOne
	d.Effects = []modelskill.EffectTemplate{{Name: "Buff", Time: 60, Icon: true, StackType: "ind1b_buff", StackOrder: 1}}
	return d
}

// dryadRoot is the root whose stack the self range caster's attacked hook
// checks on itself.
func dryadRoot() modelskill.Definition {
	d := ind1Skill(dryadRootID, 600, false)
	d.Effects = []modelskill.EffectTemplate{{Name: "Buff", Time: 60, Icon: true, StackType: "ind1b_root", StackOrder: 1}}
	return d
}

// land applies the effects of skill id, level 1, on target from the
// character, and waits until target holds them.
func (w *ind1World) land(t *testing.T, id modelskill.ID, target *npc.Hostile) {
	t.Helper()
	ref := modelskill.Ref{ID: id, Level: 1}
	me := w.player.ObjectID()
	if !w.srv.RunScript(t, me, "Ind1Director", func(s *script.Script, p *script.Player) {
		s.ApplyEffects(ref, p, script.NPCOf(target))
	}) {
		t.Fatal("the effect landing panicked")
	}
	w.srv.AdvanceUntil(t, fmt.Sprintf("skill %d landed", id), func() bool {
		var level int32
		w.srv.RunScript(t, me, "Ind1Director", func(s *script.Script, _ *script.Player) {
			level = s.AbnormalLevel(script.NPCOf(target), ref)
		})
		return level > 0
	})
}

// clear drops every desire h holds.
func (w *ind1World) clear(t *testing.T, h *npc.Hostile) {
	t.Helper()
	w.onQueue(t, h, func() { h.AI().Desires().Clear() })
}

// setTop makes c h's top desire target, its desires dropped.
func (w *ind1World) setTop(t *testing.T, h *npc.Hostile, c script.Creature) {
	t.Helper()
	w.onQueue(t, h, func() {
		h.AI().Desires().Clear()
		script.NPCOf(h).SetTopDesireTarget(c)
	})
}

// onCharacter runs fn on the character's queue and waits for it.
func (w *ind1World) onCharacter(t *testing.T, fn func(c *player.Character)) {
	t.Helper()
	done := make(chan struct{})
	if !w.srv.PlayerQueue(t, w.player.ObjectID()).Post(func() {
		defer close(done)
		fn(w.player.(player.CharacterHolder).PlayerCharacter())
	}) {
		t.Fatal("the character's queue is closed")
	}
	<-done
	w.srv.Settle(t)
	w.srv.ReadQueued(t, w.srv.Client)
}

// hallTemplate is a fixture hall keeper of id with its self buff and
// physical special, and a creation shout a Warrior would say.
func hallTemplate(id int32) *npc.Template {
	return ind1bTemplate(id, npc.AIParams{"ShoutMsg1": "1000403"}, map[npc.SkillType]modelskill.ID{
		npc.SkillTypeSelfBuff: ind1bSelfBuffID, npc.SkillTypePhysicalSpecial: ind1bSpecialID,
	})
}

// TestHallKeeperBuffsAndAttacksWhatItSees ports
// WarriorCast2SkillsApproachAggressive's created and see-creature hooks.
// Created, it runs none of its parents' created hooks: no shout. Whatever it
// sees it wants its self buff, weight 1000000, before it tells a player or
// summon apart; a player in its territory it then wants to attack, weight
// 200. A look around that sees the character and a monster wants the buff
// twice and the character once more; the monster gains no hate.
func TestHallKeeperBuffsAndAttacksWhatItSees(t *testing.T) {
	t.Parallel()
	w := bootIndividual1b(t, raidBoot{}, hallTemplate(hallCaptainID), warriorTemplate(unboundID, nil))
	keeper := w.spawnFrames(t, hallCaptainID, 150, 0)
	n := keeper.h
	w.srv.InstallNPCCastSeam(t, n, ind1bSkills())
	if got := says(keeper.frames, n.ObjectID()); len(got) != 0 {
		t.Fatalf("created lines = %q, want none", got)
	}
	for _, slot := range []script.IntSlot{script.SlotIntAI1, script.SlotIntAI3, script.SlotIntAI4} {
		if got := ind1Slot(n, slot); got != 0 {
			t.Fatalf("created slot %v = %d, want 0", slot, got)
		}
	}

	w.ticks(t, 1)
	w.wantCasts(t, "after it saw the character", n, map[modelskill.ID]ind1Cast{ind1bSelfBuffID: {n.ObjectID(), 1000000}})
	if got := n.AggroHate(w.player); got != 200 {
		t.Fatalf("hate of the character it sees = %v, want 200", got)
	}

	monster := w.spawn(t, unboundID, 150, 50)
	w.clear(t, n)
	w.onQueue(t, n, func() { script.NPCOf(n).LookNeighbor() })
	w.wantCasts(t, "after it looked at the character and a monster", n, map[modelskill.ID]ind1Cast{ind1bSelfBuffID: {n.ObjectID(), 2000000}})
	if got := n.AggroHate(w.player); got != 400 {
		t.Fatalf("hate of the character after the look = %v, want 200 more", got)
	}
	if got := n.AggroHate(monster); got != 0 {
		t.Fatalf("hate of the monster it sees = %v, want none", got)
	}
}

// TestHallKeeperNamesWhatItFights ports WarriorCast2SkillsApproachAggressive's
// attacked and timer hooks. A hit, on draw 0 under 33, wants its physical
// special on the attacker, weight 1000000; on a draw of 33 nothing, and
// either way it remembers its top desire target. 40 s after it appeared it
// tells the NPCs within 8000 (event 0, arg1 10042) the remembered creature,
// and 60 s later once more (arg1 10043), then never again. Hit by a player
// or summon, its clan helps by damage / (level + 7) x 1000, a hit of 0
// counting as 1.
func TestHallKeeperNamesWhatItFights(t *testing.T) {
	t.Parallel()
	w := bootIndividual1b(t, raidBoot{}, hallTemplate(hallCaptainID), hallTemplate(hallPatrolID), warriorTemplate(ind1ListenerID, nil))
	n := w.spawn(t, hallCaptainID, 1500, 0)
	member := w.spawn(t, hallPatrolID, 1500, 100)
	w.srv.InstallNPCCastSeam(t, n, ind1bSkills())
	w.spawn(t, ind1ListenerID, 1500, 3000)
	me := w.player.ObjectID()

	w.setTop(t, n, script.NPCOf(n))
	w.draws.set(33)
	w.hit(t, n, 10)
	w.wantCasts(t, "after a losing hit", n, nil)
	if got := script.NPCOf(n).Slot(script.SlotCreature1); got == nil || got.ObjectID() != n.ObjectID() {
		t.Fatalf("remembered creature = %v, want the keeper itself", got)
	}
	if got := member.AggroHate(w.player); got != 1250 {
		t.Fatalf("clan member hate after a hit of 10 = %v, want 10 / 8 x 1000", got)
	}

	w.setTop(t, n, w.charOf(t))
	w.draws.set(0)
	w.zeroHit(t, n)
	w.wantCasts(t, "after a winning hit", n, map[modelskill.ID]ind1Cast{ind1bSpecialID: {me, 1000000}})
	if got := script.NPCOf(n).Slot(script.SlotCreature1); got == nil || got.ObjectID() != me {
		t.Fatalf("remembered creature = %v, want the character", got)
	}
	if got := member.AggroHate(w.player); got != 1250+125 {
		t.Fatalf("clan member hate after a hit of 0 = %v, want 1250 + 1 / 8 x 1000", got)
	}

	w.srv.Advance(t, 39*time.Second)
	if got := w.heard.take(); len(got) != 0 {
		t.Fatalf("heard within 40 s: %q", got)
	}
	w.srv.Advance(t, time.Second)
	if got := w.heard.take(); !slices.Equal(got, []string{fmt.Sprintf("event=0 arg1=10042 arg2=%d", me)}) {
		t.Fatalf("heard at 40 s: %q, want event 0 arg1 10042 naming the character", got)
	}
	w.srv.Advance(t, 59*time.Second)
	if got := w.heard.take(); len(got) != 0 {
		t.Fatalf("heard within the next 60 s: %q", got)
	}
	w.srv.Advance(t, time.Second)
	if got := w.heard.take(); !slices.Equal(got, []string{fmt.Sprintf("event=0 arg1=10043 arg2=%d", me)}) {
		t.Fatalf("heard at 100 s: %q, want event 0 arg1 10043 naming the character", got)
	}
	w.srv.Advance(t, 120*time.Second)
	if got := w.heard.take(); len(got) != 0 {
		t.Fatalf("heard after the second call: %q", got)
	}
}

// zeroHit lands a skill hit of no damage on h from the character, on its
// queue.
func (w *ind1World) zeroHit(t *testing.T, h *npc.Hostile) {
	t.Helper()
	done := make(chan struct{})
	if !w.srv.PlayerQueue(t, w.player.ObjectID()).Post(func() {
		defer close(done)
		h.ReduceHP(0, w.player, ind1Skill(ind1bDD1ID, 600, false))
	}) {
		t.Fatal("the character's queue is closed")
	}
	<-done
	w.srv.Settle(t)
}

// die kills h with no killer, and lets its dying hooks run.
func (w *ind1World) die(t *testing.T, h *npc.Hostile) {
	t.Helper()
	w.kill(t, h)
	w.srv.Advance(t, 3500*time.Millisecond)
}

// charOf returns the character as a script creature.
func (w *ind1World) charOf(t *testing.T) script.Creature {
	t.Helper()
	var me script.Creature
	w.srv.RunScript(t, w.player.ObjectID(), "Ind1Director", func(_ *script.Script, p *script.Player) { me = p })
	return me
}

// TestHallKeeperTurnsOnACleric ports WarriorCast2SkillsApproachAggressive's
// see-spell hook. Seeing a cleric cast, on a draw under 20 it drops every
// desire and wants the caster alone, weight 200000; on a draw of 20, or
// seeing a fighter cast, it does nothing.
func TestHallKeeperTurnsOnACleric(t *testing.T) {
	t.Parallel()
	for _, row := range []struct {
		name  string
		class int
		draw  int
		turns bool
	}{
		{"cleric, draw 0", 15, 0, true},
		{"cleric, draw 20", 15, 20, false},
		{"fighter, draw 0", 0, 0, false},
	} {
		t.Run(row.name, func(t *testing.T) {
			t.Parallel()
			w := bootIndividual1b(t, raidSpellBoot(t), hallTemplate(hallCaptainID))
			n := w.spawn(t, hallCaptainID, 1500, 0)
			w.srv.InstallNPCCastSeam(t, n, ind1bSkills())
			w.onCharacter(t, func(c *player.Character) { c.SetClassID(row.class) })
			w.draws.set(99)
			w.hit(t, n, 10)
			w.moveCharacter(t, n, 300, 0)
			w.srv.Settle(t)
			w.ticks(t, 1)
			before := n.AggroHate(w.player)
			w.draws.set(row.draw)
			w.castSelfBuff()
			w.srv.Advance(t, 2*time.Second)
			d, ok := desire(n, actorai.IntentionAttack)
			if !row.turns {
				if got := n.AggroHate(w.player); got != before {
					t.Fatalf("hate after the spell = %v, want %v", got, before)
				}
				return
			}
			if !ok || d.FinalTarget == nil || d.FinalTarget.ObjectID() != w.player.ObjectID() || d.Weight != 200000 {
				t.Fatalf("attack desire after the cleric's spell = %+v (%v), want the caster alone at 200000", d, ok)
			}
			if got := n.AggroHate(w.player); got != 200000 {
				t.Fatalf("hate after the cleric's spell = %v, want its earlier hate dropped and 200000", got)
			}
		})
	}
}

// TestHallKeeperCountsDeathsAndDropsADewdrop ports
// WarriorCast2SkillsApproachAggressive's dying and script-event hooks. A
// dying captain tells the NPCs within 5000 (event 0, arg1 10037), which
// another captain counts and a guard does not; a dying guard tells them
// arg1 10038, which another guard counts and a captain does not. A patrol
// tells nothing. A captain that heard nine
// captains die drops a Dewdrop of Destruction for its killer instead of
// telling anyone.
func TestHallKeeperCountsDeathsAndDropsADewdrop(t *testing.T) {
	t.Parallel()
	w := bootIndividual1b(t, raidBoot{opts: []gameservertest.Option{gameservertest.WithItemTemplates(dewdropItems())}},
		hallTemplate(hallCaptainID), hallTemplate(hallGuardID), hallTemplate(hallPatrolID), warriorTemplate(ind1ListenerID, nil))
	survivor := w.spawn(t, hallCaptainID, 1500, 0)
	captain := w.spawn(t, hallCaptainID, 1500, 200)
	guard := w.spawn(t, hallGuardID, 1500, -200)
	guardTwo := w.spawn(t, hallGuardID, 1300, -200)
	patrol := w.spawn(t, hallPatrolID, 1300, 0)
	w.spawn(t, ind1ListenerID, 1500, 3000)

	w.die(t, captain)
	if got := w.heard.take(); !slices.Equal(got, []string{"event=0 arg1=10037 arg2=0"}) {
		t.Fatalf("heard after the captain's death: %q, want arg1 10037", got)
	}
	if a3, a4 := ind1Slot(survivor, script.SlotIntAI3), ind1Slot(survivor, script.SlotIntAI4); a3 != 0 || a4 != 1 {
		t.Fatalf("survivor's guard and captain counts = %d, %d, want 0, 1", a3, a4)
	}
	w.die(t, guard)
	if got := w.heard.take(); !slices.Equal(got, []string{"event=0 arg1=10038 arg2=0"}) {
		t.Fatalf("heard after the guard's death: %q, want arg1 10038", got)
	}
	if a3, a4 := ind1Slot(survivor, script.SlotIntAI3), ind1Slot(survivor, script.SlotIntAI4); a3 != 0 || a4 != 1 {
		t.Fatalf("a captain counted a guard: counts %d, %d, want 0, 1", a3, a4)
	}
	if a3, a4 := ind1Slot(guardTwo, script.SlotIntAI3), ind1Slot(guardTwo, script.SlotIntAI4); a3 != 1 || a4 != 0 {
		t.Fatalf("the other guard's guard and captain counts = %d, %d, want 1, 0", a3, a4)
	}
	w.die(t, patrol)
	if got := w.heard.take(); len(got) != 0 {
		t.Fatalf("heard after the patrol's death: %q, want nothing", got)
	}

	w.onQueue(t, survivor, func() { script.NPCOf(survivor).SetInt(script.SlotIntAI4, 9) })
	w.hit(t, survivor, survivor.MaxHP()+1)
	w.srv.Advance(t, 3500*time.Millisecond)
	frames := w.srv.ReadQueued(t, w.srv.Client)
	drops := framesOf(frames, serverpackets.OpcodeDropItem, survivor.ObjectID())
	if len(drops) != 1 || int32(binary.LittleEndian.Uint32(drops[0][9:13])) != dewdropID {
		t.Fatalf("DropItem frames from the survivor = %x, want one of item %d", drops, dewdropID)
	}
	if got := w.heard.take(); len(got) != 0 {
		t.Fatalf("heard after the survivor's death: %q, want nothing", got)
	}
}

// dewdropItems is the fixture item catalog with the Dewdrop of Destruction.
func dewdropItems() *item.Table {
	templates := gameservertest.ItemTemplates().All()
	templates = append(templates, &item.Template{
		ID: dewdropID, Name: "Dewdrop of Destruction", Kind: item.KindEtcItem, Duration: -1,
		Stackable: true, Dropable: true, Destroyable: true, EtcItem: &item.EtcItemDetail{},
	})
	return item.NewTable(templates)
}

// selfRangeTemplate is a fixture WarriorCastSelfRangeDDMagicAggressive NPC
// of id: its long range and self range magic.
func selfRangeTemplate(id int32) *npc.Template {
	return ind1bTemplate(id, nil, map[npc.SkillType]modelskill.ID{
		npc.SkillTypeWLongRangeDDMagic: ind1bLongDDID, npc.SkillTypeSelfRangeDDMagic: ind1bSelfRangeDDID,
	})
}

// TestSelfRangeCasterStrikesWhatItSeesFromAfar ports
// WarriorCastSelfRangeDDMagicAggressive's see-creature hook. Settled (life
// time past 7), fighting no one and in its territory, it wants its long
// range magic on a player farther than 200, weight 1000000, and attacks it
// (200); from 200 or nearer it only attacks. Young it does nothing at all,
// not even its parents' see-creature hooks.
func TestSelfRangeCasterStrikesWhatItSeesFromAfar(t *testing.T) {
	t.Parallel()
	for _, row := range []struct {
		name  string
		dist  int
		young bool
		cast  bool
		hate  float64
	}{
		{"far", 300, false, true, 200},
		{"near", 200, false, false, 200},
		{"young", 300, true, false, 0},
	} {
		t.Run(row.name, func(t *testing.T) {
			t.Parallel()
			w := bootIndividual1b(t, raidBoot{}, selfRangeTemplate(selfRangeID))
			n := w.spawn(t, selfRangeID, 1500, 0)
			w.srv.InstallNPCCastSeam(t, n, ind1bSkills())
			if !row.young {
				w.ticks(t, 9)
				if n.LifeTime() <= 7 {
					t.Fatalf("setup: life time %d, want past 7", n.LifeTime())
				}
			}
			if got := n.AggroHate(w.player); got != 0 {
				t.Fatalf("setup: the caster saw the character from afar: hate %v", got)
			}
			w.moveCharacter(t, n, row.dist, 0)
			w.srv.Settle(t)
			w.ticks(t, 1)
			want := map[modelskill.ID]ind1Cast{}
			if row.cast {
				want[ind1bLongDDID] = ind1Cast{w.player.ObjectID(), 1000000}
			}
			w.wantCasts(t, "after it saw the character", n, want)
			if got := n.AggroHate(w.player); got != row.hate {
				t.Fatalf("hate of the character it sees = %v, want %v", got, row.hate)
			}
		})
	}
}

// TestSelfRangeCasterAnswersHits ports
// WarriorCastSelfRangeDDMagicAggressive's attacked hook, every draw 0, on
// a hit of 10 (the Warrior hate of a hit is 10 / 8 x 100 = 125). With no
// top desire target it adds only that hate. With the character its top
// desire target 300 away it wants its long range magic on it. With another
// top desire target and the character within 200 it wants its self range
// magic on the character. Rooted (an effect of 1201's stack on it), with
// the character its top desire target farther than 40 and attackable, it
// adds the hate once more; with a top desire target it cannot attack within
// 600 (it cannot move and the monster is out of its reach), it wants its
// long range magic on that one, and its self range magic on the character
// within 200; with one 600 or farther away, it stops hating that one and
// adds the hate of the hit to the attacker.
func TestSelfRangeCasterAnswersHits(t *testing.T) {
	t.Parallel()
	boot := func(t *testing.T) (*ind1World, *npc.Hostile, *npc.Hostile) {
		t.Helper()
		w := bootIndividual1b(t, raidBoot{opts: []gameservertest.Option{gameservertest.WithScriptSkills(dryadRoot())}},
			selfRangeTemplate(selfRangeID), warriorTemplate(unboundID, nil))
		n := w.spawn(t, selfRangeID, 1500, 0)
		other := w.spawn(t, unboundID, 1500, 300)
		w.srv.InstallNPCCastSeam(t, n, ind1bSkills())
		return w, n, other
	}
	t.Run("no top desire target", func(t *testing.T) {
		t.Parallel()
		w, n, _ := boot(t)
		w.moveCharacter(t, n, 300, 0)
		w.setTop(t, n, nil)
		w.hit(t, n, 10)
		w.wantCasts(t, "after the hit", n, nil)
		if got := n.AggroHate(w.player); got != 125 {
			t.Fatalf("hate after the hit = %v, want 125", got)
		}
	})
	t.Run("top desire target from afar", func(t *testing.T) {
		t.Parallel()
		w, n, _ := boot(t)
		w.moveCharacter(t, n, 300, 0)
		w.setTop(t, n, w.charOf(t))
		w.hit(t, n, 10)
		w.wantCasts(t, "after the hit", n, map[modelskill.ID]ind1Cast{ind1bLongDDID: {w.player.ObjectID(), 1000000}})
	})
	t.Run("another top desire target", func(t *testing.T) {
		t.Parallel()
		w, n, other := boot(t)
		w.moveCharacter(t, n, 100, 0)
		w.setTop(t, n, script.NPCOf(other))
		w.hit(t, n, 10)
		w.wantCasts(t, "after the hit", n, map[modelskill.ID]ind1Cast{ind1bSelfRangeDDID: {w.player.ObjectID(), 1000000}})
		w.draws.set(33)
		w.setTop(t, n, script.NPCOf(other))
		w.hit(t, n, 10)
		w.wantCasts(t, "after a losing hit", n, nil)
	})
	t.Run("rooted, attackable top desire target", func(t *testing.T) {
		t.Parallel()
		w, n, _ := boot(t)
		w.land(t, dryadRootID, n)
		w.moveCharacter(t, n, 100, 0)
		w.srv.Settle(t)
		w.ticks(t, 1)
		before := n.AggroHate(w.player)
		w.setTop(t, n, w.charOf(t))
		w.hit(t, n, 10)
		if got := n.AggroHate(w.player) - before; got != 375 {
			t.Fatalf("hate gained from the hit = %v, want 125 from it and 250 from the Warrior's root branch and its own", got)
		}
	})
	t.Run("rooted, unattackable top desire target", func(t *testing.T) {
		t.Parallel()
		tmpl := selfRangeTemplate(selfRangeID)
		tmpl.CanMove = false
		w := bootIndividual1b(t, raidBoot{opts: []gameservertest.Option{gameservertest.WithScriptSkills(dryadRoot())}}, tmpl, warriorTemplate(unboundID, nil))
		n := w.spawn(t, selfRangeID, 1500, 0)
		other := w.spawn(t, unboundID, 1500, 300)
		w.srv.InstallNPCCastSeam(t, n, ind1bSkills())
		w.land(t, dryadRootID, n)
		w.moveCharacter(t, n, 100, 0)
		w.setTop(t, n, script.NPCOf(other))
		if script.NPCOf(n).CanAttack(script.NPCOf(other)) {
			t.Fatal("setup: the caster that cannot move may attack the monster 300 away")
		}
		w.hit(t, n, 10)
		w.wantCasts(t, "after the hit", n, map[modelskill.ID]ind1Cast{
			ind1bLongDDID: {other.ObjectID(), 1000000}, ind1bSelfRangeDDID: {w.player.ObjectID(), 1000000},
		})
	})
	t.Run("rooted, unattackable top desire target from 600 on", func(t *testing.T) {
		t.Parallel()
		tmpl := selfRangeTemplate(selfRangeID)
		tmpl.CanMove = false
		w := bootIndividual1b(t, raidBoot{opts: []gameservertest.Option{gameservertest.WithScriptSkills(dryadRoot())}}, tmpl, warriorTemplate(unboundID, nil))
		n := w.spawn(t, selfRangeID, 1500, 0)
		other := w.spawn(t, unboundID, 1500, 700)
		w.srv.InstallNPCCastSeam(t, n, ind1bSkills())
		w.land(t, dryadRootID, n)
		w.moveCharacter(t, n, 30, 0)
		w.setTop(t, n, script.NPCOf(other))
		w.onQueue(t, n, func() { n.AddDamageHate(other, 0, 500) })
		if got := n.AggroHate(other); got != 500 {
			t.Fatalf("setup: hate of the monster = %v, want 500", got)
		}
		w.draws.set(99)
		w.hit(t, n, 10)
		if got := n.AggroHate(other); got != 0 {
			t.Fatalf("hate of the monster 700 away after the hit = %v, want it stopped", got)
		}
		if got := n.AggroHate(w.player); got != 250 {
			t.Fatalf("hate of the character after the hit = %v, want 125 from this branch and 125 from the Warrior", got)
		}
		w.wantCasts(t, "after the hit", n, nil)
	})
}

// TestSelfRangeCasterHelpsItsClanAndKeepsStriking ports
// WarriorCastSelfRangeDDMagicAggressive's clan-attacked and
// skill-finished hooks, every draw 0. A settled clan member whose top
// desire target is another creature wants its self range magic on itself
// when the attacker is within 200. Once it finishes a cast with the
// character its top desire target farther than 200, it wants its long range
// magic on the character again.
func TestSelfRangeCasterHelpsItsClanAndKeepsStriking(t *testing.T) {
	t.Parallel()
	t.Run("clan", func(t *testing.T) {
		t.Parallel()
		w := bootIndividual1b(t, raidBoot{}, selfRangeTemplate(selfRangeID), selfRangeTemplate(21381), warriorTemplate(unboundID, nil))
		victim := w.spawn(t, 21381, 1500, 0)
		member := w.spawn(t, selfRangeID, 1500, 150)
		other := w.spawn(t, unboundID, 1500, 400)
		w.srv.InstallNPCCastSeam(t, member, ind1bSkills())
		w.ticks(t, 8)
		w.moveCharacter(t, victim, 100, 50)
		w.srv.Settle(t)
		w.setTop(t, member, script.NPCOf(other))
		w.hit(t, victim, 10)
		w.wantCasts(t, "member after the hit", member, map[modelskill.ID]ind1Cast{ind1bSelfRangeDDID: {member.ObjectID(), 1000000}})
	})
	t.Run("skill finished", func(t *testing.T) {
		t.Parallel()
		w := bootIndividual1b(t, raidBoot{}, selfRangeTemplate(selfRangeID))
		n := w.spawn(t, selfRangeID, 1500, 0)
		w.srv.InstallNPCCastSeam(t, n, ind1bSkills())
		w.moveCharacter(t, n, 300, 0)
		w.setTop(t, n, w.charOf(t))
		w.hit(t, n, 10)
		w.srv.AdvanceUntil(t, "the long range cast", func() bool {
			w.ticks(t, 1)
			return len(ind1Casts(n)) == 0
		})
		w.srv.AdvanceUntil(t, "the long range magic wanted again", func() bool {
			w.ticks(t, 1)
			_, ok := ind1Casts(n)[ind1bLongDDID]
			return ok
		})
	})
}

// selfRangeBuffTemplate is a fixture
// WarriorCastSelfRangeDDMagicBuffAggressive NPC of id: its buff and self
// range magic.
func selfRangeBuffTemplate(id int32) *npc.Template {
	return ind1bTemplate(id, nil, map[npc.SkillType]modelskill.ID{
		npc.SkillTypeBuff: ind1bBuffID, npc.SkillTypeSelfRangeDDMagic: ind1bSelfRangeDDID,
	})
}

// TestSelfRangeBufferBuffsItself ports
// WarriorCastSelfRangeDDMagicBuffAggressive's created and see-creature
// hooks. Created, it wants its buff on itself, weight 1000000. Seeing the
// character while it neither attacks nor casts, it wants its buff again
// unless it holds the buff's effect, and attacks the character (200).
func TestSelfRangeBufferBuffsItself(t *testing.T) {
	t.Parallel()
	for _, row := range []struct {
		name string
		held bool
	}{{"buff gone", false}, {"buff held", true}} {
		t.Run(row.name, func(t *testing.T) {
			t.Parallel()
			w := bootIndividual1b(t, raidBoot{opts: []gameservertest.Option{
				gameservertest.WithNpcSpawnSkills(ind1bSkills()), gameservertest.WithScriptSkills(ind1bHeldBuff()),
			}}, selfRangeBuffTemplate(selfRangeBuffID))
			n := w.spawn(t, selfRangeBuffID, 1500, 0)
			w.wantCasts(t, "once created", n, map[modelskill.ID]ind1Cast{ind1bBuffID: {n.ObjectID(), 1000000}})
			if row.held {
				w.land(t, ind1bBuffID, n)
			}
			w.clear(t, n)
			w.moveCharacter(t, n, 150, 0)
			w.srv.Settle(t)
			w.ticks(t, 1)
			want := map[modelskill.ID]ind1Cast{}
			if !row.held {
				want[ind1bBuffID] = ind1Cast{n.ObjectID(), 1000000}
			}
			w.wantCasts(t, "after it saw the character", n, want)
			if got := n.AggroHate(w.player); got != 200 {
				t.Fatalf("hate of the character it sees = %v, want 200", got)
			}
		})
	}
}

// TestSelfRangeBufferAnswersHitsAndBuffsItsClan ports
// WarriorCastSelfRangeDDMagicBuffAggressive's attacked and clan-attacked
// hooks, every draw 0. Hit by the character within 200 while another
// creature is its top desire target and it neither attacks nor casts, it
// wants its buff and its self range magic, each on itself. A settled clan
// member in the same state wants its self range magic on itself and its
// buff on the member that was hit.
func TestSelfRangeBufferAnswersHitsAndBuffsItsClan(t *testing.T) {
	t.Parallel()
	t.Run("attacked", func(t *testing.T) {
		t.Parallel()
		w := bootIndividual1b(t, raidBoot{}, selfRangeBuffTemplate(selfRangeBuffID), warriorTemplate(unboundID, nil))
		n := w.spawn(t, selfRangeBuffID, 1500, 0)
		other := w.spawn(t, unboundID, 1500, 300)
		w.srv.InstallNPCCastSeam(t, n, ind1bSkills())
		w.moveCharacter(t, n, 100, 0)
		w.srv.Settle(t)
		w.setTop(t, n, script.NPCOf(other))
		w.hit(t, n, 10)
		w.wantCasts(t, "after the hit", n, map[modelskill.ID]ind1Cast{
			ind1bBuffID: {n.ObjectID(), 1000000}, ind1bSelfRangeDDID: {n.ObjectID(), 1000000},
		})
	})
	t.Run("clan", func(t *testing.T) {
		t.Parallel()
		w := bootIndividual1b(t, raidBoot{}, selfRangeBuffTemplate(selfRangeBuffID), selfRangeBuffTemplate(21391), warriorTemplate(unboundID, nil))
		victim := w.spawn(t, 21391, 1500, 0)
		member := w.spawn(t, selfRangeBuffID, 1500, 150)
		other := w.spawn(t, unboundID, 1500, 400)
		w.srv.InstallNPCCastSeam(t, member, ind1bSkills())
		w.ticks(t, 8)
		w.moveCharacter(t, victim, 100, 50)
		w.srv.Settle(t)
		w.setTop(t, member, script.NPCOf(other))
		w.hit(t, victim, 10)
		w.wantCasts(t, "member after the hit", member, map[modelskill.ID]ind1Cast{
			ind1bSelfRangeDDID: {member.ObjectID(), 1000000}, ind1bBuffID: {victim.ObjectID(), 1000000},
		})
	})
}

// necroTemplate is a fixture WarriorCorpseNecroLongAggressive NPC: its
// physical special and its two magics.
func necroTemplate(id int32) *npc.Template {
	return ind1bTemplate(id, nil, map[npc.SkillType]modelskill.ID{
		npc.SkillTypePhysicalSpecial: ind1bSpecialID, npc.SkillTypeDDMagic1: ind1bDD1ID, npc.SkillTypeDDMagic2: ind1bDD2ID,
	})
}

// TestCorpseNecroStrikesFromAfar ports WarriorCorpseNecroLongAggressive's
// see-creature hook, every draw 0. Settled, attacking no one and in its
// territory, it wants its physical special on a player it sees farther than
// 100, weight 1000000; nearer, or young, it does not.
func TestCorpseNecroStrikesFromAfar(t *testing.T) {
	t.Parallel()
	for _, row := range []struct {
		name  string
		dist  int
		young bool
		cast  bool
	}{
		{"far", 300, false, true},
		{"near", 100, false, false},
		{"young", 300, true, false},
	} {
		t.Run(row.name, func(t *testing.T) {
			t.Parallel()
			w := bootIndividual1b(t, raidBoot{}, necroTemplate(necroID))
			n := w.spawn(t, necroID, 1500, 0)
			w.srv.InstallNPCCastSeam(t, n, ind1bSkills())
			if !row.young {
				w.ticks(t, 9)
			}
			w.moveCharacter(t, n, row.dist, 0)
			w.srv.Settle(t)
			w.ticks(t, 1)
			want := map[modelskill.ID]ind1Cast{}
			if row.cast {
				want[ind1bSpecialID] = ind1Cast{w.player.ObjectID(), 1000000}
			}
			w.wantCasts(t, "after it saw the character", n, want)
		})
	}
}

// TestCorpseNecroAnswersItsMostHated ports
// WarriorCorpseNecroLongAggressive's attacked and clan-attacked hooks,
// every draw 0. A hit by a player that is not yet its most hated wants
// nothing; once it is, a hit from farther than 100 wants its physical
// special on it, and below 40% of its hit points also its first magic. A
// settled clan member that attacks no one wants its physical special on an
// attacker farther than 100.
func TestCorpseNecroAnswersItsMostHated(t *testing.T) {
	t.Parallel()
	t.Run("attacked", func(t *testing.T) {
		t.Parallel()
		w := bootIndividual1b(t, raidBoot{}, necroTemplate(necroID))
		n := w.spawn(t, necroID, 1500, 0)
		w.srv.InstallNPCCastSeam(t, n, ind1bSkills())
		w.moveCharacter(t, n, 300, 0)
		w.srv.Settle(t)
		me := w.player.ObjectID()
		w.hit(t, n, 10)
		w.wantCasts(t, "after the first hit", n, nil)
		w.hit(t, n, 10)
		w.wantCasts(t, "after a hit by the most hated", n, map[modelskill.ID]ind1Cast{ind1bSpecialID: {me, 1000000}})
		w.clear(t, n)
		w.onQueue(t, n, func() { n.SetHP(n.MaxHPValue() * 0.3) })
		w.hit(t, n, 10)
		w.wantCasts(t, "after a hit below 40%", n, map[modelskill.ID]ind1Cast{ind1bSpecialID: {me, 1000000}, ind1bDD1ID: {me, 1000000}})
	})
	t.Run("clan", func(t *testing.T) {
		t.Parallel()
		w := bootIndividual1b(t, raidBoot{}, necroTemplate(necroID), necroTemplate(21567))
		victim := w.spawn(t, 21567, 1500, 0)
		member := w.spawn(t, necroID, 1500, 150)
		w.srv.InstallNPCCastSeam(t, member, ind1bSkills())
		w.ticks(t, 8)
		w.moveCharacter(t, victim, 0, -100)
		w.srv.Settle(t)
		w.clear(t, member)
		w.hit(t, victim, 10)
		w.wantCasts(t, "member after the hit", member, map[modelskill.ID]ind1Cast{ind1bSpecialID: {w.player.ObjectID(), 1000000}})
	})
}

// TestDisguiseShowsItsSilhouette ports WarriorDisguise's created and
// attacked hooks. Created, it runs none of its parents' created hooks: no
// shout. The first hit puts its silhouette where it stands, told (1000,
// the attacker) as its spawn parameters, so the WarriorBase created hook
// makes it hate the character by 500, and takes the disguise out of the
// world: the character is told to delete it.
func TestDisguiseShowsItsSilhouette(t *testing.T) {
	t.Parallel()
	w := bootIndividual1b(t, raidBoot{}, warriorTemplate(disguiseID, npc.AIParams{"silhouette": "20003", "ShoutMsg1": "1000403"}), warriorTemplate(warriorTwoID, nil))
	spawned := w.spawnFrames(t, disguiseID, 700, 0)
	n := spawned.h
	if got := says(spawned.frames, n.ObjectID()); len(got) != 0 {
		t.Fatalf("created lines = %q, want none", got)
	}
	x, y, z := n.Position()

	w.hit(t, n, 10)
	frames := w.srv.ReadQueued(t, w.srv.Client)
	if w.inWorld(n) {
		t.Fatal("the disguise stays in the world after the hit")
	}
	if got := framesOf(frames, serverpackets.OpcodeDeleteObject, n.ObjectID()); len(got) != 1 {
		t.Fatalf("DeleteObject frames of the disguise = %d, want 1", len(got))
	}
	silhouettes := w.liveNPCs(warriorTwoID)
	if len(silhouettes) != 1 {
		t.Fatalf("silhouettes = %d, want 1", len(silhouettes))
	}
	if sx, sy, sz := silhouettes[0].Position(); sx != x || sy != y || sz != z {
		t.Errorf("silhouette at %d,%d,%d, want the disguise's point %d,%d,%d", sx, sy, sz, x, y, z)
	}
	if got := silhouettes[0].AggroHate(w.player); got != 500 {
		t.Errorf("silhouette hate of the character = %v, want 500", got)
	}
}

// TestFleeRunsFromItsAttacker ports WarriorFlee's no-desire and attacked
// hooks, with a drift range of 250. With nothing to do it wants to wander
// with weight 20 (a Warrior's is 5). A hit makes it want to run 250 from
// the attacker, weight 30, instead of fighting back: no hate and no attack
// desire, and it takes up the run.
func TestFleeRunsFromItsAttacker(t *testing.T) {
	t.Parallel()
	w := bootIndividual1b(t, raidBoot{opts: []gameservertest.Option{gameservertest.WithMaxDriftRange(250)}}, warriorTemplate(fleeID, nil))
	n := w.spawn(t, fleeID, 100, 0)
	w.ticks(t, 2)
	if d, ok := desire(n, actorai.IntentionWander); !ok || d.Weight != 20 || d.Timer != 5 {
		t.Fatalf("wander desire = %+v (%v), want timer 5, weight 20", d, ok)
	}

	w.clear(t, n)
	w.hit(t, n, 10)
	d, ok := desire(n, actorai.IntentionFlee)
	if !ok || d.Distance != 250 || d.Weight != 30 || d.FinalTarget == nil || d.FinalTarget.ObjectID() != w.player.ObjectID() {
		t.Fatalf("flee desire = %+v (%v), want a 250 run from the character, weight 30", d, ok)
	}
	if d, ok := desire(n, actorai.IntentionAttack); ok {
		t.Fatalf("attack desire after the hit = %+v, want none", d)
	}
	if got := n.AggroHate(w.player); got != 0 {
		t.Fatalf("hate after the hit = %v, want none", got)
	}
	w.ticks(t, 1)
	if got := n.AI().CurrentIntention(); got != actorai.IntentionFlee {
		t.Fatalf("intention after the hit = %v, want flee", got)
	}
}

// TestCorpseNecroCastsOnACorpse ports the corpse branch of
// WarriorCorpseNecroLongAggressive's see-creature hook, every draw 0:
// attacking no one while it hates someone (here a monster, by threat
// alone), it wants its second magic on a dead player it sees within 100,
// weight 1000000; hating no one it does not.
func TestCorpseNecroCastsOnACorpse(t *testing.T) {
	t.Parallel()
	for _, row := range []struct {
		name  string
		hates bool
	}{{"hating", true}, {"hating no one", false}} {
		t.Run(row.name, func(t *testing.T) {
			t.Parallel()
			w := bootIndividual1b(t, raidBoot{}, necroTemplate(necroID), warriorTemplate(unboundID, nil))
			n := w.spawn(t, necroID, 1500, 0)
			other := w.spawn(t, unboundID, 1500, 300)
			w.srv.InstallNPCCastSeam(t, n, ind1bSkills())
			w.moveCharacter(t, n, 50, 0)
			w.srv.Settle(t)
			w.onCharacter(t, func(c *player.Character) { c.Die(nil) })
			w.clear(t, n)
			w.ticks(t, 1)
			w.clear(t, n)
			w.onQueue(t, n, func() {
				if row.hates {
					n.AddDamageHate(other, 0, 1)
				}
				script.NPCOf(n).LookNeighbor()
			})
			if got := n.AI().CurrentIntention(); got == actorai.IntentionAttack {
				t.Fatalf("setup: intention %v, want no attack", got)
			}
			if got := script.NPCOf(n).MostHated() != nil; got != row.hates {
				t.Fatalf("setup: hates someone = %v, want %v", got, row.hates)
			}
			want := map[modelskill.ID]ind1Cast{}
			if row.hates {
				want[ind1bDD2ID] = ind1Cast{w.player.ObjectID(), 1000000}
			}
			w.wantCasts(t, "after it saw the corpse", n, want)
		})
	}
}
