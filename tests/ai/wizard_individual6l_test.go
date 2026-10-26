package ai

import (
	"strconv"
	"sync/atomic"
	"testing"
	"time"

	actorai "github.com/fatal10110/acis_golang/internal/gameserver/model/actor/ai"
	"github.com/fatal10110/acis_golang/internal/gameserver/model/actor/npc"
	"github.com/fatal10110/acis_golang/internal/gameserver/model/location"
	modelskill "github.com/fatal10110/acis_golang/internal/gameserver/model/skill"
	"github.com/fatal10110/acis_golang/internal/gameserver/script"
	"github.com/fatal10110/acis_golang/internal/gameserver/script/ai/defaultnpc"
	"github.com/fatal10110/acis_golang/internal/gameserver/script/ai/monster"
	"github.com/fatal10110/acis_golang/internal/gameserver/script/ai/wizard"
	"github.com/fatal10110/acis_golang/internal/gameservertest"
)

// The NPC ids of the twelfth PR of the sixth individual behavior batch,
// one the datapack binds to each class.
const (
	i6lDDMagic2ID  = int32(21537) // WizardSaintDDMagic2
	i6lRangeAggrID = int32(21532) // WizardSaintRangeDDMagic2Aggressive
	i6lSelfID      = int32(21650) // WizardSaintSelfRangeDDMagic
	i6lSelfAggrID  = int32(21535) // WizardSaintSelfRangeDDMagicAggressive
)

// The fixture skills the PR's classes add to the earlier ones.
const (
	i6lMiddle    = modelskill.ID(9494) // W_MIDDLE_RANGE_DD_MAGIC
	i6lLong2     = modelskill.ID(9495) // W_LONG_RANGE_DD_MAGIC2
	i6lSelfRange = modelskill.ID(9496) // W_SELF_RANGE_DD_MAGIC
)

// individual6lScripts are the behavior chain down to WizardSaintBasic and
// the classes of the PR, in scripts.xml order.
func individual6lScripts() ([]script.Listing, script.Catalog) {
	const p = "script.ai.individual.Monster.WizardBase.Wizard.WizardSaintBasic."
	entries := []struct {
		path string
		ctor func() script.Script
	}{
		{"script.ai.individual.DefaultNpc", defaultnpc.New},
		{"script.ai.individual.Monster.MonsterAI", monster.New},
		{"script.ai.individual.Monster.WizardBase.WizardBase", wizard.Base},
		{"script.ai.individual.Monster.WizardBase.Wizard.Wizard", wizard.Wizard},
		{p + "WizardSaintBasic", wizard.SaintBasic},
		{p + "WizardSaintDDMagic2.WizardSaintDDMagic2", wizard.SaintDDMagic2},
		{p + "WizardSaintDDMagic2.WizardSaintRangeDDMagic2Aggressive", wizard.SaintRangeDDMagic2Aggressive},
		{p + "WizardSaintSelfRangeDDMagic.WizardSaintSelfRangeDDMagic", wizard.SaintSelfRangeDDMagic},
		{p + "WizardSaintSelfRangeDDMagic.WizardSaintSelfRangeDDMagicAggressive", wizard.SaintSelfRangeDDMagicAggressive},
	}
	var list []script.Listing
	catalog := script.Catalog{}
	for _, e := range entries {
		list = append(list, script.Listing{Path: e.path})
		catalog[e.path] = e.ctor
	}
	return list, catalog
}

// bootIndividual6l boots the PR's classes beside templates, every NPC
// casting i6lSkills, every draw the current value of the world's draw.
func bootIndividual6l(t *testing.T, templates ...*npc.Template) *individual6World {
	t.Helper()
	list, catalog := individual6lScripts()
	kinds := map[int32]script.NPCKind{}
	for _, tmpl := range templates {
		kinds[int32(tmpl.ID)] = script.KindHostile
	}
	draw := new(atomic.Int32)
	extra := raidBoot{opts: []gameservertest.Option{
		gameservertest.WithNPCScripts(kinds, list, catalog),
		gameservertest.WithScriptRand(func(n int) int { return min(int(draw.Load()), n-1) }),
		gameservertest.WithNpcSpawnSkills(i6lSkills()),
	}}
	return &individual6World{warriorWorld: bootRaid(t, extra, templates...), draw: draw}
}

// i6lTemplate is a fixture saint of id listing every skill type the PR's
// classes and their parent cast.
func i6lTemplate(id int32) *npc.Template {
	return i6bTemplate(id, nil, map[npc.SkillType]modelskill.ID{
		npc.SkillTypeWLongRangeDDMagic: wLongDDID, npc.SkillTypeWShortRangeDDMagic: wShortDDID,
		npc.SkillTypeDebuff: i6gDebuff, npc.SkillTypeBuff: i6kBuff,
		npc.SkillTypeWMiddleRangeDDMagic: i6lMiddle, npc.SkillTypeWLongRangeDDMagic2: i6lLong2,
		npc.SkillTypeWSelfRangeDDMagic: i6lSelfRange,
	})
}

// i6lSkills are the fixture skills of the PR's NPCs: a self buff and
// harmless one-target magics, each costing nothing, or 100000 MP for the
// costly ones.
func i6lSkills(costly ...modelskill.ID) *modelskill.Table {
	defs := []modelskill.Definition{npcSkill(i6kBuff, 1, false)}
	for _, id := range []modelskill.ID{wLongDDID, wShortDDID, i6gDebuff, i6lMiddle, i6lLong2, i6lSelfRange} {
		defs = append(defs, harmlessSkill(id))
	}
	for i := range defs {
		for _, id := range costly {
			if defs[i].ID == id {
				defs[i].MPConsume = 100000
			}
		}
	}
	return modelskill.NewTable(defs)
}

// i6lArmed spawns an NPC of id dx, dy from the character, its skills
// i6lSkills with costly ones, settled with empty desires and hate list.
func (w *individual6World) i6lArmed(t *testing.T, id int32, dx, dy int, costly ...modelskill.ID) *npc.Hostile {
	t.Helper()
	h := w.spawn(t, id, dx, dy)
	w.srv.InstallNPCCastSeam(t, h, i6lSkills(costly...))
	w.i6eSettled(t, h)
	return h
}

// i6lCastsUntil runs AI cycles, a second apart, until done holds for h,
// failing after 20 cycles.
func (w *individual6World) i6lCastsUntil(t *testing.T, h *npc.Hostile, what string, done func() bool) {
	t.Helper()
	for i := 0; !done(); i++ {
		if i == 20 {
			t.Fatalf("npc %d: %s never came", h.NpcID(), what)
		}
		w.ticks(t, 1)
		w.srv.Advance(t, time.Second)
		w.srv.ReadQueued(t, w.srv.Client)
	}
}

// TestWizardSaintDDMagic2StrikesFromAfar runs WizardSaintDDMagic2's
// attacked and skill-finished hooks for the character's hits, its parent
// answering each one twice. A hit from 300 away, while the saint hates the
// character, on a draw of 33, wants its long range DD magic, weight
// 1000000, beside its parent's short range DD magic twice; on a draw of 80,
// or from 50 away, no long range one. Once it gave up its spells a hit of
// 80 on the level 1 saint adds 80 / (1 + 7) * 100 = 1000 per answer, its
// own and its parent's two, with no cast. The end of a cast, with the
// character 300 away, brings its long range DD magic and, on a draw under
// 33, its debuff.
func TestWizardSaintDDMagic2StrikesFromAfar(t *testing.T) {
	t.Parallel()
	w := bootIndividual6l(t, i6lTemplate(i6lDDMagic2ID))
	me := w.player.ObjectID()

	w.draw.Store(33)
	far := w.i6lArmed(t, i6lDDMagic2ID, 300, 0)
	w.hit(t, far, 10)
	i6fWantCasts(t, "300 away, draw 33", far, map[modelskill.ID]i6fCast{wShortDDID: {me, 2000000, false}, wLongDDID: {me, 1000000, false}})

	near := w.i6lArmed(t, i6lDDMagic2ID, 50, 0)
	w.hit(t, near, 10)
	i6fWantCasts(t, "50 away, draw 33", near, map[modelskill.ID]i6fCast{wShortDDID: {me, 2000000, false}})

	w.draw.Store(80)
	calm := w.i6lArmed(t, i6lDDMagic2ID, 0, 300)
	w.hit(t, calm, 10)
	i6fWantCasts(t, "300 away, draw 80", calm, map[modelskill.ID]i6fCast{wShortDDID: {me, 2000000, false}})

	quit := w.i6lArmed(t, i6lDDMagic2ID, 0, -300)
	w.onQueue(t, quit, func() { quit.Scratch().SetInt(npc.IntAI0, 1) })
	w.hit(t, quit, 80)
	before := i6cAttackWeight(quit, me)
	w.hit(t, quit, 80)
	if got := i6cAttackWeight(quit, me); got < before+3000 {
		t.Fatalf("given up saint attack desire after another hit = %v, want at least %v", got, before+3000)
	}
	i6fWantCasts(t, "given up", quit, nil)

	w.draw.Store(0)
	w.i6lCastsUntil(t, calm, "the long range DD magic after a cast", func() bool {
		return castOn(calm, wLongDDID, me) && castOn(calm, i6gDebuff, me)
	})
}

// TestWizardSaintAggressivesOfThePRStrikeOnSight runs the see-creature
// hooks of WizardSaintRangeDDMagic2Aggressive and
// WizardSaintSelfRangeDDMagicAggressive. Settled in its territory with
// nobody on its hate list, each wants its short range DD magic on the
// character it sees 30 away and, 100 away, its long (the first) or middle
// (the second) range DD magic, weight 1000000, and hates it. One that
// cannot pay gives up its spells and wants to attack, weight 1000. Fresh,
// neither reacts.
func TestWizardSaintAggressivesOfThePRStrikeOnSight(t *testing.T) {
	t.Parallel()
	for _, c := range []struct {
		id  int32
		far modelskill.ID
	}{{i6lRangeAggrID, wLongDDID}, {i6lSelfAggrID, i6lMiddle}} {
		t.Run(strconv.Itoa(int(c.id)), func(t *testing.T) {
			t.Parallel()
			w := bootIndividual6l(t, i6lTemplate(c.id))
			me := w.player.ObjectID()
			for _, at := range []struct {
				dx    int
				skill modelskill.ID
			}{{-30, wShortDDID}, {-100, c.far}} {
				settled := w.i6lArmed(t, c.id, 1500, at.dx*10)
				w.ticks(t, 1)
				w.i6kLookFrom(t, settled, at.dx)
				i6fWantCasts(t, strconv.Itoa(at.dx), settled, map[modelskill.ID]i6fCast{at.skill: {me, 1000000, false}})
				if got := w.hateListOf(settled); got <= 0 {
					t.Fatalf("hate list entry of the character it saw %d away = %v, want some", -at.dx, got)
				}
				broke := w.i6lArmed(t, c.id, -1500, at.dx*10, at.skill)
				w.ticks(t, 1)
				w.i6kLookFrom(t, broke, at.dx)
				w.i6eGaveUp(t, broke, 1000)
			}

			fresh := w.spawn(t, c.id, 2500, 0)
			w.srv.InstallNPCCastSeam(t, fresh, i6lSkills())
			w.onQueue(t, fresh, func() { fresh.AI().Desires().Clear() })
			w.i6kLookFrom(t, fresh, -30)
			i6fWantCasts(t, "fresh", fresh, nil)
		})
	}
}

// TestWizardSaintRangeDDMagic2AggressiveAnswersFarHits runs
// WizardSaintRangeDDMagic2Aggressive's attacked hook for the character's
// hit from 300 away. While it did not act last on the character, on a draw under 33, it wants its second long range DD magic
// on the character, weight 1000000, beside its parent's long range DD
// magic and its grandparent's short range DD magic and debuff twice; on a
// draw of 33, or acting last on the character, or from 50 away, no second
// long range one.
func TestWizardSaintRangeDDMagic2AggressiveAnswersFarHits(t *testing.T) {
	t.Parallel()
	w := bootIndividual6l(t, i6lTemplate(i6lRangeAggrID))
	me := w.player.ObjectID()

	far := w.i6lArmed(t, i6lRangeAggrID, 300, 0)
	w.hit(t, far, 10)
	i6fWantCasts(t, "draw 0", far, map[modelskill.ID]i6fCast{
		i6lLong2: {me, 1000000, false}, wLongDDID: {me, 1000000, false},
		wShortDDID: {me, 2000000, false}, i6gDebuff: {me, 2000000, false},
	})

	near := w.i6lArmed(t, i6lRangeAggrID, 50, 0)
	w.hit(t, near, 10)
	i6fWantCasts(t, "50 away", near, map[modelskill.ID]i6fCast{wShortDDID: {me, 2000000, false}, i6gDebuff: {me, 2000000, false}})

	focused := w.i6lArmed(t, i6lRangeAggrID, 0, 300)
	w.onQueue(t, focused, func() { script.NPCOf(focused).SetTopDesireTarget(script.PlayerOf(w.player)) })
	w.hit(t, focused, 10)
	if _, ok := castDesireOf(focused, i6lLong2); ok {
		t.Fatal("saint acting last on the character wants its second long range DD magic")
	}

	w.draw.Store(33)
	calm := w.i6lArmed(t, i6lRangeAggrID, 0, -300)
	w.hit(t, calm, 10)
	i6fWantCasts(t, "draw 33", calm, map[modelskill.ID]i6fCast{wLongDDID: {me, 1000000, false}, wShortDDID: {me, 2000000, false}})
}

// TestWizardSaintSelfRangeDDMagicAnswersHits runs
// WizardSaintSelfRangeDDMagic's attacked hook for the character's hits, its
// parent answering each one twice. A hit from 30 away, while it did not
// act last on the character, on a draw under 33, wants its self
// range DD magic on itself, weight 1000000; short of MP for it the saint
// gives up its spells and wants to attack the character, not itself. A
// hit from 300 away, while it hates the character, on a draw under 80,
// wants its middle range DD magic on the character; on a draw of 80 none.
func TestWizardSaintSelfRangeDDMagicAnswersHits(t *testing.T) {
	t.Parallel()
	w := bootIndividual6l(t, i6lTemplate(i6lSelfID))
	me := w.player.ObjectID()

	adjacent := w.i6lArmed(t, i6lSelfID, 30, 0)
	w.hit(t, adjacent, 10)
	i6fWantCasts(t, "30 away", adjacent, map[modelskill.ID]i6fCast{
		i6lSelfRange: {adjacent.ObjectID(), 1000000, false}, wShortDDID: {me, 2000000, false}, i6gDebuff: {me, 2000000, false},
	})

	broke := w.i6lArmed(t, i6lSelfID, -30, 0, i6lSelfRange)
	w.hit(t, broke, 10)
	if got := broke.Scratch().Int(npc.IntAI0); got != 1 {
		t.Fatalf("saint short of MP int slot 0 = %d, want 1", got)
	}
	if got := i6cAttackWeight(broke, me); got < 1000 {
		t.Fatalf("saint short of MP attack desire on the character = %v, want at least 1000", got)
	}
	if got := i6cAttackWeight(broke, broke.ObjectID()); got != 0 {
		t.Fatalf("saint short of MP attack desire on itself = %v, want none", got)
	}
	i6fWantCasts(t, "short of MP", broke, nil)

	far := w.i6lArmed(t, i6lSelfID, 300, 0)
	w.hit(t, far, 10)
	i6fWantCasts(t, "300 away", far, map[modelskill.ID]i6fCast{
		i6lMiddle: {me, 1000000, false}, wShortDDID: {me, 2000000, false}, i6gDebuff: {me, 2000000, false},
	})

	w.draw.Store(80)
	calm := w.i6lArmed(t, i6lSelfID, 0, 300)
	w.hit(t, calm, 10)
	i6fWantCasts(t, "300 away, draw 80", calm, map[modelskill.ID]i6fCast{wShortDDID: {me, 2000000, false}})
}

// TestWizardSaintSelfRangeDDMagicWalksCastsAndRuns runs
// WizardSaintSelfRangeDDMagic's move-finished and skill-finished hooks.
// Arriving from a walk, with the character it hates 600 away, on a draw of
// 0 it wants its debuff and its middle range DD magic on the character,
// weight 1000000, and no short range one. The end of a cast with the
// character within 100 makes it want to run from the character, weight
// 1000000.
func TestWizardSaintSelfRangeDDMagicWalksCastsAndRuns(t *testing.T) {
	t.Parallel()
	w := bootIndividual6l(t, i6lTemplate(i6lSelfID))
	me := w.player.ObjectID()

	walker := w.spawn(t, i6lSelfID, 0, 600)
	w.srv.InstallNPCCastSeam(t, walker, i6lSkills())
	x, y, z := walker.Position()
	w.onQueue(t, walker, func() {
		walker.AI().Desires().Clear()
		walker.AI().Hates().Add(w.player, 100)
	})
	if !walker.AI().AddMoveToDesire(location.Location{X: x + 100, Y: y, Z: z}, 1e6) {
		t.Fatal("the walk was not queued")
	}
	w.i6eWalk(t, walker)
	if !castOn(walker, i6gDebuff, me) || !castOn(walker, i6lMiddle, me) {
		t.Fatal("saint arriving from a walk wants no debuff and middle range DD magic on its most hated")
	}
	if _, ok := castDesireOf(walker, wShortDDID); ok {
		t.Fatal("saint arriving from a walk 600 from its most hated wants its short range DD magic")
	}

	w.draw.Store(80)
	runner := w.i6lArmed(t, i6lSelfID, 50, 0)
	w.hit(t, runner, 10)
	w.i6lCastsUntil(t, runner, "the run from the character after a cast", func() bool {
		d, ok := desire(runner, actorai.IntentionFlee)
		return ok && d.Weight == 1000000 && d.FinalTarget != nil && d.FinalTarget.ObjectID() == me
	})
}
