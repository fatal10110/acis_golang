package ai

import (
	"fmt"
	"slices"
	"testing"
	"time"

	actorai "github.com/fatal10110/acis_golang/internal/gameserver/model/actor/ai"
	"github.com/fatal10110/acis_golang/internal/gameserver/model/actor/npc"
	modelskill "github.com/fatal10110/acis_golang/internal/gameserver/model/skill"
	"github.com/fatal10110/acis_golang/internal/gameserver/network/serverpackets"
	"github.com/fatal10110/acis_golang/internal/gameserver/script"
	"github.com/fatal10110/acis_golang/internal/gameserver/script/ai/warrior"
	"github.com/fatal10110/acis_golang/internal/gameservertest"
)

// The NPC ids the fourth PR of the first individual behavior batch (#186
// PR 4) spawns, each a fixture monster of level 1.
const (
	guardAntID     = int32(29004) // QueenAntPrivateGuardAnt
	petHunterID    = int32(21444) // WarriorAttackPetAggressive
	roarerID       = int32(21269) // WarriorBerserkerAggressive
	holdAggrID     = int32(21661) // WarriorHoldAggressive
	spellWatcherID = int32(21295) // WarriorSeeSpellAggressive
	dyingCallerID  = int32(21653) // WarriorSummonPrivateAtDyingAggressive
	cast3ID        = int32(21177) // WarriorCast3SkillsApproach
	cast3AggrID    = int32(21309) // WarriorCast3SkillsApproachAggressive
)

// ind1dStunID is the stun a transformed NPC casts on the creature its
// predecessor named.
const ind1dStunID = modelskill.ID(4663)

// ind1dBatch are the fourth PR's classes, in scripts.xml order.
var ind1dBatch = []struct {
	path string
	ctor func() script.Script
}{
	{"script.ai.individual.Monster.WarriorBase.Warrior.WarriorAggressive.QueenAntPrivateGuardAnt", warrior.QueenAntPrivateGuardAnt},
	{"script.ai.individual.Monster.WarriorBase.Warrior.WarriorAggressive.WarriorAttackPetAggressive", warrior.AttackPetAggressive},
	{"script.ai.individual.Monster.WarriorBase.Warrior.WarriorAggressive.WarriorBerserkerAggressive", warrior.BerserkerAggressive},
	{"script.ai.individual.Monster.WarriorBase.Warrior.WarriorAggressive.WarriorHoldAggressive", warrior.HoldAggressive},
	{"script.ai.individual.Monster.WarriorBase.Warrior.WarriorAggressive.WarriorSeeSpellAggressive", warrior.SeeSpellAggressive},
	{"script.ai.individual.Monster.WarriorBase.Warrior.WarriorAggressive.WarriorSummonPrivateAtDyingAggressive", warrior.SummonPrivateAtDyingAggressive},
	{"script.ai.individual.Monster.WarriorBase.Warrior.WarriorCast3SkillsApproach.WarriorCast3SkillsApproach", warrior.Cast3SkillsApproach},
	{"script.ai.individual.Monster.WarriorBase.Warrior.WarriorCast3SkillsApproach.WarriorCast3SkillsApproachAggressive", warrior.Cast3SkillsApproachAggressive},
}

// bootIndividual1d boots the fourth PR's classes after the Warrior chain
// beside templates, with extra, a director script to send script events
// and call privates through, a listener behavior on ind1ListenerID, every
// NPC casting the batch's skills, every draw 0 until a scenario queues
// others.
func bootIndividual1d(t *testing.T, extra raidBoot, templates ...*npc.Template) *ind1World {
	t.Helper()
	list, catalog := warriorScripts()
	for _, c := range ind1dBatch {
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
		gameservertest.WithNpcSpawnSkills(ind1dSkills()),
	}, extra.opts...)
	w := bootRaid(t, raidBoot{opts: opts, ready: extra.ready}, templates...)
	return &ind1World{warriorWorld: w, draws: draws, heard: heard}
}

// ind1dTemplate is a fixture monster of id with ai as its AI parameters,
// standing still, listing the batch's skills by type, in the clan the
// scenario's NPCs share.
func ind1dTemplate(id int32, ai npc.AIParams) *npc.Template {
	tmpl := ind1cTemplate(id)
	tmpl.AIParams, tmpl.CanMove = ai, false
	tmpl.SkillsByType[npc.SkillTypeSelfBuff] = modelskill.Ref{ID: ind2SelfBuffSkill, Level: 1}
	return tmpl
}

// ind1dSkills are the skills the batch's fixture NPCs cast: the physical
// special a one-target attack of reach 900, the blast and the self buff
// self skills, and the transformed NPC's stun.
func ind1dSkills() *modelskill.Table {
	return modelskill.NewTable([]modelskill.Definition{
		ind1Skill(ind1cSpecialID, 900, false), ind1Skill(ind1cBlastID, 0, true), ind2SelfBuff(), ind1Skill(ind1dStunID, 900, false),
	})
}

// ind1dSkillHit lands a skill hit of damage on h from the character, on
// its queue.
func (w *ind1World) ind1dSkillHit(t *testing.T, h *npc.Hostile, damage int) {
	t.Helper()
	done := make(chan struct{})
	if !w.srv.PlayerQueue(t, w.player.ObjectID()).Post(func() {
		defer close(done)
		h.ReduceHP(float64(damage), w.player, ind1Skill(ind1cSpecialID, 900, false))
	}) {
		t.Fatal("the character's queue is closed")
	}
	<-done
	w.srv.Settle(t)
}

// TestGuardAntJumpsHomeOutOfItsTerritory ports QueenAntPrivateGuardAnt's
// created and timer "3002" hooks with a drift range of 250. Every 10 s, a
// guard ant 400 from its spawn point, out of its territory, jumps back
// there (TeleportToLocation) and forgets whom it attacks; one 100 away
// stays where it is and keeps its attack desire.
func TestGuardAntJumpsHomeOutOfItsTerritory(t *testing.T) {
	t.Parallel()
	for _, row := range []struct {
		dist int
		home bool
	}{{400, true}, {100, false}} {
		t.Run(fmt.Sprintf("%d away", row.dist), func(t *testing.T) {
			t.Parallel()
			w := bootIndividual1d(t, raidBoot{opts: []gameservertest.Option{gameservertest.WithMaxDriftRange(250)}}, ind1dTemplate(guardAntID, nil))
			n := w.spawn(t, guardAntID, 1500, 0)
			home := script.NPCOf(n).SpawnLocation()
			w.teleport(t, n, row.dist, 0)
			w.hit(t, n, 10)
			if _, ok := desire(n, actorai.IntentionAttack); !ok {
				t.Fatal("setup: no attack desire after the hit")
			}

			w.srv.Advance(t, 10*time.Second)
			w.srv.Settle(t)
			frames := w.srv.ReadQueued(t, w.srv.Client)
			x, y, _ := n.Position()
			jumps := len(framesOf(frames, serverpackets.OpcodeTeleportToLocation, n.ObjectID()))
			_, attacks := desire(n, actorai.IntentionAttack)
			if row.home {
				if x != home.X || y != home.Y || jumps != 1 {
					t.Fatalf("after timer 3002 at %d,%d with %d jumps, want one jump to its spawn point %d,%d", x, y, jumps, home.X, home.Y)
				}
				if attacks {
					t.Fatal("attack desire kept after the jump home, want none")
				}
				return
			}
			if x != home.X+row.dist || jumps != 0 || !attacks {
				t.Fatalf("after timer 3002 at %d with %d jumps (attack desire %v), want it to stay at %d with its attack desire", x, jumps, attacks, home.X+row.dist)
			}
		})
	}
}

// TestGuardAntShufflesItsHate ports QueenAntPrivateGuardAnt's timer "3001":
// 90 s after it appears (a draw of 0 over the 240 s spread) it shuffles its
// hate on a draw under 66, here moving the other NPC's 1000 onto the
// character's 100; on a draw of 66 it keeps its hate.
func TestGuardAntShufflesItsHate(t *testing.T) {
	t.Parallel()
	for _, row := range []struct {
		draw    int
		shuffle bool
	}{{65, true}, {66, false}} {
		t.Run(fmt.Sprintf("draw %d", row.draw), func(t *testing.T) {
			t.Parallel()
			w := bootIndividual1d(t, raidBoot{}, ind1dTemplate(guardAntID, nil), warriorTemplate(unboundID, nil))
			n := w.spawn(t, guardAntID, 400, 0)
			other := w.spawn(t, unboundID, 400, 200)
			ind2Hate(t, w, n, other, 1000, 100)

			w.srv.Advance(t, 89*time.Second)
			if got := n.AggroHate(w.player); got != 100 {
				t.Fatalf("hate of the character before 90 s = %v, want 100", got)
			}
			w.draws.set(0, row.draw, 1000)
			w.srv.Advance(t, time.Second)
			want := 100.0
			if row.shuffle {
				want = 1200
			}
			if got := n.AggroHate(w.player); got != want {
				t.Fatalf("hate of the character after timer 3001 = %v, want %v", got, want)
			}
		})
	}
}

// TestGuardAntTurnsOnItsClansAttacker ports QueenAntPrivateGuardAnt's
// clan-attacked hook, which never calls its parent. A hit of 10 on a
// settled guard ant makes each guard ant of its clan, itself included,
// want to attack the character by int(10 / 8 * 50) = 62; the hit ant adds
// its Warrior attacked hate, 125, and no Warrior clan-attacked 125.
func TestGuardAntTurnsOnItsClansAttacker(t *testing.T) {
	t.Parallel()
	w := bootIndividual1d(t, raidBoot{}, ind1dTemplate(guardAntID, nil))
	hit := w.spawn(t, guardAntID, 1500, 0)
	member := w.spawn(t, guardAntID, 1500, 200)
	w.ticks(t, 8)
	if got := member.AggroHate(w.player); got != 0 {
		t.Fatalf("setup: member hate before the hit = %v, want none", got)
	}
	w.hit(t, hit, 10)
	if got := member.AggroHate(w.player); got != 62 {
		t.Errorf("member hate after the hit = %v, want 62", got)
	}
	if got := hit.AggroHate(w.player); got != 125+62 {
		t.Errorf("hit ant hate = %v, want 187", got)
	}
}

// TestAttackPetTurnsOnTheNamedCreature ports WarriorAttackPetAggressive's
// script-event hook: event 10017 naming the character makes it want to
// attack the character with weight 100; one naming no object wants
// nothing. Its parent's hook still runs: event 10020 naming the character
// adds the Warrior's 100.
func TestAttackPetTurnsOnTheNamedCreature(t *testing.T) {
	t.Parallel()
	w := bootIndividual1d(t, raidBoot{}, ind1dTemplate(petHunterID, nil))
	n := w.spawn(t, petHunterID, 1500, 0)
	me := w.player.ObjectID()

	w.sendEvent(t, n, 10017, 0x7ffffff0, 0)
	if got := n.AggroHate(w.player); got != 0 {
		t.Fatalf("hate after event 10017 naming nothing = %v, want none", got)
	}
	w.sendEvent(t, n, 10017, me, 0)
	if got := n.AggroHate(w.player); got != 100 {
		t.Fatalf("hate after event 10017 naming the character = %v, want 100", got)
	}
	w.sendEvent(t, n, 10020, me, 0)
	if got := n.AggroHate(w.player); got != 200 {
		t.Fatalf("hate after event 10020 = %v, want 200", got)
	}
}

// TestBerserkerAggressiveRoarsAsItsClanDies ports
// WarriorBerserkerAggressive's created and clan-died hooks: it appears with
// i_ai1 clear, and each death of a clan member makes it want to cast its
// fury on itself, one level higher each time, up to its MaxRoarCount of 2.
func TestBerserkerAggressiveRoarsAsItsClanDies(t *testing.T) {
	t.Parallel()
	w := bootIndividual1d(t, raidBoot{}, berserkerTemplate(roarerID), clanMember(unboundID))
	b := w.spawn(t, roarerID, 1500, 0)
	w.srv.InstallNPCCastSeam(t, b, berserkerSkills())
	if got := ind1Slot(b, script.SlotIntAI1); got != 0 {
		t.Fatalf("created i_ai1 = %d, want 0", got)
	}
	for i, wantLevel := range []int{1, 2, 2} {
		w.kill(t, w.spawn(t, unboundID, 1500, 100))
		top := 0
		for _, d := range b.AI().Desires().Snapshot() {
			if d.Kind == actorai.IntentionCast && d.Skill.ID == furyID {
				top = max(top, d.Skill.Level)
				if d.Weight != 1000000 || d.FinalTarget == nil || d.FinalTarget.ObjectID() != b.ObjectID() {
					t.Fatalf("fury desire %+v, want on itself, weight 1000000", d)
				}
			}
		}
		if top != wantLevel {
			t.Fatalf("after %d deaths the highest fury level wanted = %d, want %d", i+1, top, wantLevel)
		}
	}
	if got := ind1Slot(b, script.SlotIntAI1); got != 2 {
		t.Fatalf("roar count = %d, want 2", got)
	}
}

// TestHoldAggressiveWalksHomeWhenIdle ports WarriorHoldAggressive's
// no-desire hook, which never calls its parent: moved off its spawn point
// with nothing to do, it wants to walk back there with weight 30; on its
// spawn point it wants nothing. It never wanders.
func TestHoldAggressiveWalksHomeWhenIdle(t *testing.T) {
	t.Parallel()
	for _, moved := range []bool{true, false} {
		t.Run(fmt.Sprintf("moved %v", moved), func(t *testing.T) {
			t.Parallel()
			tmpl := ind1dTemplate(holdAggrID, nil)
			tmpl.CanMove = true
			w := bootIndividual1d(t, raidBoot{}, tmpl)
			n := w.spawn(t, holdAggrID, 1500, 0)
			home := script.NPCOf(n).SpawnLocation()
			if moved {
				w.teleport(t, n, 200, 0)
			}
			w.ticks(t, 2)
			d, ok := desire(n, actorai.IntentionMoveTo)
			if moved && (!ok || d.Weight != 30 || d.Location.X != home.X || d.Location.Y != home.Y) {
				t.Fatalf("move desire = %+v (%v), want weight 30 to its spawn point %d,%d", d, ok, home.X, home.Y)
			}
			if !moved && ok {
				t.Fatalf("move desire on its spawn point = %+v, want none", d)
			}
			if d, ok := desire(n, actorai.IntentionWander); ok {
				t.Fatalf("wander desire = %+v, want none", d)
			}
		})
	}
}

// TestSeeSpellAggressiveKeepsItsBuffAndStrikes ports
// WarriorSeeSpellAggressive's created, timer "2001" and attacked hooks.
// It appears wanting its self buff on itself, weight 1000000, and wants it
// again 10 s later, out of a fight and not holding it. A hit wants the self
// buff again and, by the character it hates most on a draw under 33, its
// physical special on the character; on a draw of 33 only the self buff.
func TestSeeSpellAggressiveKeepsItsBuffAndStrikes(t *testing.T) {
	t.Parallel()
	w := bootIndividual1d(t, raidBoot{}, ind1dTemplate(spellWatcherID, nil))
	n := w.spawn(t, spellWatcherID, 1500, 0)
	id, me := n.ObjectID(), w.player.ObjectID()
	w.wantCasts(t, "as it appears", n, map[modelskill.ID]ind1Cast{ind2SelfBuffSkill: {id, 1000000}})
	w.clear(t, n)
	w.srv.Advance(t, 10*time.Second)
	w.srv.Settle(t)
	w.wantCasts(t, "after timer 2001", n, map[modelskill.ID]ind1Cast{ind2SelfBuffSkill: {id, 1000000}})

	w.hit(t, n, 10)
	for _, row := range []struct {
		draw   int
		strike bool
	}{{32, true}, {33, false}} {
		w.clear(t, n)
		w.draws.set(row.draw)
		w.hit(t, n, 10)
		want := map[modelskill.ID]ind1Cast{ind2SelfBuffSkill: {id, 1000000}}
		if row.strike {
			want[ind1cSpecialID] = ind1Cast{me, 1000000}
		}
		w.wantCasts(t, fmt.Sprintf("a hit on draw %d", row.draw), n, want)
	}
}

// TestSeeSpellAggressiveAnswersOnlySkillHits ports
// WarriorSeeSpellAggressive's clan-attacked hook: a settled member's
// parent hook, which adds int(10 / 8 * 100) = 125 hate, runs only for a
// skill hit on its clan; a plain hit adds none.
func TestSeeSpellAggressiveAnswersOnlySkillHits(t *testing.T) {
	t.Parallel()
	victim := ind1dTemplate(unboundID, nil)
	w := bootIndividual1d(t, raidBoot{}, ind1dTemplate(spellWatcherID, nil), victim)
	member := w.spawn(t, spellWatcherID, 1500, 0)
	v := w.spawn(t, unboundID, 1500, 200)
	w.ticks(t, 8)
	w.hit(t, v, 10)
	if got := member.AggroHate(w.player); got != 0 {
		t.Fatalf("member hate after a plain hit on its clan = %v, want none", got)
	}
	w.ind1dSkillHit(t, v, 10)
	if got := member.AggroHate(w.player); got != 125 {
		t.Fatalf("member hate after a skill hit on its clan = %v, want 125", got)
	}
}

// TestSummonPrivateAtDyingCallsAndNamesItsTarget ports
// WarriorSummonPrivateAtDyingAggressive's attacked, dying and timer
// hooks. A hit by its top desire target keeps that attacker in i_ai3.
// Its dying hook runs 3 s after it dies: at a SummonPrivateRate of 100 it
// calls its private, then names the attacker (event 10020) to the NPCs
// within 500, the new private among them, and again every 1.5 s (timer "5001"); at a rate of 0 it does
// neither.
func TestSummonPrivateAtDyingCallsAndNamesItsTarget(t *testing.T) {
	t.Parallel()
	for _, rate := range []string{"100", "0"} {
		t.Run("rate "+rate, func(t *testing.T) {
			t.Parallel()
			tmpl := ind1dTemplate(dyingCallerID, npc.AIParams{"SummonPrivateRate": rate})
			tmpl.Privates = []npc.PrivateEntry{{NpcID: int(unboundID), Weight: 1, RespawnDelay: 240 * time.Second}}
			w := bootIndividual1d(t, raidBoot{}, tmpl, warriorTemplate(unboundID, nil))
			n := w.spawn(t, dyingCallerID, 1500, 0)
			w.spawn(t, unboundID, 1500, 200)
			me := w.player.ObjectID()

			w.hit(t, n, 10)
			if got := ind1Slot(n, script.SlotIntAI3); got != 0 {
				t.Fatalf("i_ai3 after a hit by another than its top desire target = %d, want 0", got)
			}
			w.setTop(t, n, script.PlayerOf(w.player))
			w.hit(t, n, 10)
			if got := ind1Slot(n, script.SlotIntAI3); got != me {
				t.Fatalf("i_ai3 after a hit by its top desire target = %d, want %d", got, me)
			}
			w.heard.take()

			w.die(t, n)
			named := fmt.Sprintf("event=10020 arg1=%d arg2=0", me)
			var want []string
			privates := 0
			if rate == "100" {
				// The listener and the private, called just before.
				want, privates = []string{named, named}, 1
			}
			if got := w.heard.take(); !slices.Equal(got, want) {
				t.Fatalf("events heard as it dies = %q, want %q", got, want)
			}
			if got := len(n.Minions()); got != privates {
				t.Fatalf("privates called as it dies = %d, want %d", got, privates)
			}
			w.srv.Advance(t, 1500*time.Millisecond)
			w.srv.Settle(t)
			if got := w.heard.take(); !slices.Equal(got, want) {
				t.Fatalf("events heard 1.5 s later = %q, want %q", got, want)
			}
		})
	}
}

// TestCast3SkillsApproachBuffsAsItAppears ports WarriorCast3SkillsApproach's
// created and timer hooks. Untransformed it appears wanting its self buff
// on itself, weight 1000000. Transformed (IsTransform 1) and called with
// spawn parameters 1000 and the character, it wants no self buff yet but
// its stun on the character, weight 10000 from it and 10000 from its
// WarriorBase parent, and to attack the character, 500 from each; 3 s
// later (timer "2002") it wants its self buff.
func TestCast3SkillsApproachBuffsAsItAppears(t *testing.T) {
	t.Parallel()
	w := bootIndividual1d(t, raidBoot{}, ind1dTemplate(cast3ID, nil), ind1dTemplate(cast3AggrID, npc.AIParams{"IsTransform": "1"}))
	n := w.spawn(t, cast3ID, 1500, 0)
	w.wantCasts(t, "untransformed, as it appears", n, map[modelskill.ID]ind1Cast{ind2SelfBuffSkill: {n.ObjectID(), 1000000}})

	me := w.player.ObjectID()
	tr := w.ind4cCallPrivate(t, n, cast3AggrID, 1500, 200, script.Params{P1: 1000, P2: me})
	w.wantCasts(t, "transformed, as it appears", tr, map[modelskill.ID]ind1Cast{ind1dStunID: {me, 20000}})
	if got := tr.AggroHate(w.player); got != 1000 {
		t.Fatalf("transformed hate of the character = %v, want 1000", got)
	}
	w.clear(t, tr)
	w.srv.Advance(t, 3*time.Second)
	w.srv.Settle(t)
	ind5bWantCast(t, "after timer 2002", tr, ind2SelfBuffSkill, tr.ObjectID())
}

// TestCast3SkillsApproachBlastsAndStrikes ports WarriorCast3SkillsApproach's
// attacked hook, every draw 0. Hit by the character it hates most from 150
// it wants its blast on itself and its physical special on the character,
// weight 1000000; from 300 only the physical special.
func TestCast3SkillsApproachBlastsAndStrikes(t *testing.T) {
	t.Parallel()
	for _, row := range []struct {
		dist  int
		blast bool
	}{{150, true}, {300, false}} {
		t.Run(fmt.Sprintf("%d away", row.dist), func(t *testing.T) {
			t.Parallel()
			w := bootIndividual1d(t, raidBoot{}, ind1dTemplate(cast3ID, nil))
			n := w.spawn(t, cast3ID, 1500, 0)
			w.moveCharacter(t, n, row.dist, 0)
			w.srv.Settle(t)
			w.hit(t, n, 10)
			w.clear(t, n)
			w.hit(t, n, 10)
			want := map[modelskill.ID]ind1Cast{ind1cSpecialID: {w.player.ObjectID(), 1000000}}
			if row.blast {
				want[ind1cBlastID] = ind1Cast{n.ObjectID(), 1000000}
			}
			w.wantCasts(t, "after the hit", n, want)
		})
	}
}

// TestCast3SkillsApproachAnswersItsClan ports WarriorCast3SkillsApproach's
// clan-attacked hook, every draw 0. A settled member 150 from the
// character it hates most wants its blast on itself and its physical
// special on the character when the character hits its clan, weight
// 1000000, and its parent's hook adds int(10 / 8 * 100) = 125 hate.
func TestCast3SkillsApproachAnswersItsClan(t *testing.T) {
	t.Parallel()
	w := bootIndividual1d(t, raidBoot{}, ind1dTemplate(cast3ID, nil), ind1dTemplate(unboundID, nil))
	member := w.spawn(t, cast3ID, 1500, 0)
	v := w.spawn(t, unboundID, 1500, 100)
	w.moveCharacter(t, member, 150, 0)
	w.srv.Settle(t)
	w.ticks(t, 8)
	w.hit(t, member, 10)
	w.clear(t, member)
	before := member.AggroHate(w.player)
	w.hit(t, v, 10)
	w.wantCasts(t, "after the hit on its clan", member, map[modelskill.ID]ind1Cast{
		ind1cBlastID: {member.ObjectID(), 1000000}, ind1cSpecialID: {w.player.ObjectID(), 1000000},
	})
	if got := member.AggroHate(w.player) - before; got != 125 {
		t.Fatalf("member hate gained from the hit on its clan = %v, want 125", got)
	}
}

// TestCast3SkillsApproachAggressiveStrikesOnSight ports
// WarriorCast3SkillsApproachAggressive's see-creature hook, every draw 0.
// Settled, out of a fight and in its territory, it wants its physical
// special on the character it sees, weight 1000000, and turns on it; fresh
// it only turns on it.
func TestCast3SkillsApproachAggressiveStrikesOnSight(t *testing.T) {
	t.Parallel()
	for _, settled := range []bool{true, false} {
		t.Run(fmt.Sprintf("settled %v", settled), func(t *testing.T) {
			t.Parallel()
			w := bootIndividual1d(t, raidBoot{}, ind1dTemplate(cast3AggrID, nil))
			n := w.spawn(t, cast3AggrID, 3000, 0)
			if settled {
				w.ticks(t, 8)
			}
			w.clear(t, n)
			w.moveCharacter(t, n, 300, 0)
			w.srv.Settle(t)
			w.ticks(t, 1)
			if got := n.AggroHate(w.player); got == 0 {
				t.Fatal("no hate of the character it sees")
			}
			if settled {
				ind5bWantCast(t, "after it saw the character", n, ind1cSpecialID, w.player.ObjectID())
			} else {
				ind5bNoCast(t, "after it saw the character", n, ind1cSpecialID)
			}
		})
	}
}
