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
	"github.com/fatal10110/acis_golang/internal/gameservertest"
)

// The NPC ids of the fifth individual behavior batch's fourth PR the
// tests spawn.
const (
	ind5dWalkerID      = int32(22217) // TyrannoPrimevalWalker
	ind5dDDSpecialID   = int32(22046) // WarriorCastDDMagicPhysicalSpecial
	ind5dDDSpecialAggr = int32(21308) // WarriorCastDDMagicPhysicalSpecialAggressive
	ind5dDDSpecialTwo  = int32(21392) // WarriorCastDDMagicPhysicalSpecialAggressive
	ind5dBuffAggrID    = int32(21384) // WarriorCastDDMagicPhysicalSpecialBuffAggressive
	ind5dBuffAggrTwo   = int32(21643) // WarriorCastDDMagicPhysicalSpecialBuffAggressive
	ind5dPoisonID      = int32(21316) // WarriorCastEnchantPhysicalPoisonAggressive
	ind5dPoisonDDID    = int32(21321) // WarriorCastDDMagicEnchantPhysicalPoisonAggressive
	ind5dPoisonRangeID = int32(21319) // WarriorCastEnchantRangePhysicalPoisonAggressive
)

// The skills the batch's fixture NPCs list by type, and the markers the
// tests land to stand for a held debuff.
const (
	ind5dLongRange    = modelskill.ID(9520) // W_LONG_RANGE_DD_MAGIC
	ind5dRangeStrike  = modelskill.ID(9521) // PHYSICAL_SPECIAL_RANGE
	ind5dBuff         = modelskill.ID(9522) // BUFF
	ind5dBuff1        = modelskill.ID(9523) // BUFF1
	ind5dBuff2        = modelskill.ID(9524) // BUFF2
	ind5dStrike       = modelskill.ID(9525) // PHYSICAL_SPECIAL
	ind5dDebuff1      = modelskill.ID(9526) // DEBUFF1
	ind5dDebuff2      = modelskill.ID(9527) // DEBUFF2
	ind5dDDMagic      = modelskill.ID(9528) // DD_MAGIC
	ind5dRangePS      = modelskill.ID(9529) // RANGE_PHYSICAL_SPECIAL, 60 MP
	ind5dCostlyStrike = modelskill.ID(9530) // PHYSICAL_SPECIAL, 50 MP
	ind5dDebuff1Mark  = modelskill.ID(9531) // the first debuff's stack, its level the stack order
)

// The stack types of the batch's buffs and debuffs.
const (
	ind5dBuffStack    = "ind5d_buff"
	ind5dBuff1Stack   = "ind5d_buff1"
	ind5dBuff2Stack   = "ind5d_buff2"
	ind5dDebuff1Stack = "ind5d_debuff1"
	ind5dDebuff2Stack = "ind5d_debuff2"
)

// ind5dBatch are the classes of the PR, in scripts.xml order.
var ind5dBatch = []struct {
	path string
	ctor func() script.Script
}{
	{"script.ai.individual.Monster.WarriorBase.Warrior.TyrannoPrimeval.TyrannoPrimevalWalker", warrior.TyrannoPrimevalWalker},
	{"script.ai.individual.Monster.WarriorBase.Warrior.WarriorCastDDMagicPhysicalSpecial.WarriorCastDDMagicPhysicalSpecial", warrior.CastDDMagicPhysicalSpecial},
	{"script.ai.individual.Monster.WarriorBase.Warrior.WarriorCastDDMagicPhysicalSpecial.WarriorCastDDMagicPhysicalSpecialAggressive", warrior.CastDDMagicPhysicalSpecialAggressive},
	{"script.ai.individual.Monster.WarriorBase.Warrior.WarriorCastDDMagicPhysicalSpecial.WarriorCastDDMagicPhysicalSpecialBuffAggressive", warrior.CastDDMagicPhysicalSpecialBuffAggressive},
	{"script.ai.individual.Monster.WarriorBase.Warrior.WarriorCastEnchantPhysicalPoison.WarriorCastDDMagicEnchantPhysicalPoisonAggressive", warrior.CastDDMagicEnchantPhysicalPoisonAggressive},
	{"script.ai.individual.Monster.WarriorBase.Warrior.WarriorCastEnchantPhysicalPoison.WarriorCastEnchantPhysicalPoison", warrior.CastEnchantPhysicalPoison},
	{"script.ai.individual.Monster.WarriorBase.Warrior.WarriorCastEnchantPhysicalPoison.WarriorCastEnchantPhysicalPoisonAggressive", warrior.CastEnchantPhysicalPoisonAggressive},
	{"script.ai.individual.Monster.WarriorBase.Warrior.WarriorCastEnchantPhysicalPoison.WarriorCastEnchantRangePhysicalPoisonAggressive", warrior.CastEnchantRangePhysicalPoisonAggressive},
}

// ind5dStacked gives d one effect of stack type stack, lasting a minute,
// its stack order order.
func ind5dStacked(d modelskill.Definition, stack string, order float64) modelskill.Definition {
	d.Effects = []modelskill.EffectTemplate{{Name: "Buff", Time: 60, Icon: true, StackType: stack, StackOrder: order}}
	return d
}

// ind5dDefs are the batch's skills: the magic and strikes one-target
// attacks of reach 900, the first buff a one-target buff of reach 600 and
// the other two self buffs, each buff and debuff with an effect of its
// own stack; the first debuff also at levels 2 and 10, the ranged physical
// special costing 60 MP and the costly strike 50; the marker a self buff of
// the first debuff's stack at levels 1 and 10, its stack order its level.
func ind5dDefs() []modelskill.Definition {
	buff := ind1Skill(ind5dBuff, 600, false)
	buff.Offensive, buff.Magic, buff.SkillType, buff.Power = false, false, "BUFF", 0
	defs := []modelskill.Definition{
		ind1Skill(ind5dLongRange, 900, false), ind1Skill(ind5dRangeStrike, 900, false), ind1Skill(ind5dStrike, 900, false),
		ind1Skill(ind5dDDMagic, 900, false),
		ind5dStacked(buff, ind5dBuffStack, 1),
		ind5dStacked(ind1Skill(ind5dBuff1, 0, true), ind5dBuff1Stack, 1),
		ind5dStacked(ind1Skill(ind5dBuff2, 0, true), ind5dBuff2Stack, 1),
		ind5dStacked(ind1Skill(ind5dDebuff2, 900, false), ind5dDebuff2Stack, 1),
	}
	for _, level := range []int{1, 2, 10} {
		d := ind5dStacked(ind1Skill(ind5dDebuff1, 900, false), ind5dDebuff1Stack, 1)
		d.Level = level
		defs = append(defs, d)
	}
	for _, level := range []int{1, 10} {
		d := ind5dStacked(ind1Skill(ind5dDebuff1Mark, 0, true), ind5dDebuff1Stack, float64(level))
		d.Level = level
		defs = append(defs, d)
	}
	rangePS := ind1Skill(ind5dRangePS, 900, false)
	rangePS.MPConsume = 60
	costly := ind1Skill(ind5dCostlyStrike, 900, false)
	costly.MPConsume = 50
	return append(defs, rangePS, costly)
}

// ind5dTemplate is a fixture monster of id with ai listing the skills of
// types by type, at level 1.
func ind5dTemplate(id int32, ai npc.AIParams, types map[npc.SkillType]modelskill.ID) *npc.Template {
	tmpl := warriorTemplate(id, ai)
	tmpl.SkillsByType = map[npc.SkillType]modelskill.Ref{}
	for t, skill := range types {
		tmpl.SkillsByType[t] = modelskill.Ref{ID: skill, Level: 1}
	}
	return tmpl
}

// ind5dClan puts tmpl in the scenario clan, of range 500.
func ind5dClan(tmpl *npc.Template) *npc.Template {
	tmpl.Clans, tmpl.ClanRange = []string{"ind5d_clan"}, 500
	return tmpl
}

// ind5dDDSpecialTypes are the skills of the DD magic physical specials.
var ind5dDDSpecialTypes = map[npc.SkillType]modelskill.ID{
	npc.SkillTypeWLongRangeDDMagic: ind5dLongRange, npc.SkillTypePhysicalSpecialRange: ind5dRangeStrike, npc.SkillTypeBuff: ind5dBuff,
}

// ind5dPoisonTypes are the skills of the physical poisons.
var ind5dPoisonTypes = map[npc.SkillType]modelskill.ID{
	npc.SkillTypeBuff1: ind5dBuff1, npc.SkillTypeBuff2: ind5dBuff2, npc.SkillTypePhysicalSpecial: ind5dStrike,
	npc.SkillTypeDebuff1: ind5dDebuff1, npc.SkillTypeDebuff2: ind5dDebuff2, npc.SkillTypeDDMagic: ind5dDDMagic,
}

// bootIndividual5d boots the PR's classes after the Warrior chain beside
// templates and an unbound decoy monster, with extra and a director
// script to land effects through, every NPC casting the batch's skills,
// every draw 0 until a scenario queues others.
func bootIndividual5d(t *testing.T, extra raidBoot, templates ...*npc.Template) *ind1World {
	t.Helper()
	list, catalog := warriorScripts()
	for _, c := range ind5dBatch {
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
		gameservertest.WithNpcSpawnSkills(modelskill.NewTable(ind5dDefs())),
		gameservertest.WithScriptSkills(ind5dDefs()...),
	}, extra.opts...)
	w := bootRaid(t, raidBoot{opts: opts, ready: extra.ready}, templates...)
	return &ind1World{warriorWorld: w, draws: draws, heard: &ind1Heard{}}
}

// ind5dDecoy spawns the unbound monster far from the character and makes
// h hate it most.
func (w *ind1World) ind5dDecoy(t *testing.T, h *npc.Hostile) {
	t.Helper()
	decoy := w.spawn(t, unboundID, 0, 1500)
	w.onQueue(t, h, func() { h.AddDamageHate(decoy, 0, 100000) })
}

// ind5dClear drops h's queued desires.
func (w *ind1World) ind5dClear(t *testing.T, h *npc.Hostile) {
	t.Helper()
	w.onQueue(t, h, func() { h.AI().Desires().Clear() })
}

// ind5dLand lands the effects of skill ref on target, from target itself,
// and fails unless target then holds ref's stack at least at level 0.
func (w *ind1World) ind5dLand(t *testing.T, ref modelskill.Ref, target script.Creature) {
	t.Helper()
	me := w.player.ObjectID()
	if !w.srv.RunScript(t, me, "Ind1Director", func(s *script.Script, _ *script.Player) {
		s.ApplyEffects(ref, target, target)
	}) {
		t.Fatal("the effect landing panicked")
	}
	w.srv.AdvanceUntil(t, "the effect landed", func() bool {
		level := int32(-1)
		w.srv.RunScript(t, me, "Ind1Director", func(s *script.Script, _ *script.Player) {
			level = s.AbnormalLevel(target, ref)
		})
		return level >= 0
	})
}

// ind5dWant fails unless h's queued cast desires are exactly want, by
// skill id: each on the creature of the object id, weight 1000000.
func ind5dWant(t *testing.T, w *ind1World, when string, h *npc.Hostile, want map[modelskill.ID]int32) {
	t.Helper()
	casts := map[modelskill.ID]ind1Cast{}
	for id, target := range want {
		casts[id] = ind1Cast{target, 1000000}
	}
	w.wantCasts(t, when, h, casts)
}

// TestTyrannoPrimevalWalkerWalksItsRoute ports TyrannoPrimevalWalker's
// created hook: after TyrannoPrimeval's (its chance multiplier i_ai3 set
// to 1), it wants to walk the route its SuperPointName parameter names,
// with weight 50.
func TestTyrannoPrimevalWalkerWalksItsRoute(t *testing.T) {
	t.Parallel()
	w := bootIndividual5d(t, raidBoot{}, warriorTemplate(ind5dWalkerID, npc.AIParams{"SuperPointName": "lost_06"}))
	n := w.spawn(t, ind5dWalkerID, 1500, 0)
	if a3 := ind1Slot(n, script.SlotIntAI3); a3 != 1 {
		t.Fatalf("created i_ai3 = %d, want 1", a3)
	}
	d, ok := desire(n, actorai.IntentionMoveRoute)
	if !ok || d.RouteName != "lost_06" || d.Weight != 50 {
		t.Fatalf("route desire once created = %+v (%v), want lost_06 with weight 50", d, ok)
	}
}

// TestCastDDMagicPhysicalSpecialAnswersHitsByRange ports
// WarriorCastDDMagicPhysicalSpecial's attacked hook, every draw 0. Hit
// from 300 away, it wants nothing while it hates no one, then its long
// range magic on the character it hates most. Hit from 100 away while it
// hates a decoy most, it wants its ranged physical special on the
// character, unless the draw is 33.
func TestCastDDMagicPhysicalSpecialAnswersHitsByRange(t *testing.T) {
	t.Parallel()
	w := bootIndividual5d(t, raidBoot{}, ind5dTemplate(ind5dDDSpecialID, nil, ind5dDDSpecialTypes))
	me := w.player.ObjectID()
	far := w.spawn(t, ind5dDDSpecialID, 300, 0)
	w.hit(t, far, 10)
	ind5dWant(t, w, "a first hit from 300 away", far, nil)
	w.hit(t, far, 10)
	ind5dWant(t, w, "a hit from 300 away by its most hated", far, map[modelskill.ID]int32{ind5dLongRange: me})

	near := w.spawn(t, ind5dDDSpecialID, 0, 100)
	w.ind5dDecoy(t, near)
	w.draws.set(33)
	w.hit(t, near, 10)
	ind5dWant(t, w, "a hit from 100 away, its draw 33", near, nil)
	w.draws.set(0)
	w.hit(t, near, 10)
	ind5dWant(t, w, "a hit from 100 away by one it does not hate most", near, map[modelskill.ID]int32{ind5dRangeStrike: me})
}

// TestCastDDMagicPhysicalSpecialStrikesItsClansCloseAttacker ports
// WarriorCastDDMagicPhysicalSpecial's clan-attacked hook, every draw 0:
// a clan member settled past 7 AI cycles that hates a decoy most wants
// its ranged physical special on the character hitting another member
// from 100 away; one 300 away from the character wants nothing.
func TestCastDDMagicPhysicalSpecialStrikesItsClansCloseAttacker(t *testing.T) {
	t.Parallel()
	w := bootIndividual5d(t, raidBoot{}, ind5dClan(ind5dTemplate(ind5dDDSpecialID, nil, ind5dDDSpecialTypes)), ind5dClan(warriorTemplate(warriorID, nil)))
	victim := w.spawn(t, warriorID, 200, 0)
	near := w.spawn(t, ind5dDDSpecialID, 100, 0)
	far := w.spawn(t, ind5dDDSpecialID, 300, 0)
	w.ticks(t, 8)
	if near.LifeTime() <= 7 || far.LifeTime() <= 7 {
		t.Fatalf("clan member life times = %d, %d, want past 7", near.LifeTime(), far.LifeTime())
	}
	w.ind5dDecoy(t, near)
	w.onQueue(t, far, func() { far.AddDamageHate(near, 0, 100000) })

	w.hit(t, victim, 10)
	ind5dWant(t, w, "the member 100 away", near, map[modelskill.ID]int32{ind5dRangeStrike: w.player.ObjectID()})
	ind5dWant(t, w, "the member 300 away", far, nil)
}

// TestCastDDMagicPhysicalSpecialCastsLongRangeAfterACast ports
// WarriorCastDDMagicPhysicalSpecial's skill-finished hook: once its long
// range magic on the character it hates most, 300 away, ends, it wants
// that magic on the character again, its queue emptied while it cast.
func TestCastDDMagicPhysicalSpecialCastsLongRangeAfterACast(t *testing.T) {
	t.Parallel()
	tmpl := ind5dTemplate(ind5dDDSpecialID, nil, ind5dDDSpecialTypes)
	tmpl.CanMove = false
	w := bootIndividual5d(t, raidBoot{}, tmpl)
	n := w.spawn(t, ind5dDDSpecialID, 300, 0)
	w.hit(t, n, 10)
	w.hit(t, n, 10)
	var frames [][]byte
	for i := 0; len(castsOf(frames, n, ind5dLongRange)) == 0; i++ {
		if i == 40 {
			t.Fatal("the long range magic did not start within 40 AI cycles")
		}
		frames = append(frames, w.ticks(t, 1)...)
		w.srv.Advance(t, 500*time.Millisecond)
		frames = append(frames, w.srv.ReadQueued(t, w.srv.Client)...)
	}
	w.ind5dClear(t, n)
	ind5bRunUntil(t, w, "the long range magic wanted again", func() bool {
		_, ok := ind1Casts(n)[ind5dLongRange]
		return ok
	})
	ind5bWantCast(t, "after the cast", n, ind5dLongRange, w.player.ObjectID())
}

// TestCastDDMagicPhysicalSpecialAggressiveSightings ports
// WarriorCastDDMagicPhysicalSpecialAggressive's see-creature hook, every
// draw 0. One that sees the character before its eighth AI cycle ignores
// it. One that first sees it once settled, wanting no one, in its
// territory, wants its long range magic on it from 300 away and attacks
// it.
func TestCastDDMagicPhysicalSpecialAggressiveSightings(t *testing.T) {
	t.Parallel()
	early := ind5dTemplate(ind5dDDSpecialAggr, nil, ind5dDDSpecialTypes)
	early.CanMove = false
	settled := ind5dTemplate(ind5dDDSpecialTwo, nil, ind5dDDSpecialTypes)
	settled.CanMove = false
	w := bootIndividual5d(t, raidBoot{}, early, settled)
	e := w.spawn(t, ind5dDDSpecialAggr, 150, 0)
	w.ticks(t, 1)
	if got := e.AggroHate(w.player); got != 0 {
		t.Fatalf("hate of the character seen before settling = %v, want none", got)
	}
	ind5dWant(t, w, "seeing the character before settling", e, nil)

	n := w.spawn(t, ind5dDDSpecialTwo, 1500, 0)
	w.ticks(t, 8)
	if n.LifeTime() <= 7 {
		t.Fatalf("life time = %d, want past 7", n.LifeTime())
	}
	w.moveCharacter(t, n, -300, 0)
	w.ticks(t, 1)
	ind5bWantCast(t, "seeing the character 300 away once settled", n, ind5dLongRange, w.player.ObjectID())
	if got := n.AggroHate(w.player); got <= 0 {
		t.Fatalf("hate of the character seen once settled = %v, want it attacked", got)
	}
}

// TestCastDDMagicPhysicalSpecialBuffAggressiveKeepsItsBuff ports
// WarriorCastDDMagicPhysicalSpecialBuffAggressive's created, see-creature,
// attacked and clan-attacked hooks, every draw 0. It appears wanting its
// buff on itself. Seeing the character while idle, it wants the buff on
// itself and attacks; one seeing a character that holds the buff's stack
// does not want it. Hit while idle, it wants the buff on itself until it
// holds it. Settled and done buffing itself on sight, it wants the buff
// on a clan member hit by the character.
func TestCastDDMagicPhysicalSpecialBuffAggressiveKeepsItsBuff(t *testing.T) {
	t.Parallel()
	one := ind5dClan(ind5dTemplate(ind5dBuffAggrID, nil, ind5dDDSpecialTypes))
	one.CanMove = false
	two := ind5dTemplate(ind5dBuffAggrTwo, nil, ind5dDDSpecialTypes)
	two.CanMove = false
	w := bootIndividual5d(t, raidBoot{}, one, two, ind5dClan(warriorTemplate(warriorID, nil)))
	player := script.PlayerOf(w.player)
	buff := modelskill.Ref{ID: ind5dBuff, Level: 1}

	seer := w.spawn(t, ind5dBuffAggrTwo, 150, 0)
	ind5dWant(t, w, "once created", seer, map[modelskill.ID]int32{ind5dBuff: seer.ObjectID()})
	w.ind5dClear(t, seer)
	w.ticks(t, 1)
	ind5bWantCast(t, "seeing the character", seer, ind5dBuff, seer.ObjectID())
	if got := seer.AggroHate(w.player); got <= 0 {
		t.Fatalf("hate of the character seen = %v, want it attacked", got)
	}

	w.ind5dLand(t, buff, player)
	blind := w.spawn(t, ind5dBuffAggrTwo, -150, 0)
	w.ind5dClear(t, blind)
	w.ticks(t, 1)
	ind5bNoCast(t, "seeing a character holding the buff's stack", blind, ind5dBuff)

	hit := w.spawn(t, ind5dBuffAggrTwo, 0, 300)
	w.ind5dClear(t, hit)
	w.hit(t, hit, 10)
	ind5dWant(t, w, "hit while idle", hit, map[modelskill.ID]int32{ind5dBuff: hit.ObjectID()})
	w.ind5dLand(t, buff, script.NPCOf(hit))
	w.ind5dClear(t, hit)
	w.hit(t, hit, 10)
	ind5bNoCast(t, "hit while holding the buff", hit, ind5dBuff)

	victim := w.spawn(t, warriorID, 1500, 0)
	member := w.spawn(t, ind5dBuffAggrID, 1500, 100)
	w.ticks(t, 8)
	if member.LifeTime() <= 7 {
		t.Fatalf("clan member life time = %d, want past 7", member.LifeTime())
	}
	ind5bRunUntil(t, w, "the clan member's buffs on sight", func() bool {
		return len(ind1Casts(member)) == 0 && member.AI().CurrentIntention() != actorai.IntentionCast
	})
	w.ind5dClear(t, member)
	w.hit(t, victim, 10)
	ind5bWantCast(t, "its clan member hit", member, ind5dBuff, victim.ObjectID())
}

// TestCastEnchantPhysicalPoisonBuffsStrikesAndPoisons ports
// WarriorCastEnchantPhysicalPoison's created and attacked hooks through
// its aggressive child. It appears wanting both buffs on itself. A hit
// with every draw 0 wants both buffs again, its physical special and both
// debuffs on the character; held, the buffs are not wanted, and with the
// first debuff's stack held at level 1 it wants that debuff a level
// deeper, at level 10 not at all. Draws of 33 want nothing.
func TestCastEnchantPhysicalPoisonBuffsStrikesAndPoisons(t *testing.T) {
	t.Parallel()
	w := bootIndividual5d(t, raidBoot{}, ind5dTemplate(ind5dPoisonID, nil, ind5dPoisonTypes))
	me, player := w.player.ObjectID(), script.PlayerOf(w.player)
	n := w.spawn(t, ind5dPoisonID, 1500, 0)
	self := n.ObjectID()
	ind5dWant(t, w, "once created", n, map[modelskill.ID]int32{ind5dBuff1: self, ind5dBuff2: self})

	w.ind5dClear(t, n)
	w.hit(t, n, 10)
	ind5dWant(t, w, "a hit", n, map[modelskill.ID]int32{
		ind5dBuff1: self, ind5dBuff2: self, ind5dStrike: me, ind5dDebuff1: me, ind5dDebuff2: me,
	})

	w.ind5dLand(t, modelskill.Ref{ID: ind5dBuff1, Level: 1}, script.NPCOf(n))
	w.ind5dLand(t, modelskill.Ref{ID: ind5dBuff2, Level: 1}, script.NPCOf(n))
	w.ind5dLand(t, modelskill.Ref{ID: ind5dDebuff1Mark, Level: 1}, player)
	w.ind5dClear(t, n)
	w.hit(t, n, 10)
	ind5dWant(t, w, "a hit, the buffs and the first debuff's stack held", n, map[modelskill.ID]int32{ind5dStrike: me, ind5dDebuff1: me, ind5dDebuff2: me})
	if got := ind1Casts(n)[ind5dDebuff1].Skill.Level; got != 2 {
		t.Fatalf("first debuff level wanted over a held level 1 = %d, want 2", got)
	}

	w.ind5dLand(t, modelskill.Ref{ID: ind5dDebuff1Mark, Level: 10}, player)
	w.ind5dClear(t, n)
	w.hit(t, n, 10)
	ind5dWant(t, w, "a hit, the first debuff's stack held at level 10", n, map[modelskill.ID]int32{ind5dStrike: me, ind5dDebuff2: me})

	w.draws.set(33)
	w.ind5dClear(t, n)
	w.hit(t, n, 10)
	ind5dWant(t, w, "a hit, every draw 33", n, nil)
}

// TestCastEnchantPhysicalPoisonPoisonsItsClansAttackerAndCasters ports
// WarriorCastEnchantPhysicalPoison's clan-attacked and see-spell hooks,
// every draw 0: a clan member's attacker, then a character it sees cast,
// gets both debuffs wanted on it.
func TestCastEnchantPhysicalPoisonPoisonsItsClansAttackerAndCasters(t *testing.T) {
	t.Parallel()
	boot := ind1AggroSpellBoot(t)
	w := bootIndividual5d(t, boot, ind5dClan(ind5dTemplate(ind5dPoisonID, nil, ind5dPoisonTypes)), ind5dClan(warriorTemplate(warriorID, nil)))
	me := w.player.ObjectID()
	victim := w.spawn(t, warriorID, 1500, 0)
	member := w.spawn(t, ind5dPoisonID, 1500, 100)
	w.ind5dClear(t, member)
	w.hit(t, victim, 10)
	ind5dWant(t, w, "its clan member hit", member, map[modelskill.ID]int32{ind5dDebuff1: me, ind5dDebuff2: me})

	seer := w.spawn(t, ind5dPoisonID, 150, 0)
	w.ticks(t, 1)
	w.ind5dClear(t, seer)
	w.castAggroBuff()
	w.srv.AdvanceUntil(t, "the debuff desires", func() bool { return len(ind1Casts(seer)) == 2 })
	ind5bWantCast(t, "after the spell", seer, ind5dDebuff1, me)
	ind5bWantCast(t, "after the spell", seer, ind5dDebuff2, me)
}

// TestCastDDMagicEnchantPhysicalPoisonCastsAtFarAttackers ports
// WarriorCastDDMagicEnchantPhysicalPoisonAggressive's attacked and
// clan-attacked hooks, every draw 0. Settled and not attacking, it wants
// its DD magic on the character hitting a clan member from far away. Hit
// from 300 away by the character it hates most it wants the magic on it,
// not on the first hit, nor on a hit from 50 away.
func TestCastDDMagicEnchantPhysicalPoisonCastsAtFarAttackers(t *testing.T) {
	t.Parallel()
	w := bootIndividual5d(t, raidBoot{}, ind5dClan(ind5dTemplate(ind5dPoisonDDID, nil, ind5dPoisonTypes)), ind5dClan(warriorTemplate(warriorID, nil)))
	me := w.player.ObjectID()
	victim := w.spawn(t, warriorID, 1500, 0)
	member := w.spawn(t, ind5dPoisonDDID, 1500, 100)
	w.ticks(t, 8)
	if member.LifeTime() <= 7 || member.AI().CurrentIntention() == actorai.IntentionAttack {
		t.Fatalf("clan member life time %d, intention %v, want past 7 and not attacking", member.LifeTime(), member.AI().CurrentIntention())
	}
	w.ind5dClear(t, member)
	w.hit(t, victim, 10)
	ind5bWantCast(t, "its clan member hit from far away", member, ind5dDDMagic, me)

	far := w.spawn(t, ind5dPoisonDDID, -1500, 0)
	w.ind5dClear(t, far)
	w.moveCharacter(t, far, -300, 0)
	w.hit(t, far, 10)
	ind5bNoCast(t, "a first hit from 300 away", far, ind5dDDMagic)
	w.hit(t, far, 10)
	ind5bWantCast(t, "a hit from 300 away by its most hated", far, ind5dDDMagic, me)

	w.moveCharacter(t, far, -50, 0)
	w.ind5dClear(t, far)
	w.hit(t, far, 10)
	ind5bNoCast(t, "a hit from 50 away by its most hated", far, ind5dDDMagic)
}

// TestCastEnchantRangePhysicalPoisonPicksWhatItCanCast ports
// WarriorCastEnchantRangePhysicalPoisonAggressive's attacked hook. Hit by
// the character while it hates a decoy most, on a draw of 0 (the parent's
// draws 99), it wants its ranged physical special (60 MP) at 100 MP, its
// physical special (50 MP) at 55, and at 10 neither: it marks i_ai0 and
// wants to attack the character.
func TestCastEnchantRangePhysicalPoisonPicksWhatItCanCast(t *testing.T) {
	t.Parallel()
	tmpl := ind5dTemplate(ind5dPoisonRangeID, nil, map[npc.SkillType]modelskill.ID{
		npc.SkillTypeRangePhysicalSpecial: ind5dRangePS, npc.SkillTypePhysicalSpecial: ind5dCostlyStrike,
	})
	tmpl.MPMax = 100
	w := bootIndividual5d(t, raidBoot{}, tmpl)
	me := w.player.ObjectID()
	n := w.spawn(t, ind5dPoisonRangeID, 1500, 0)
	w.ind5dDecoy(t, n)
	for _, row := range []struct {
		mp   int
		want map[modelskill.ID]int32
	}{
		{100, map[modelskill.ID]int32{ind5dRangePS: me}},
		{55, map[modelskill.ID]int32{ind5dCostlyStrike: me}},
		{10, nil},
	} {
		w.ind5dClear(t, n)
		w.onQueue(t, n, func() { n.SetCurrentMP(row.mp) })
		w.draws.set(99, 0)
		w.hit(t, n, 10)
		ind5dWant(t, w, fmt.Sprintf("a hit at %d MP", row.mp), n, row.want)
	}
	if got := ind1Slot(n, script.SlotIntAI0); got != 1 {
		t.Fatalf("i_ai0 with neither castable = %d, want 1", got)
	}
	if d, ok := desire(n, actorai.IntentionAttack); !ok || d.FinalTarget == nil || d.FinalTarget.ObjectID() != me || d.Weight < 1000 {
		t.Fatalf("attack desire with neither castable = %+v (%v), want at least 1000 on the character", d, ok)
	}
}
