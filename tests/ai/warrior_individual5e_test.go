package ai

import (
	"testing"
	"time"

	actorai "github.com/fatal10110/acis_golang/internal/gameserver/model/actor/ai"
	"github.com/fatal10110/acis_golang/internal/gameserver/model/actor/npc"
	modelskill "github.com/fatal10110/acis_golang/internal/gameserver/model/skill"
	"github.com/fatal10110/acis_golang/internal/gameserver/script"
	"github.com/fatal10110/acis_golang/internal/gameserver/script/ai/warrior"
	"github.com/fatal10110/acis_golang/internal/gameservertest"
)

// The NPC ids of the fifth individual behavior batch's fifth PR the tests
// spawn.
const (
	ind5eSleepID      = int32(20284) // WarriorCastSleepMagic
	ind5eSleepAggrID  = int32(20673) // WarriorCastSleepMagicAggressive
	ind5eSplashID     = int32(20449) // WarriorCastSplash
	ind5eSplashAggrID = int32(21428) // WarriorCastSplashAggressive
	ind5eTrooperID    = int32(35414) // Agit02DoomTrooperAgit
	ind5eTrooperTwoID = int32(35635) // Agit02DoomTrooperAgit
)

// The skills the batch's fixture NPCs list by type.
const (
	ind5eSleep  = modelskill.ID(9540) // SLEEP_MAGIC
	ind5eSplash = modelskill.ID(9541) // SELF_RANGE_DD_MAGIC
)

// ind5eSleepStack is the stack type of the sleep's effect.
const ind5eSleepStack = "ind5e_sleep"

// ind5eBatch are the classes of the PR, in scripts.xml order.
var ind5eBatch = []struct {
	path string
	ctor func() script.Script
}{
	{"script.ai.individual.Monster.WarriorBase.Warrior.WarriorCastSleepMagic.WarriorCastSleepMagic", warrior.CastSleepMagic},
	{"script.ai.individual.Monster.WarriorBase.Warrior.WarriorCastSleepMagic.WarriorCastSleepMagicAggressive", warrior.CastSleepMagicAggressive},
	{"script.ai.individual.Monster.WarriorBase.Warrior.WarriorCastSplash.Agit02DoomTrooperAgit", warrior.Agit02DoomTrooperAgit},
	{"script.ai.individual.Monster.WarriorBase.Warrior.WarriorCastSplash.WarriorCastSplash", warrior.CastSplash},
	{"script.ai.individual.Monster.WarriorBase.Warrior.WarriorCastSplash.WarriorCastSplashAggressive", warrior.CastSplashAggressive},
}

// ind5eDefs are the batch's skills: the sleep a one-target magic of reach
// 900 with an effect of its own stack, the splash a self skill.
func ind5eDefs() []modelskill.Definition {
	return []modelskill.Definition{
		ind5dStacked(ind1Skill(ind5eSleep, 900, false), ind5eSleepStack, 1),
		ind1Skill(ind5eSplash, 0, true),
	}
}

// ind5eTemplate is a fixture monster of id listing the batch's sleep and
// splash by type.
func ind5eTemplate(id int32) *npc.Template {
	return ind5dTemplate(id, nil, map[npc.SkillType]modelskill.ID{
		npc.SkillTypeSleepMagic: ind5eSleep, npc.SkillTypeSelfRangeDDMagic: ind5eSplash,
	})
}

// bootIndividual5e boots the PR's classes after the Warrior chain beside
// templates and an unbound decoy monster, with extra and a director
// script to land effects through, every NPC casting the batch's skills,
// every draw 0 until a scenario queues others.
func bootIndividual5e(t *testing.T, extra raidBoot, templates ...*npc.Template) *ind1World {
	t.Helper()
	list, catalog := warriorScripts()
	for _, c := range ind5eBatch {
		list = append(list, script.Listing{Path: c.path})
		catalog[c.path] = c.ctor
	}
	const director = "test.Ind1Director"
	catalog[director] = func() script.Script { return script.Script{} }
	list = append(list, script.Listing{Path: director})
	templates = append(templates, warriorTemplate(unboundID, nil))
	kinds := map[int32]script.NPCKind{}
	for _, tmpl := range templates {
		kinds[int32(tmpl.ID)] = script.KindHostile
	}
	draws := &ind1Draws{}
	opts := append([]gameservertest.Option{
		gameservertest.WithScriptRand(draws.rand),
		gameservertest.WithNPCScripts(kinds, list, catalog),
		gameservertest.WithNpcSpawnSkills(modelskill.NewTable(ind5eDefs())),
		gameservertest.WithScriptSkills(ind5eDefs()...),
	}, extra.opts...)
	w := bootRaid(t, raidBoot{opts: opts, ready: extra.ready}, templates...)
	return &ind1World{warriorWorld: w, draws: draws, heard: &ind1Heard{}}
}

// ind5eHold is the weight an Agit02DoomTrooperAgit wants to attack by for
// amount dealt, damage or a spell's hate, scaled by scale: amount / its
// maximum HP / 0.05 * scale.
func ind5eHold(h *npc.Hostile, amount, scale float64) float64 {
	return amount / float64(h.MaxHP()) / 0.05 * scale
}

// ind5eWantHold fails unless h wants to attack the character with weight
// want, holding its ground.
func ind5eWantHold(t *testing.T, w *ind1World, when string, h *npc.Hostile, want float64) {
	t.Helper()
	d, ok := desire(h, actorai.IntentionAttack)
	if !ok || d.FinalTarget == nil || d.FinalTarget.ObjectID() != w.player.ObjectID() || d.Weight != want || d.MoveToTarget {
		t.Fatalf("%s: attack desire = %+v (%v), want weight %v on the character, holding its ground", when, d, ok, want)
	}
}

// TestCastSleepMagicSleepsTheOneItHatesMost ports WarriorCastSleepMagic's
// created, attacked and timer hooks, every draw 0. It appears with i_ai0,
// i_ai1 and i_ai2 at 0. The first hit starts the 5 s timer "2001" and
// marks i_ai0 and i_ai1. Fired while it attacks, the timer wants the sleep
// on the character it hates most and restarts; a hit by that character in
// between skips one firing, and a character holding the sleep's stack is
// not put to sleep again. Fired while idle, it clears i_ai1 and stops.
func TestCastSleepMagicSleepsTheOneItHatesMost(t *testing.T) {
	t.Parallel()
	w := bootIndividual5e(t, raidBoot{}, ind5eTemplate(ind5eSleepID))
	me := w.player.ObjectID()
	n := w.spawn(t, ind5eSleepID, 50, 0)
	for _, slot := range []script.IntSlot{script.SlotIntAI0, script.SlotIntAI1, script.SlotIntAI2} {
		if got := ind1Slot(n, slot); got != 0 {
			t.Fatalf("created slot %v = %d, want 0", slot, got)
		}
	}
	w.hit(t, n, 10)
	if a0, a1 := ind1Slot(n, script.SlotIntAI0), ind1Slot(n, script.SlotIntAI1); a0 != 1 || a1 != 1 {
		t.Fatalf("after the first hit i_ai0, i_ai1 = %d, %d, want 1, 1", a0, a1)
	}
	ind5bNoCast(t, "the first hit", n, ind5eSleep)
	w.ticks(t, 1)
	if got := n.AI().CurrentIntention(); got != actorai.IntentionAttack {
		t.Fatalf("setup: intention after the first hit = %v, want attack", got)
	}
	w.srv.Advance(t, 5*time.Second)
	ind5bWantCast(t, "the timer fired while attacking", n, ind5eSleep, me)

	w.ind5dClear(t, n)
	w.hit(t, n, 10)
	if got := ind1Slot(n, script.SlotIntAI2); got != 1 {
		t.Fatalf("i_ai2 after a hit by its most hated = %d, want 1", got)
	}
	ind5bNoCast(t, "a hit by its most hated", n, ind5eSleep)
	w.srv.Advance(t, 5*time.Second)
	ind5bNoCast(t, "the timer fired after a hit by its most hated", n, ind5eSleep)
	if got := ind1Slot(n, script.SlotIntAI2); got != 0 {
		t.Fatalf("i_ai2 after the timer = %d, want 0", got)
	}

	w.ind5dLand(t, modelskill.Ref{ID: ind5eSleep, Level: 1}, script.PlayerOf(w.player))
	w.ind5dClear(t, n)
	w.srv.Advance(t, 5*time.Second)
	ind5bNoCast(t, "the timer fired, the character holding the sleep's stack", n, ind5eSleep)

	idle := w.spawn(t, ind5eSleepID, -1500, 0)
	w.hit(t, idle, 10)
	if got := idle.AI().CurrentIntention(); got == actorai.IntentionAttack || got == actorai.IntentionCast {
		t.Fatalf("setup: intention of the idle one = %v, want neither attack nor cast", got)
	}
	w.srv.Advance(t, 5*time.Second)
	if got := ind1Slot(idle, script.SlotIntAI1); got != 0 {
		t.Fatalf("i_ai1 after the timer fired while idle = %d, want 0", got)
	}
	ind5bNoCast(t, "the timer fired while idle", idle, ind5eSleep)
}

// TestCastSleepMagicSleepsAnotherAttacker ports the rest of
// WarriorCastSleepMagic's attacked hook: hated less than a decoy, the
// character's second hit wants the sleep on it on a draw under 30 while
// the NPC holds over a tenth of its HP; not on a draw of 30, nor at a
// tenth.
func TestCastSleepMagicSleepsAnotherAttacker(t *testing.T) {
	t.Parallel()
	w := bootIndividual5e(t, raidBoot{}, ind5eTemplate(ind5eSleepID))
	me := w.player.ObjectID()
	n := w.spawn(t, ind5eSleepID, 1500, 0)
	w.ind5dDecoy(t, n)
	w.hit(t, n, 10)
	ind5bNoCast(t, "the first hit", n, ind5eSleep)
	w.hit(t, n, 10)
	ind5bWantCast(t, "a second hit, draw 0", n, ind5eSleep, me)

	w.ind5dClear(t, n)
	w.draws.set(30)
	w.hit(t, n, 10)
	ind5bNoCast(t, "a hit, draw 30", n, ind5eSleep)

	w.draws.set(0)
	w.onQueue(t, n, func() { n.SetCurrentHP(int(n.MaxHP()) / 10) })
	w.ind5dClear(t, n)
	w.hit(t, n, 1)
	ind5bNoCast(t, "a hit at a tenth of its HP", n, ind5eSleep)
}

// TestCastSleepMagicAggressiveAttacksOnSight ports
// WarriorCastSleepMagicAggressive's see-creature hook: it attacks the
// character it sees.
func TestCastSleepMagicAggressiveAttacksOnSight(t *testing.T) {
	t.Parallel()
	w := bootIndividual5e(t, raidBoot{}, ind5eTemplate(ind5eSleepAggrID))
	n := w.spawn(t, ind5eSleepAggrID, 150, 0)
	w.ticks(t, 1)
	if got := n.AggroHate(w.player); got <= 0 {
		t.Fatalf("hate of the character seen = %v, want it attacked", got)
	}
}

// TestCastSplashSplashesItsMostHatedCloseAttacker ports
// WarriorCastSplash's attacked hook, every draw 0: the character it hates
// most hitting it from 100 away wants the splash on itself, not on the
// first hit, nor on a draw of 33; from 300 away, never.
func TestCastSplashSplashesItsMostHatedCloseAttacker(t *testing.T) {
	t.Parallel()
	w := bootIndividual5e(t, raidBoot{}, ind5eTemplate(ind5eSplashID))
	near := w.spawn(t, ind5eSplashID, 100, 0)
	w.hit(t, near, 10)
	ind5bNoCast(t, "the first hit", near, ind5eSplash)
	w.draws.set(33)
	w.hit(t, near, 10)
	ind5bNoCast(t, "a hit from 100 away, draw 33", near, ind5eSplash)
	w.draws.set(0)
	w.hit(t, near, 10)
	ind5bWantCast(t, "a hit from 100 away by its most hated", near, ind5eSplash, near.ObjectID())

	far := w.spawn(t, ind5eSplashID, 0, 300)
	w.hit(t, far, 10)
	w.hit(t, far, 10)
	ind5bNoCast(t, "hits from 300 away", far, ind5eSplash)
}

// TestCastSplashSplashesItsClansCloseAttacker ports WarriorCastSplash's
// clan-attacked hook, every draw 0: a clan member settled past 7 AI
// cycles that hates the character most wants the splash on itself when
// the character hits another member from 100 away; one 300 away wants
// nothing.
func TestCastSplashSplashesItsClansCloseAttacker(t *testing.T) {
	t.Parallel()
	w := bootIndividual5e(t, raidBoot{}, ind5dClan(ind5eTemplate(ind5eSplashID)), ind5dClan(warriorTemplate(warriorID, nil)))
	victim := w.spawn(t, warriorID, 200, 0)
	near := w.spawn(t, ind5eSplashID, 100, 0)
	far := w.spawn(t, ind5eSplashID, 300, 0)
	w.ticks(t, 8)
	if near.LifeTime() <= 7 || far.LifeTime() <= 7 {
		t.Fatalf("clan member life times = %d, %d, want past 7", near.LifeTime(), far.LifeTime())
	}
	for _, h := range []*npc.Hostile{near, far} {
		w.onQueue(t, h, func() { h.AddDamageHate(w.player, 0, 100000) })
		w.ind5dClear(t, h)
	}
	w.hit(t, victim, 10)
	ind5bWantCast(t, "the member 100 away", near, ind5eSplash, near.ObjectID())
	ind5bNoCast(t, "the member 300 away", far, ind5eSplash)
}

// TestCastSplashAggressiveAttacksOnSight ports
// WarriorCastSplashAggressive's see-creature hook: it attacks the
// character it sees.
func TestCastSplashAggressiveAttacksOnSight(t *testing.T) {
	t.Parallel()
	w := bootIndividual5e(t, raidBoot{}, ind5eTemplate(ind5eSplashAggrID))
	n := w.spawn(t, ind5eSplashAggrID, 150, 0)
	w.ticks(t, 1)
	if got := n.AggroHate(w.player); got <= 0 {
		t.Fatalf("hate of the character seen = %v, want it attacked", got)
	}
}

// TestAgit02DoomTrooperAgitStaysWhereItIs ports Agit02DoomTrooperAgit's
// created and no-desire hooks: it spawns with flag 0, and with no desire
// left it does nothing, where a WarriorCastSplash beside it wanders.
func TestAgit02DoomTrooperAgitStaysWhereItIs(t *testing.T) {
	t.Parallel()
	w := bootIndividual5e(t, raidBoot{}, ind5eTemplate(ind5eTrooperID), ind5eTemplate(ind5eSplashID))
	trooper := w.spawn(t, ind5eTrooperID, 1500, 0)
	roamer := w.spawn(t, ind5eSplashID, -1500, 0)
	if got := ind1Slot(trooper, script.SlotFlag); got != 0 {
		t.Fatalf("created flag = %d, want 0", got)
	}
	w.ticks(t, 3)
	if got := roamer.AI().CurrentIntention(); got != actorai.IntentionWander {
		t.Fatalf("setup: the WarriorCastSplash intention = %v, want wander", got)
	}
	if got := trooper.AI().CurrentIntention(); got == actorai.IntentionWander {
		t.Fatal("the trooper wanders")
	}
	if d, ok := desire(trooper, actorai.IntentionWander); ok {
		t.Fatalf("the trooper queued a wander desire %+v", d)
	}
}

// TestAgit02DoomTrooperAgitHoldsAgainstItsAttacker ports
// Agit02DoomTrooperAgit's attacked hook: a hit of 100 wants it to attack
// the character where it stands, weight 100 / its maximum HP / 0.05 * 100.
// Once it wants to attack the
// character, a hit from 100 away on a draw under 10 of 33 wants the
// splash on itself; on a draw of 10, or from 300 away, not.
func TestAgit02DoomTrooperAgitHoldsAgainstItsAttacker(t *testing.T) {
	t.Parallel()
	w := bootIndividual5e(t, raidBoot{}, ind5eTemplate(ind5eTrooperID))
	n := w.spawn(t, ind5eTrooperID, 100, 0)
	w.hit(t, n, 100)
	ind5eWantHold(t, w, "the first hit", n, ind5eHold(n, 100, 100))
	ind5bNoCast(t, "the first hit", n, ind5eSplash)
	w.draws.set(10)
	w.hit(t, n, 100)
	ind5bNoCast(t, "a hit, draw 10", n, ind5eSplash)
	w.draws.set(0)
	w.hit(t, n, 100)
	ind5bWantCast(t, "a hit from 100 away by the one it wants to attack", n, ind5eSplash, n.ObjectID())

	far := w.spawn(t, ind5eTrooperID, 0, 300)
	w.hit(t, far, 100)
	w.hit(t, far, 100)
	ind5bNoCast(t, "hits from 300 away", far, ind5eSplash)
	ind5eWantHold(t, w, "two hits from 300 away", far, 2*ind5eHold(far, 100, 100))
}

// TestAgit02DoomTrooperAgitAnswersItsFlag ports Agit02DoomTrooperAgit's
// clan-attacked hook, every draw 0: a clan member sharing the hit
// trooper's flag, settled past 7 AI cycles and wanting to attack the
// character, 100 away, wants its splash cast at the character, which the
// self skill resolves to itself, and to attack it where it stands by the
// damage; a member of another flag answers nothing.
func TestAgit02DoomTrooperAgitAnswersItsFlag(t *testing.T) {
	t.Parallel()
	w := bootIndividual5e(t, raidBoot{}, ind5dClan(ind5eTemplate(ind5eTrooperID)), ind5dClan(ind5eTemplate(ind5eTrooperTwoID)))
	victim := w.spawn(t, ind5eTrooperID, 200, 0)
	member := w.spawn(t, ind5eTrooperTwoID, 100, 0)
	stranger := w.spawn(t, ind5eTrooperTwoID, 0, 100)
	w.ticks(t, 8)
	if member.LifeTime() <= 7 {
		t.Fatalf("member life time = %d, want past 7", member.LifeTime())
	}
	w.onQueue(t, stranger, func() { script.NPCOf(stranger).SetInt(script.SlotFlag, 1) })
	before := map[*npc.Hostile]float64{}
	for _, h := range []*npc.Hostile{member, stranger} {
		w.onQueue(t, h, func() { script.NPCOf(h).AddAttackDesireHold(script.PlayerOf(w.player), 100) })
		d, ok := desire(h, actorai.IntentionAttack)
		if !ok {
			t.Fatal("setup: a member wants to attack no one")
		}
		before[h] = d.Weight
	}
	w.hit(t, victim, 100)
	ind5bWantCast(t, "the member sharing the flag", member, ind5eSplash, member.ObjectID())
	ind5eWantHold(t, w, "the member sharing the flag", member, before[member]+ind5eHold(member, 100, 100))
	ind5bNoCast(t, "the member of another flag", stranger, ind5eSplash)
	ind5eWantHold(t, w, "the member of another flag", stranger, before[stranger])
}

// TestAgit02DoomTrooperAgitHoldsAgainstWhatItSees ports
// Agit02DoomTrooperAgit's see-creature and see-spell hooks: seeing the
// character it wants to attack it where it stands, weight 200; seeing it
// cast a self buff of 10 hate adds 10 / its maximum HP / 0.05 * 150.
func TestAgit02DoomTrooperAgitHoldsAgainstWhatItSees(t *testing.T) {
	t.Parallel()
	w := bootIndividual5e(t, ind1AggroSpellBoot(t), ind5eTemplate(ind5eTrooperID))
	n := w.spawn(t, ind5eTrooperID, 150, 0)
	w.ticks(t, 1)
	ind5eWantHold(t, w, "seeing the character", n, 200)
	w.castAggroBuff()
	w.srv.AdvanceUntil(t, "the spell's attack desire", func() bool {
		d, ok := desire(n, actorai.IntentionAttack)
		return ok && d.Weight != 200
	})
	ind5eWantHold(t, w, "after the spell", n, 200+ind5eHold(n, 10, 150))
}
