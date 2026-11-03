package ai

import (
	"fmt"
	"testing"
	"time"

	actorai "github.com/fatal10110/acis_golang/internal/gameserver/model/actor/ai"
	"github.com/fatal10110/acis_golang/internal/gameserver/model/actor/npc"
	modelskill "github.com/fatal10110/acis_golang/internal/gameserver/model/skill"
	"github.com/fatal10110/acis_golang/internal/gameserver/script"
	"github.com/fatal10110/acis_golang/internal/gameserver/script/ai/warrior"
	"github.com/fatal10110/acis_golang/internal/gameserver/world"
	"github.com/fatal10110/acis_golang/internal/gameservertest"
)

// The third individual behavior batch's NPC ids the scenarios spawn (#188
// PR 1), each a fixture monster of level 1.
const (
	ind3BombID        = int32(21517) // PartyLeaderBombSpawn
	ind3BombTwoID     = int32(21512) // PartyLeaderBombSpawn
	ind3CastDDID      = int32(29993) // PartyLeaderCastDD, bound by the test
	ind3CastDDTwoID   = int32(29994) // PartyLeaderCastDD, bound by the test
	ind3CastDDAggroID = int32(20963) // PartyLeaderCastDDAggressive
	ind3CoupleID      = int32(21432) // PartyLeaderCouplePhysicalSpecial
	ind3PatrolID      = int32(27041) // PartyLeaderPatrol
	ind3SpecialID     = int32(20947) // PartyLeaderPhysicalSpecial
	ind3SpecialTwoID  = int32(20989) // PartyLeaderPhysicalSpecial
	ind3SpecialAggrID = int32(20983) // PartyLeaderPhysicalSpecialAggressive
	ind3KetraStoneID  = int32(21345) // PartyLeaderCastDDMagicStoneAggressive
	ind3VarkaStoneID  = int32(21371) // PartyLeaderCastDDMagicStoneAggressive
	ind3CastDDPath    = "test.PartyLeaderCastDD"
)

// The skills the batch's fixture NPCs list by type.
const (
	ind3DDMagicSkill  = modelskill.ID(9601)
	ind3SpecialSkill  = modelskill.ID(9602)
	ind3HealSkill     = modelskill.ID(9603)
	ind3SelfBuffSkill = modelskill.ID(9604)
)

// ind3Batch are the batch's classes, in scripts.xml order.
var ind3Batch = []struct {
	path string
	ctor func() script.Script
}{
	{"script.ai.individual.Monster.WarriorBase.Warrior.PartyLeaderWarrior.PartyLeaderBombSpawn", warrior.PartyLeaderBombSpawn},
	{"script.ai.individual.Monster.WarriorBase.Warrior.PartyLeaderWarrior.PartyLeaderCastDD", warrior.PartyLeaderCastDD},
	{"script.ai.individual.Monster.WarriorBase.Warrior.PartyLeaderWarrior.PartyLeaderCastDDAggressive", warrior.PartyLeaderCastDDAggressive},
	{"script.ai.individual.Monster.WarriorBase.Warrior.PartyLeaderWarrior.PartyLeaderCastDDMagicStoneAggressive", warrior.PartyLeaderCastDDMagicStoneAggressive},
	{"script.ai.individual.Monster.WarriorBase.Warrior.PartyLeaderWarrior.PartyLeaderCouplePhysicalSpecial", warrior.PartyLeaderCouplePhysicalSpecial},
	{"script.ai.individual.Monster.WarriorBase.Warrior.PartyLeaderWarrior.PartyLeaderPatrol", warrior.PartyLeaderPatrol},
	{"script.ai.individual.Monster.WarriorBase.Warrior.PartyLeaderWarrior.PartyLeaderPhysicalSpecial", warrior.PartyLeaderPhysicalSpecial},
	{"script.ai.individual.Monster.WarriorBase.Warrior.PartyLeaderWarrior.PartyLeaderPhysicalSpecialAggressive", warrior.PartyLeaderPhysicalSpecialAggressive},
	// PartyLeaderCastDD binds no NPC of its own; the scenarios bind its
	// hooks to fixture ids to drive them.
	{ind3CastDDPath, func() script.Script {
		s := warrior.PartyLeaderCastDD()
		s.NPCs = []int32{ind3CastDDID, ind3CastDDTwoID}
		return s
	}},
}

// bootIndividual3 boots the batch after the party Warriors beside
// templates, every draw 0 until a scenario queues others, with the
// director script sendEvent sends through.
func bootIndividual3(t *testing.T, templates ...*npc.Template) *ind1World {
	t.Helper()
	return bootIndividual3With(t, raidBoot{}, templates...)
}

// bootIndividual3With is bootIndividual3 with extra's options added last
// and its step run before the character enters the world.
func bootIndividual3With(t *testing.T, extra raidBoot, templates ...*npc.Template) *ind1World {
	t.Helper()
	list, catalog := partyScripts()
	for _, c := range ind3Batch {
		list = append(list, script.Listing{Path: c.path})
		catalog[c.path] = c.ctor
	}
	const director = "test.Ind1Director"
	catalog[director] = func() script.Script { return script.Script{} }
	list = append(list, script.Listing{Path: director})
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
	return &ind1World{warriorWorld: w, draws: draws, heard: &ind1Heard{}}
}

// ind3Template is a fixture batch NPC of id with ai as its AI parameters,
// listing its DD magic, physical special, heal and self buff by type; with
// clan set, in the clan the scenario's NPCs share.
func ind3Template(id int32, ai npc.AIParams, clan bool) *npc.Template {
	tmpl := warriorTemplate(id, ai)
	tmpl.SkillsByType = map[npc.SkillType]modelskill.Ref{
		npc.SkillTypeDDMagic:         {ID: ind3DDMagicSkill, Level: 1},
		npc.SkillTypePhysicalSpecial: {ID: ind3SpecialSkill, Level: 1},
		npc.SkillTypeMagicHeal:       {ID: ind3HealSkill, Level: 1},
		npc.SkillTypeSelfBuff:        {ID: ind3SelfBuffSkill, Level: 1},
	}
	if clan {
		tmpl.Clans, tmpl.ClanRange = []string{"party"}, 500
	}
	return tmpl
}

// ind3Skills are the batch NPCs' skills, the DD magic and physical special
// reaching 2000.
func ind3Skills() *modelskill.Table {
	return modelskill.NewTable([]modelskill.Definition{
		ind1Skill(ind3DDMagicSkill, 2000, false), ind1Skill(ind3SpecialSkill, 2000, false),
		ind1Skill(ind3HealSkill, 0, true), ind1Skill(ind3SelfBuffSkill, 0, true),
	})
}

// ind3Live counts the hostile NPCs of the template npcID in the world.
func ind3Live(w *ind1World, npcID int32) int {
	n := 0
	w.srv.State.FindObject(func(o world.Tracked) bool {
		if h, ok := o.(*npc.Hostile); ok && h.NpcID() == int(npcID) {
			n++
		}
		return false
	})
	return n
}

// clearDesires drops h's queued desires and settles.
func (w *ind1World) clearDesires(t *testing.T, h *npc.Hostile) {
	t.Helper()
	w.onQueue(t, h, func() { h.AI().Desires().Clear() })
}

// TestPartyLeaderCastDDCastsAtWhatItFights ports PartyLeaderCastDD's
// created, attacked and timer hooks, every draw 0. The NPC clears i_ai1
// and i_ai2 as it spawns, and its parent's created hook then sets i_ai2,
// which doubles as the summon flag of a leader with no SummonPrivateRate:
// the first tick of the fight timer is skipped. The first hit arms the 5 s
// timer "2001" with no DD magic. The second tick of the fight wants the DD
// magic on the most hated, weight 1000000, with no check of the target's
// effects, which the NPC then casts (MagicSkillUse). A hit by the most hated marks the next tick
// to skip, and wants the DD magic only when the attacker stands closer
// than 100; out of a fight the tick clears both slots and the timer stops.
func TestPartyLeaderCastDDCastsAtWhatItFights(t *testing.T) {
	t.Parallel()
	w := bootIndividual3(t, ind3Template(ind3CastDDID, nil, false))
	n := w.spawn(t, ind3CastDDID, 300, 0)
	w.srv.InstallNPCCastSeam(t, n, ind3Skills())
	me := w.player.ObjectID()
	if a1, a2 := ind1Slot(n, script.SlotIntAI1), ind1Slot(n, script.SlotIntAI2); a1 != 0 || a2 != 1 {
		t.Fatalf("created i_ai1, i_ai2 = %d, %d, want 0, 1", a1, a2)
	}

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
	w.wantCasts(t, "after the first tick", n, nil)
	if got := ind1Slot(n, script.SlotIntAI2); got != 0 {
		t.Fatalf("i_ai2 after the first tick = %d, want 0", got)
	}
	w.srv.Advance(t, 5*time.Second)
	w.wantCasts(t, "after the second tick", n, map[modelskill.ID]ind1Cast{ind3DDMagicSkill: {me, 1000000}})
	if got := castsOf(w.ticks(t, 2), n, ind3DDMagicSkill); len(got) != 1 || got[0] != me {
		t.Fatalf("DD magic MagicSkillUse targets = %v, want one on %d", got, me)
	}

	for _, row := range []struct {
		name string
		dx   int
		cast bool
	}{{"far", 150, false}, {"near", 50, true}} {
		w.clearDesires(t, n)
		w.moveCharacter(t, n, row.dx, 0)
		w.hit(t, n, 10)
		want := map[modelskill.ID]ind1Cast{}
		if row.cast {
			want[ind3DDMagicSkill] = ind1Cast{me, 1000000}
		}
		w.wantCasts(t, "after a hit by the most hated, "+row.name, n, want)
		if got := ind1Slot(n, script.SlotIntAI2); got != 1 {
			t.Fatalf("%s: i_ai2 after a hit by the most hated = %d, want 1", row.name, got)
		}
	}
	w.clearDesires(t, n)
	w.ticks(t, 1)
	w.srv.Advance(t, 5*time.Second)
	if got := ind1Slot(n, script.SlotIntAI2); got != 0 {
		t.Fatalf("i_ai2 after the tick it skipped = %d, want 0", got)
	}
	w.wantCasts(t, "after the skipped tick", n, nil)

	w.onQueue(t, n, func() { script.NPCOf(n).RemoveAllAttackDesire(); n.AI().Desires().Clear() })
	w.ticks(t, 1)
	if fightingIntention(n) {
		t.Fatalf("intention after the desires were dropped = %v, want neither attack nor cast", n.AI().CurrentIntention())
	}
	w.srv.Advance(t, 5*time.Second)
	if a1, a2 := ind1Slot(n, script.SlotIntAI1), ind1Slot(n, script.SlotIntAI2); a1 != 0 || a2 != 0 {
		t.Fatalf("i_ai1, i_ai2 after the tick out of a fight = %d, %d, want 0, 0", a1, a2)
	}
	w.srv.Advance(t, 10*time.Second)
	w.wantCasts(t, "once the timer stopped", n, nil)
}

// TestPartyLeaderCastDDTickDrawsOnlyWithDDMagic pins the tick's draw
// order: a template that lists no DD magic draws nothing, so the queued
// draw of 99 is still there for the next caller; one that lists it draws
// 99, which is not under 50, and wants nothing.
func TestPartyLeaderCastDDTickDrawsOnlyWithDDMagic(t *testing.T) {
	t.Parallel()
	for _, row := range []struct {
		name   string
		listed bool
		left   int
	}{{"unlisted", false, 99}, {"listed", true, 0}} {
		t.Run(row.name, func(t *testing.T) {
			t.Parallel()
			tmpl := ind3Template(ind3CastDDID, nil, false)
			if !row.listed {
				delete(tmpl.SkillsByType, npc.SkillTypeDDMagic)
			}
			w := bootIndividual3(t, tmpl)
			n := w.spawn(t, ind3CastDDID, 300, 0)
			w.hit(t, n, 10)
			w.ticks(t, 1)
			if !fightingIntention(n) {
				t.Fatalf("intention after the hit = %v, want attack", n.AI().CurrentIntention())
			}
			w.srv.Advance(t, 5*time.Second) // the skipped first tick
			w.clearDesires(t, n)
			w.draws.set(0, 99)
			w.srv.Advance(t, 5*time.Second)
			w.wantCasts(t, "after the second tick", n, nil)
			if got := w.draws.rand(100); got != row.left {
				t.Fatalf("next draw after the tick = %d, want %d", got, row.left)
			}
		})
	}
}

// TestPartyLeaderCastDDAnswersItsClanFromAfar ports PartyLeaderCastDD's
// clan-attacked hook, every draw 0: a settled clan member wants its DD
// magic, weight 1000000, on the attacker of another member standing more
// than 100 from it, even while it fights; one standing at 50 gets none,
// and neither does a young member.
func TestPartyLeaderCastDDAnswersItsClanFromAfar(t *testing.T) {
	t.Parallel()
	for _, row := range []struct {
		name      string
		settle    int
		attacking bool
		dx        int
		cast      bool
	}{{"far", 8, false, 0, true}, {"fighting", 8, true, -300, true}, {"near", 8, false, -50, false}, {"young", 0, false, 0, false}} {
		t.Run(row.name, func(t *testing.T) {
			t.Parallel()
			w := bootIndividual3(t, ind3Template(ind3CastDDID, nil, true), ind3Template(ind3CastDDTwoID, nil, true))
			victim := w.spawn(t, ind3CastDDID, 1500, 0)
			member := w.spawn(t, ind3CastDDTwoID, 1500, 200)
			w.srv.InstallNPCCastSeam(t, member, ind3Skills())
			w.ticks(t, row.settle)
			if row.attacking {
				w.hit(t, member, 10)
				w.ticks(t, 1)
				if !fightingIntention(member) {
					t.Fatalf("setup: member intention = %v, want attack or cast", member.AI().CurrentIntention())
				}
			}
			if row.dx != 0 {
				w.moveCharacter(t, member, row.dx, 0)
			}
			w.clearDesires(t, member)
			w.hit(t, victim, 10)
			want := map[modelskill.ID]ind1Cast{}
			if row.cast {
				want[ind3DDMagicSkill] = ind1Cast{w.player.ObjectID(), 1000000}
			}
			w.wantCasts(t, "member after the hit", member, want)
		})
	}
}

// TestPartyLeaderCastDDAggressiveAttacksOnSight ports
// PartyLeaderCastDDAggressive's see-creature hook: the character it sees
// gets hate 200.
func TestPartyLeaderCastDDAggressiveAttacksOnSight(t *testing.T) {
	t.Parallel()
	w := bootIndividual3(t, ind3Template(ind3CastDDAggroID, nil, false))
	n := w.spawn(t, ind3CastDDAggroID, 1500, 0)
	w.ticks(t, 3)
	if got := n.AggroHate(w.player); got != 0 {
		t.Fatalf("hate before it saw the character = %v, want 0", got)
	}
	w.moveCharacter(t, n, -150, 0)
	w.ticks(t, 1)
	if got := n.AggroHate(w.player); got != 200 {
		t.Fatalf("hate of the character it sees = %v, want 200", got)
	}
}

// TestPartyLeaderBombSpawnCallsPrivatesOnSight ports PartyLeaderBombSpawn's
// created and see-creature hooks. It spawns with i_ai0 0, weight point 10
// and none of its two declared privates, its created hook calling no
// parent. The first character it sees sets i_ai0 and gets hate 200; it
// creates the privates then only with no SummonPrivateRate, and seeing
// the character again creates no more.
func TestPartyLeaderBombSpawnCallsPrivatesOnSight(t *testing.T) {
	t.Parallel()
	for _, row := range []struct {
		name     string
		ai       npc.AIParams
		privates int
	}{{"no summon rate", nil, 2}, {"summon rate", npc.AIParams{"SummonPrivateRate": "30"}, 0}} {
		t.Run(row.name, func(t *testing.T) {
			t.Parallel()
			w := bootIndividual3(t, leaderTemplate(ind3BombID, row.ai, 2), warriorTemplate(partyPrivateID, nil))
			n := w.spawn(t, ind3BombID, 1500, 0)
			if a0, wp := ind1Slot(n, script.SlotIntAI0), ind1Slot(n, script.SlotWeightPoint); a0 != 0 || wp != 10 {
				t.Fatalf("created i_ai0, weight point = %d, %d, want 0, 10", a0, wp)
			}
			w.ticks(t, 3)
			privatesOf(t, n, 0)
			w.moveCharacter(t, n, -150, 0)
			w.ticks(t, 1)
			if got := ind1Slot(n, script.SlotIntAI0); got != 1 {
				t.Fatalf("i_ai0 after it saw the character = %d, want 1", got)
			}
			privatesOf(t, n, row.privates)
			if got := n.AggroHate(w.player); got != 200 {
				t.Fatalf("hate of the character it sees = %v, want 200", got)
			}

			// It sees the character again once the character leaves its
			// sight and comes back; the privates come only once.
			w.moveCharacter(t, n, 800, 0)
			w.ticks(t, 1)
			w.moveCharacter(t, n, -150, 0)
			w.ticks(t, 1)
			privatesOf(t, n, row.privates)
			if got := ind3Live(w, partyPrivateID); got != row.privates {
				t.Fatalf("privates in the world after the second sighting = %d, want %d", got, row.privates)
			}
		})
	}
}

// TestPartyLeaderBombSpawnAttacksTheNamedCreature ports
// PartyLeaderBombSpawn's script-event hook. Event 10000 naming the
// character makes an idle bomb want to attack it with weight 1000 (hate
// 1000); the same event naming no creature, and another event id, do
// nothing. A bomb already attacking adds no hate of the creature event
// 10000 names.
func TestPartyLeaderBombSpawnAttacksTheNamedCreature(t *testing.T) {
	t.Parallel()
	w := bootIndividual3(t, leaderTemplate(ind3BombID, nil, 0), leaderTemplate(ind3BombTwoID, nil, 0))
	idle := w.spawn(t, ind3BombID, 1500, 0)
	busy := w.spawn(t, ind3BombTwoID, -1500, 0)

	w.sendEvent(t, idle, 10000, w.player.ObjectID()+1000000, 0)
	if got := idle.AggroHate(w.player); got != 0 {
		t.Fatalf("hate after an event naming no creature = %v, want 0", got)
	}
	w.sendEvent(t, idle, 10003, w.player.ObjectID(), 0)
	if got := idle.AggroHate(w.player); got != 0 {
		t.Fatalf("hate after event 10003 = %v, want 0", got)
	}
	w.sendEvent(t, idle, 10000, w.player.ObjectID(), 0)
	if got := idle.AggroHate(w.player); got != 1000 {
		t.Fatalf("hate after event 10000 = %v, want 1000", got)
	}

	w.moveCharacter(t, busy, 150, 0)
	w.ticks(t, 1)
	if got := busy.AI().CurrentIntention(); got != actorai.IntentionAttack {
		t.Fatalf("setup: intention once it saw the character = %v, want attack", got)
	}
	w.sendEvent(t, busy, 10000, idle.ObjectID(), 0)
	if got := busy.AggroHate(idle); got != 0 {
		t.Fatalf("hate of the other bomb while attacking = %v, want 0", got)
	}
}

// TestPartyLeaderPatrolNeverWanders ports PartyLeaderPatrol's no-desire
// hook: with no desire left it does nothing, where a
// PartyLeaderPhysicalSpecial beside it wanders.
func TestPartyLeaderPatrolNeverWanders(t *testing.T) {
	t.Parallel()
	w := bootIndividual3(t, ind3Template(ind3PatrolID, nil, false), ind3Template(ind3SpecialID, nil, false))
	patrol := w.spawn(t, ind3PatrolID, 1500, 0)
	roamer := w.spawn(t, ind3SpecialID, -1500, 0)
	w.ticks(t, 3)
	if got := roamer.AI().CurrentIntention(); got != actorai.IntentionWander {
		t.Fatalf("setup: the PartyLeaderPhysicalSpecial intention = %v, want wander", got)
	}
	if got := patrol.AI().CurrentIntention(); got == actorai.IntentionWander {
		t.Fatal("the patrol wanders")
	}
	if d, ok := desire(patrol, actorai.IntentionWander); ok {
		t.Fatalf("the patrol queued a wander desire %+v", d)
	}
}

// TestPartyLeaderPhysicalSpecialStrikesOnceWhenLow ports the attacked
// hooks of PartyLeaderPhysicalSpecial and PartyLeaderCouplePhysicalSpecial.
// A hit by the creature the NPC most wants to attack, while the NPC is
// under 20% of its HP, on a draw under 33, wants the physical special on
// the attacker, weight 1000000, and sets i_ai0, after which no hit strikes
// again. At full HP, before it targets the attacker, or on a draw of 33 it
// wants none. A template that lists no physical special casts nothing but
// still spends its strike.
func TestPartyLeaderPhysicalSpecialStrikesOnceWhenLow(t *testing.T) {
	t.Parallel()
	for _, id := range []int32{ind3SpecialID, ind3CoupleID} {
		for _, row := range []struct {
			name   string
			listed bool
			draw   int
			strike bool
		}{{"strikes", true, 0, true}, {"draw 33", true, 33, false}, {"unlisted", false, 0, true}} {
			t.Run(fmt.Sprintf("%d/%s", id, row.name), func(t *testing.T) {
				t.Parallel()
				tmpl := ind3Template(id, nil, false)
				if !row.listed {
					delete(tmpl.SkillsByType, npc.SkillTypePhysicalSpecial)
				}
				w := bootIndividual3(t, tmpl)
				n := w.spawn(t, id, 50, 0)
				w.srv.InstallNPCCastSeam(t, n, ind3Skills())
				me := w.player.ObjectID()

				setHP := func(share float64) {
					w.onQueue(t, n, func() { n.SetHP(share * float64(script.NPCOf(n).MaxHP())) })
				}
				setHP(0.1)
				w.hit(t, n, 1)
				w.wantCasts(t, "before it targets the character", n, nil)
				w.ticks(t, 1)
				if top := n.AI().TopDesireTarget(); top == nil || top.ObjectID() != me {
					t.Fatalf("setup: top desire target = %v, want the character", top)
				}
				setHP(0.9)
				w.hit(t, n, 1)
				w.wantCasts(t, "at full HP", n, nil)
				if got := ind1Slot(n, script.SlotIntAI0); got != 0 {
					t.Fatalf("i_ai0 at full HP = %d, want 0", got)
				}

				setHP(0.15)
				w.draws.set(row.draw)
				w.hit(t, n, 1)
				want := map[modelskill.ID]ind1Cast{}
				if row.strike && row.listed {
					want[ind3SpecialSkill] = ind1Cast{me, 1000000}
				}
				w.wantCasts(t, "under 20%", n, want)
				wantAI0 := int32(0)
				if row.strike {
					wantAI0 = 1
				}
				if got := ind1Slot(n, script.SlotIntAI0); got != wantAI0 {
					t.Fatalf("i_ai0 under 20%% = %d, want %d", got, wantAI0)
				}
				if !row.strike {
					return
				}
				w.draws.set(0)
				w.clearDesires(t, n)
				w.hit(t, n, 1)
				w.wantCasts(t, "after its strike", n, nil)
			})
		}
	}
}

// TestPartyLeaderPhysicalSpecialAnswersItsClanOnce ports the clan-attacked
// hooks of PartyLeaderPhysicalSpecial and PartyLeaderCouplePhysicalSpecial,
// every draw 0: a settled clan member wants the physical special on
// another member's attacker, weight 1000000, and sets i_ai0, so the next
// hit gets nothing; a young member gets nothing.
func TestPartyLeaderPhysicalSpecialAnswersItsClanOnce(t *testing.T) {
	t.Parallel()
	for _, memberID := range []int32{ind3SpecialTwoID, ind3CoupleID} {
		for _, row := range []struct {
			name   string
			settle int
			strike bool
		}{{"settled", 8, true}, {"young", 0, false}} {
			t.Run(fmt.Sprintf("%d/%s", memberID, row.name), func(t *testing.T) {
				t.Parallel()
				w := bootIndividual3(t, ind3Template(ind3SpecialID, nil, true), ind3Template(memberID, nil, true))
				victim := w.spawn(t, ind3SpecialID, 1500, 0)
				member := w.spawn(t, memberID, 1500, 200)
				w.srv.InstallNPCCastSeam(t, member, ind3Skills())
				w.ticks(t, row.settle)
				w.hit(t, victim, 10)
				want := map[modelskill.ID]ind1Cast{}
				if row.strike {
					want[ind3SpecialSkill] = ind1Cast{w.player.ObjectID(), 1000000}
				}
				w.wantCasts(t, "member after the first hit", member, want)
				if !row.strike {
					return
				}
				if got := ind1Slot(member, script.SlotIntAI0); got != 1 {
					t.Fatalf("member i_ai0 after its strike = %d, want 1", got)
				}
				w.clearDesires(t, member)
				w.hit(t, victim, 10)
				w.wantCasts(t, "member after the second hit", member, nil)
			})
		}
	}
}

// TestPartyLeaderPhysicalSpecialAggressiveStrikesOnSight ports
// PartyLeaderPhysicalSpecialAggressive's see-creature hook, every draw 0.
// Settled, wandering in its territory, it spends its strike on the
// character it sees more than 100 away, then attacks it (hate 200); seen
// at 50 it only attacks. So does a young one, and one already attacking
// another NPC: the sight only adds hate 200.
func TestPartyLeaderPhysicalSpecialAggressiveStrikesOnSight(t *testing.T) {
	t.Parallel()
	for _, row := range []struct {
		name      string
		settle    int
		attacking bool
		dx        int
		strike    bool
	}{{"far", 9, false, -150, true}, {"near", 9, false, -50, false}, {"young", 1, false, -150, false}, {"attacking", 9, true, -150, false}} {
		t.Run(row.name, func(t *testing.T) {
			t.Parallel()
			w := bootIndividual3(t, ind3Template(ind3SpecialAggrID, nil, false), warriorTemplate(unboundID, nil))
			n := w.spawn(t, ind3SpecialAggrID, 1500, 0)
			w.srv.InstallNPCCastSeam(t, n, ind3Skills())
			w.ticks(t, row.settle)
			switch {
			case row.settle <= 7:
				if n.LifeTime() > 7 {
					t.Fatalf("setup: life time %d, want at most 7", n.LifeTime())
				}
			case row.attacking:
				x, y, _ := n.Position()
				prey := w.spawn(t, unboundID, x-w.at.X+100, y-w.at.Y)
				w.onQueue(t, n, func() { script.NPCOf(n).AddAttackDesire(script.NPCOf(prey), 100) })
				w.ticks(t, 1)
				if got := n.AI().CurrentIntention(); got != actorai.IntentionAttack {
					t.Fatalf("setup: intention after it went for the other NPC = %v, want attack", got)
				}
			default:
				if n.LifeTime() <= 7 || n.AI().CurrentIntention() != actorai.IntentionWander {
					t.Fatalf("setup: life time %d, intention %v, want past 7 and wander", n.LifeTime(), n.AI().CurrentIntention())
				}
			}
			hateBefore := n.AggroHate(w.player)
			w.moveCharacter(t, n, row.dx, 0)
			w.ticks(t, 1)
			want := map[modelskill.ID]ind1Cast{}
			wantAI0 := int32(0)
			if row.strike {
				want[ind3SpecialSkill] = ind1Cast{w.player.ObjectID(), 1000000}
				wantAI0 = 1
			}
			w.wantCasts(t, "after it saw the character", n, want)
			if got := ind1Slot(n, script.SlotIntAI0); got != wantAI0 {
				t.Fatalf("i_ai0 after it saw the character = %d, want %d", got, wantAI0)
			}
			if got := n.AggroHate(w.player) - hateBefore; got != 200 {
				t.Fatalf("hate the sight added = %v, want 200", got)
			}
		})
	}
}

// TestPartyLeaderCouplePhysicalSpecialRallies ports
// PartyLeaderCouplePhysicalSpecial's party-died hook. When its private dies
// within 100 of it, it wants to heal and buff itself, weight 1000000 each,
// and drops its hate to want to attack only the creature it most wants to
// attack, weight 1000. A private dying 300 away changes nothing.
func TestPartyLeaderCouplePhysicalSpecialRallies(t *testing.T) {
	t.Parallel()
	for _, row := range []struct {
		name  string
		dx    int
		rally bool
	}{{"near", 50, true}, {"far", 300, false}} {
		t.Run(row.name, func(t *testing.T) {
			t.Parallel()
			leaderTmpl := ind3Template(ind3CoupleID, nil, false)
			leaderTmpl.Privates = []npc.PrivateEntry{{NpcID: int(partyPrivateID), Weight: 1, RespawnDelay: 240 * time.Second}}
			w := bootIndividual3(t, leaderTmpl, warriorTemplate(partyPrivateID, nil))
			leader := w.spawn(t, ind3CoupleID, 300, 0)
			private := privatesOf(t, leader, 1)[0]
			w.srv.InstallNPCCastSeam(t, leader, ind3Skills())
			me := w.player.ObjectID()

			w.hit(t, leader, 10)
			w.ticks(t, 1)
			if top := leader.AI().TopDesireTarget(); top == nil || top.ObjectID() != me {
				t.Fatalf("setup: top desire target = %v, want the character", top)
			}
			w.clearDesires(t, leader)
			lx, ly, lz := leader.Position()
			w.onQueue(t, private, func() {
				script.NPCOf(private).InstantTeleportTo(script.Loc{X: lx + row.dx, Y: ly, Z: lz})
			})
			w.kill(t, private)

			want := map[modelskill.ID]ind1Cast{}
			if row.rally {
				want[ind3HealSkill] = ind1Cast{leader.ObjectID(), 1000000}
				want[ind3SelfBuffSkill] = ind1Cast{leader.ObjectID(), 1000000}
			}
			w.wantCasts(t, "after the private died", leader, want)
			d, ok := desire(leader, actorai.IntentionAttack)
			if !row.rally {
				if ok {
					t.Fatalf("attack desire after a far private died = %+v, want none", d)
				}
				return
			}
			if !ok || d.FinalTarget == nil || d.FinalTarget.ObjectID() != me || d.Weight != 1000 {
				t.Fatalf("attack desire after the private died = %+v (%v), want weight 1000 on the character", d, ok)
			}
			if got := leader.AggroHate(w.player); got != 1000 {
				t.Fatalf("hate of the character after the private died = %v, want 1000", got)
			}
		})
	}
}
