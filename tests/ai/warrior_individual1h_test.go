package ai

import (
	"fmt"
	"slices"
	"testing"
	"time"

	"github.com/fatal10110/acis_golang/internal/gameserver/model/actor/npc"
	modelskill "github.com/fatal10110/acis_golang/internal/gameserver/model/skill"
	"github.com/fatal10110/acis_golang/internal/gameserver/network/serverpackets"
	"github.com/fatal10110/acis_golang/internal/gameserver/script"
	"github.com/fatal10110/acis_golang/internal/gameserver/script/ai/warrior"
	"github.com/fatal10110/acis_golang/internal/gameservertest"
)

// The NPC ids the eighth PR of the first individual behavior batch (#186
// PR 8) spawns, each a fixture monster of level 1.
const (
	ancientEggID  = int32(18344) // AncientEgg
	plantID       = int32(18345) // PrimevalPlant
	plantTwoID    = int32(18346) // PrimevalPlant
	royalHoldID   = int32(18134) // RoyalRushWarriorPhysicalSpecial1Hold
	royalHoldTwID = int32(18167) // RoyalRushWarriorPhysicalSpecial1Hold
)

// ind1hDebuffID is the plant's self range debuff.
const ind1hDebuffID = modelskill.ID(9481)

// ind1hBatch are the eighth PR's classes, in scripts.xml order.
var ind1hBatch = []struct {
	path string
	ctor func() script.Script
}{
	{"script.ai.individual.Monster.WarriorBase.Warrior.WarriorHold.AncientEgg", warrior.AncientEgg},
	{"script.ai.individual.Monster.WarriorBase.Warrior.WarriorHold.PrimevalPlant", warrior.PrimevalPlant},
	{"script.ai.individual.Monster.WarriorBase.Warrior.WarriorHold.RoyalRushWarriorPhysicalSpecial1Hold", warrior.RoyalRushWarriorPhysicalSpecial1Hold},
}

// bootIndividual1h boots the eighth PR's classes after the Warrior chain
// beside templates, with a director script to send script events through
// and a listener behavior on ind1ListenerID recording the events it hears,
// the plant's debuff known to scripts, every draw 0 until a test queues others.
func bootIndividual1h(t *testing.T, templates ...*npc.Template) *ind1World {
	t.Helper()
	list, catalog := warriorScripts()
	for _, c := range ind1hBatch {
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
	w := bootRaid(t, raidBoot{opts: []gameservertest.Option{
		gameservertest.WithScriptRand(draws.rand),
		gameservertest.WithNPCScripts(kinds, list, catalog),
		gameservertest.WithScriptSkills(ind1Skill(ind1hDebuffID, 0, true)),
	}}, templates...)
	return &ind1World{warriorWorld: w, draws: draws, heard: heard}
}

// TestAncientEggCallsOutItsAttacker ports AncientEgg's attacked and script
// event hooks. Hit, on a draw of at most 80 it sends script event 10016
// naming the attacker to the NPCs within 300 of it, and on a draw of 81
// sends nothing; it never hates its attacker, its parents left out. Script
// event 10016 sent to it, which a plain Warrior turns on, does nothing.
func TestAncientEggCallsOutItsAttacker(t *testing.T) {
	t.Parallel()
	w := bootIndividual1h(t, warriorTemplate(ancientEggID, nil), warriorTemplate(ind1ListenerID, nil))
	egg := w.spawn(t, ancientEggID, 100, 0)
	w.spawn(t, ind1ListenerID, 300, 0)
	me := w.player.ObjectID()
	want := []string{fmt.Sprintf("event=10016 arg1=%d arg2=0", me)}

	w.hit(t, egg, 10)
	if got := w.heard.take(); !slices.Equal(got, want) {
		t.Fatalf("heard after a hit, draw 0 = %q, want %q", got, want)
	}
	w.draws.set(80)
	w.hit(t, egg, 10)
	if got := w.heard.take(); !slices.Equal(got, want) {
		t.Fatalf("heard after a hit, draw 80 = %q, want %q", got, want)
	}
	w.draws.set(81)
	w.hit(t, egg, 10)
	if got := w.heard.take(); len(got) != 0 {
		t.Fatalf("heard after a hit, draw 81 = %q, want nothing", got)
	}
	if got := egg.AggroHate(w.player); got != 0 {
		t.Fatalf("egg hate after three hits = %v, want none", got)
	}

	w.draws.set(0)
	w.sendEvent(t, egg, 10016, me, 0)
	if got := egg.AggroHate(w.player); got != 0 {
		t.Fatalf("egg hate after event 10016 = %v, want none", got)
	}
}

// plantTemplate is a fixture PrimevalPlant of id casting its self range
// debuff by the chance prob.
func plantTemplate(id int32, prob int) *npc.Template {
	tmpl := warriorTemplate(id, npc.AIParams{"ProbSelfRangeDeBuff1": fmt.Sprint(prob)})
	tmpl.SkillsByType = map[npc.SkillType]modelskill.Ref{npc.SkillTypeSelfRangeDebuff1: {ID: ind1hDebuffID, Level: 1}}
	return tmpl
}

// TestPrimevalPlantDebuffsAroundItEvery15Seconds ports PrimevalPlant's
// created and timer hooks. As it appears it starts the timer "5001" firing
// every 15 s: each firing on a draw of at most ProbSelfRangeDeBuff1 wants
// its self range debuff on itself, weight 1000000, which it then casts
// (MagicSkillUse); on a higher draw it wants nothing.
func TestPrimevalPlantDebuffsAroundItEvery15Seconds(t *testing.T) {
	t.Parallel()
	w := bootIndividual1h(t, plantTemplate(plantID, 30))
	n := w.spawn(t, plantID, 300, 0)
	w.srv.InstallNPCCastSeam(t, n, modelskill.NewTable([]modelskill.Definition{ind1Skill(ind1hDebuffID, 0, true)}))
	w.draws.set(30)
	w.srv.Advance(t, 14900*time.Millisecond)
	ind5bNoCast(t, "before the first firing", n, ind1hDebuffID)
	w.srv.Advance(t, 100*time.Millisecond)
	ind5bWantCast(t, "at the first firing, draw 30", n, ind1hDebuffID, n.ObjectID())
	w.clearDesires(t, n)

	w.draws.set(31)
	w.srv.Advance(t, 15000*time.Millisecond)
	ind5bNoCast(t, "at the second firing, draw 31", n, ind1hDebuffID)

	w.draws.set(0)
	w.srv.Advance(t, 14900*time.Millisecond)
	ind5bNoCast(t, "before the third firing", n, ind1hDebuffID)
	w.srv.Advance(t, 100*time.Millisecond)
	ind5bWantCast(t, "at the third firing, draw 0", n, ind1hDebuffID, n.ObjectID())
	frames := ind5bRunUntil(t, w, "the debuff's cast", func() bool { return len(ind1Casts(n)) == 0 })
	if got := castsOf(frames, n, ind1hDebuffID); len(got) != 1 || got[0] != n.ObjectID() {
		t.Fatalf("debuffs cast on itself = %v, want one", got)
	}
}

// TestPrimevalPlantIgnoresHitsAndScriptEvents ports PrimevalPlant's empty
// attacked and script event hooks: neither a hit nor script event 10016
// makes it hate the character or want anything, its parents left out.
func TestPrimevalPlantIgnoresHitsAndScriptEvents(t *testing.T) {
	t.Parallel()
	w := bootIndividual1h(t, plantTemplate(plantTwoID, 100))
	n := w.spawn(t, plantTwoID, 100, 0)
	w.hit(t, n, 10)
	w.hit(t, n, 10)
	if got := n.AggroHate(w.player); got != 0 {
		t.Fatalf("hate after two hits = %v, want none", got)
	}
	w.sendEvent(t, n, 10016, w.player.ObjectID(), 0)
	if got := n.AggroHate(w.player); got != 0 {
		t.Fatalf("hate after event 10016 = %v, want none", got)
	}
	ind5bNoCast(t, "after the hits", n, ind1hDebuffID)
}

// TestRoyalRushWarriorPhysicalSpecial1HoldStrikesFromWhereItStands ports
// RoyalRushWarriorPhysicalSpecial1Hold's created and attacked hooks, every
// draw 0. Its created hook does not call its parent, so the Warrior's
// created shout is not heard. Hit from farther than 100 by its top desire
// target it wants its first physical special on the attacker held in
// place, weight 1000000; hit from closer it wants none, its parent's
// attack desire still raised. Once a hit finds it below half of its hit
// points it draws its WeaponID weapon glowing at +15 and from then on
// charges a soulshot (MagicSkillUse 2154) rather than striking.
func TestRoyalRushWarriorPhysicalSpecial1HoldStrikesFromWhereItStands(t *testing.T) {
	t.Parallel()
	ai := npc.AIParams{"WeaponID": fmt.Sprint(royalWeaponID), "ShoutMsg1": "1000286"}
	w := bootIndividual1h(t, royalTemplate(royalHoldID, ai), royalTemplate(royalHoldTwID, nil))
	g := w.spawnFrames(t, royalHoldID, 300, 0)
	if got := says(g.frames, g.h.ObjectID()); len(got) != 0 {
		t.Fatalf("created shouts = %q, want none", got)
	}
	n := g.h
	if got := ind1Slot(n, script.SlotIntAI0); got != 0 {
		t.Fatalf("created i_ai0 = %d, want 0", got)
	}
	near := w.spawn(t, royalHoldTwID, 50, 0)
	for _, h := range []*npc.Hostile{n, near} {
		w.srv.InstallNPCCastSeam(t, h, royalSkills())
	}
	me := w.player.ObjectID()

	w.hit(t, n, 10)
	i6fWantCasts(t, "after the first hit", n, nil)
	if got := n.AggroHate(w.player); got <= 0 {
		t.Fatalf("hate after the first hit = %v, want some", got)
	}
	w.hit(t, n, 10)
	i6fWantCasts(t, "after a hit by its top desire target from 300", n, map[modelskill.ID]i6fCast{royalSpecial1ID: {me, 1000000, true}})

	w.hit(t, near, 10)
	w.hit(t, near, 10)
	i6fWantCasts(t, "after a hit by its top desire target from 50", near, nil)
	if got := near.AggroHate(w.player); got <= 0 {
		t.Fatalf("near hate after two hits = %v, want some", got)
	}

	w.clearDesires(t, n)
	w.hit(t, n, int(n.HP())-n.MaxHP()*45/100)
	if s := n.NPCInfoSnapshot(); s.RightHand == int(royalWeaponID) {
		t.Fatal("drew its weapon above half of its hit points")
	}
	w.clearDesires(t, n)
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
	i6fWantCasts(t, "after it drew its weapon", n, nil)
}

// TestRoyalRushWarriorPhysicalSpecial1HoldAssistsItsClan ports
// RoyalRushWarriorPhysicalSpecial1Hold's clan-attacked hook, every draw 0:
// a settled clan member that is not attacking wants its first physical
// special held in place on an attacker farther than 100 from it, weight
// 1000000, and nothing on one closer; a member that drew its weapon
// charges a soulshot (MagicSkillUse 2154) instead, and a member already
// attacking does nothing. Its parent still makes the settled members hate
// the attacker.
func TestRoyalRushWarriorPhysicalSpecial1HoldAssistsItsClan(t *testing.T) {
	t.Parallel()
	w := bootIndividual1h(t, royalTemplate(royalHoldID, nil), royalTemplate(royalHoldTwID, nil))
	victim := w.spawn(t, royalHoldID, 0, 400)
	far := w.spawn(t, royalHoldTwID, 300, 0)
	near := w.spawn(t, royalHoldTwID, 50, 0)
	drawn := w.spawn(t, royalHoldTwID, -300, 0)
	attacking := w.spawn(t, royalHoldTwID, -60, 0)
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
	w.srv.ReadQueued(t, w.srv.Client)
	w.hit(t, victim, 10)
	frames := w.srv.ReadQueued(t, w.srv.Client)
	me := w.player.ObjectID()
	i6fWantCasts(t, "far member", far, map[modelskill.ID]i6fCast{royalSpecial1ID: {me, 1000000, true}})
	i6fWantCasts(t, "near member", near, nil)
	i6fWantCasts(t, "member with its weapon drawn", drawn, nil)
	i6fWantCasts(t, "attacking member", attacking, nil)
	if got := castsOf(frames, drawn, 2154); len(got) != 1 {
		t.Fatalf("soulshot MagicSkillUse frames of the member with its weapon drawn = %d, want 1", len(got))
	}
	for _, h := range []*npc.Hostile{far, near, attacking} {
		if got := castsOf(frames, h, 2154); len(got) != 0 {
			t.Fatalf("member %d charged a soulshot", h.ObjectID())
		}
	}
	for _, h := range []*npc.Hostile{far, near, drawn} {
		if got := h.AggroHate(w.player); got <= 0 {
			t.Fatalf("member %d hate after the clan hit = %v, want some", h.ObjectID(), got)
		}
	}
}

// TestRoyalRushWarriorPhysicalSpecial1HoldAttacksTheNamedCreature ports
// RoyalRushWarriorPhysicalSpecial1Hold's script event 1234: on a draw
// under 80 it wants the creature the event names with weight 300;
// otherwise it drops its attack desires first and wants it with weight
// 1000. Another event, 10016 that its parent heeds, does nothing.
func TestRoyalRushWarriorPhysicalSpecial1HoldAttacksTheNamedCreature(t *testing.T) {
	t.Parallel()
	w := bootIndividual1h(t, royalTemplate(royalHoldID, nil))
	n := w.spawn(t, royalHoldID, 1500, 0)
	me := w.player.ObjectID()
	w.sendEvent(t, n, 10016, me, 0)
	if got := n.AggroHate(w.player); got != 0 {
		t.Fatalf("hate after event 10016 = %v, want none", got)
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
