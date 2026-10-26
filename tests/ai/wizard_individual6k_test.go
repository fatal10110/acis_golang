package ai

import (
	"strconv"
	"sync/atomic"
	"testing"
	"time"

	actorai "github.com/fatal10110/acis_golang/internal/gameserver/model/actor/ai"
	"github.com/fatal10110/acis_golang/internal/gameserver/model/actor/npc"
	modelskill "github.com/fatal10110/acis_golang/internal/gameserver/model/skill"
	"github.com/fatal10110/acis_golang/internal/gameserver/script"
	"github.com/fatal10110/acis_golang/internal/gameserver/script/ai/defaultnpc"
	"github.com/fatal10110/acis_golang/internal/gameserver/script/ai/monster"
	"github.com/fatal10110/acis_golang/internal/gameserver/script/ai/wizard"
	"github.com/fatal10110/acis_golang/internal/gameservertest"
)

// The NPC ids of the eleventh PR of the sixth individual behavior batch:
// the datapack ids each class binds (WizardFiendArcher and
// WizardFiendArcherSelfBuff bind none; their children stand for them).
const (
	i6kTeleportID     = int32(21588) // WizardCorpseVampireTeleport
	i6kStep2ID        = int32(21453) // WizardGrowthStep2
	i6kFiendAggrID    = int32(20819) // WizardFiendArcherAggressive
	i6kFiendSelfAggID = int32(21388) // WizardFiendArcherSelfBuffAggressive
	i6kSaintID        = int32(21521) // WizardSaintBasic
	i6kSaintPlainID   = int32(21520) // WizardSaintBasic
	i6kSaintAggrID    = int32(21526) // WizardSaintBasicAggressive
)

// The fixture skills the PR's classes add to the earlier ones.
const (
	i6kBuff     = modelskill.ID(9491) // BUFF
	i6kTeleport = modelskill.ID(9492) // TELEPORT_EFFECT
	i6kFiend    = modelskill.ID(9493) // W_FIEND_ARCHER
)

// individual6kScripts are the behavior chain down to Wizard and the
// classes of the PR, with the parents and the fully grown beast they
// need, in scripts.xml order.
func individual6kScripts() ([]script.Listing, script.Catalog) {
	const p = "script.ai.individual.Monster.WizardBase.Wizard."
	entries := []struct {
		path string
		ctor func() script.Script
	}{
		{"script.ai.individual.DefaultNpc", defaultnpc.New},
		{"script.ai.individual.Monster.MonsterAI", monster.New},
		{"script.ai.individual.Monster.WizardBase.WizardBase", wizard.Base},
		{p + "Wizard", wizard.Wizard},
		{p + "WizardDDMagic2.WizardDDMagic2", wizard.DDMagic2},
		{p + "WizardDDMagic2.WizardCorpseVampireBasic.WizardCorpseVampireBasic", wizard.CorpseVampireBasic},
		{p + "WizardDDMagic2.WizardCorpseVampireBasic.WizardCorpseVampireTeleport", wizard.CorpseVampireTeleport},
		{p + "WizardDDMagic2.WizardGrowth.WizardGrowthStep2", wizard.GrowthStep2},
		{p + "WizardDDMagic2.WizardGrowth.WizardGrowthStep4Aggressive", wizard.GrowthStep4Aggressive},
		{p + "WizardFiendArcher.WizardFiendArcher", wizard.FiendArcher},
		{p + "WizardFiendArcher.WizardFiendArcherAggressive", wizard.FiendArcherAggressive},
		{p + "WizardFiendArcher.WizardFiendArcherSelfBuff", wizard.FiendArcherSelfBuff},
		{p + "WizardFiendArcher.WizardFiendArcherSelfBuffAggressive", wizard.FiendArcherSelfBuffAggressive},
		{p + "WizardSaintBasic.WizardSaintBasic", wizard.SaintBasic},
		{p + "WizardSaintBasic.WizardSaintBasicAggressive", wizard.SaintBasicAggressive},
		{"test.Ind1Director", func() script.Script { return script.Script{} }},
	}
	var list []script.Listing
	catalog := script.Catalog{}
	for _, e := range entries {
		list = append(list, script.Listing{Path: e.path})
		catalog[e.path] = e.ctor
	}
	return list, catalog
}

// bootIndividual6k boots the PR's classes beside templates, every NPC
// casting i6kSkills, every draw the current value of the world's draw.
func bootIndividual6k(t *testing.T, templates ...*npc.Template) *individual6World {
	t.Helper()
	list, catalog := individual6kScripts()
	kinds := map[int32]script.NPCKind{}
	for _, tmpl := range templates {
		kinds[int32(tmpl.ID)] = script.KindHostile
	}
	draw := new(atomic.Int32)
	extra := raidBoot{opts: []gameservertest.Option{
		gameservertest.WithNPCScripts(kinds, list, catalog),
		gameservertest.WithScriptRand(func(n int) int { return min(int(draw.Load()), n-1) }),
		gameservertest.WithNpcSpawnSkills(i6kSkills()),
	}}
	return &individual6World{warriorWorld: bootRaid(t, extra, templates...), draw: draw}
}

// i6kTemplate is a fixture wizard of id with ai listing every skill type
// the PR's classes cast.
func i6kTemplate(id int32, ai npc.AIParams) *npc.Template {
	return i6bTemplate(id, ai, map[npc.SkillType]modelskill.ID{
		npc.SkillTypeWLongRangeDDMagic: wLongDDID, npc.SkillTypeWShortRangeDDMagic: wShortDDID,
		npc.SkillTypeDebuff: i6gDebuff, npc.SkillTypeSelfBuff: i6hSelfBuff, npc.SkillTypeBuff: i6kBuff,
		npc.SkillTypeTeleportEffect: i6kTeleport, npc.SkillTypeWFiendArcher: i6kFiend,
	})
}

// i6kSkills are the fixture skills of the PR's NPCs: harmless one-target
// magics and self buffs, each costing nothing, or 100000 MP for the costly
// ones.
func i6kSkills(costly ...modelskill.ID) *modelskill.Table {
	defs := []modelskill.Definition{npcSkill(i6hSelfBuff, 1, false), npcSkill(i6kBuff, 1, false), npcSkill(i6kTeleport, 1, false)}
	for _, id := range []modelskill.ID{wLongDDID, wShortDDID, i6gDebuff, i6kFiend} {
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

// i6kArmed spawns an NPC of id dx, dy from the character, its skills
// i6kSkills with costly ones.
func (w *individual6World) i6kArmed(t *testing.T, id int32, dx, dy int, costly ...modelskill.ID) *npc.Hostile {
	t.Helper()
	h := w.spawn(t, id, dx, dy)
	w.srv.InstallNPCCastSeam(t, h, i6kSkills(costly...))
	return h
}

// i6kSettled is i6eSettled followed by one idle AI cycle, which leaves
// the next cycle, the one a look runs, clear of the periodic desire decay.
func (w *individual6World) i6kSettled(t *testing.T, hs ...*npc.Hostile) {
	t.Helper()
	w.i6eSettled(t, hs...)
	w.ticks(t, 1)
}

// i6kLookFrom moves the character dx in front of h and runs one AI cycle,
// whose see-creature scan notices it.
func (w *individual6World) i6kLookFrom(t *testing.T, h *npc.Hostile, dx int) {
	t.Helper()
	w.i6fMoveNear(t, h, dx)
	w.ticks(t, 1)
	w.srv.Settle(t)
	w.srv.ReadQueued(t, w.srv.Client)
}

// TestWizardFiendArcherShootsWhatHitsIt runs WizardFiendArcher's attacked
// and skill-finished hooks through its aggressive child. A hit of 80 from
// the character on the level 1 archer hates it 80 / (1 + 7) * 100 = 1000,
// 300 more on its first hit, and wants its fiend archer skill on it,
// weight 1000000; one that cannot pay gives up its spells and wants to
// attack, weight 1000. The end of its cast brings the next one.
func TestWizardFiendArcherShootsWhatHitsIt(t *testing.T) {
	t.Parallel()
	w := bootIndividual6k(t, i6kTemplate(i6kFiendAggrID, nil))
	me := w.player.ObjectID()
	h := w.i6kArmed(t, i6kFiendAggrID, 300, 0)
	broke := w.i6kArmed(t, i6kFiendAggrID, -300, 0, i6kFiend)

	w.hit(t, h, 80)
	first := w.hateListOf(h)
	i6fWantCasts(t, "the first hit", h, map[modelskill.ID]i6fCast{i6kFiend: {me, 1000000, false}})
	w.hit(t, h, 80)
	if second := w.hateListOf(h) - first; first-second != 300 {
		t.Fatalf("hate of the first hit = %v, of the second %v, want 300 more on the first", first, second)
	}
	w.hit(t, broke, 80)
	w.i6eGaveUp(t, broke, 1000)
	i6fWantCasts(t, "short of MP", broke, nil)

	w.i6eCastsWithin(t, h, i6kFiend, 2)
}

// TestWizardFiendArcherAnswersItsClan runs WizardFiendArcher's
// clan-attacked hook through its aggressive child, for the character's hit
// on its clan member. A settled archer with an empty hate list wants its
// fiend archer skill on the attacker, weight 1000000; one that cannot pay
// gives up its spells and wants to attack, weight 1000; a fresh one does
// nothing.
func TestWizardFiendArcherAnswersItsClan(t *testing.T) {
	t.Parallel()
	w := bootIndividual6k(t, inClan(i6kTemplate(i6kFiendAggrID, nil)), inClan(warriorTemplate(i6eMateID, nil)))
	me := w.player.ObjectID()
	settled := w.i6kArmed(t, i6kFiendAggrID, 300, 0)
	broke := w.i6kArmed(t, i6kFiendAggrID, -300, 0, i6kFiend)
	mate := w.spawn(t, i6eMateID, 150, 150)
	w.i6eSettled(t, settled, broke)
	fresh := w.i6kArmed(t, i6kFiendAggrID, 0, 300)

	w.hit(t, mate, 10)
	i6fWantCasts(t, "settled", settled, map[modelskill.ID]i6fCast{i6kFiend: {me, 1000000, false}})
	w.i6eGaveUp(t, broke, 1000)
	i6fWantCasts(t, "fresh", fresh, nil)
}

// TestWizardAggressivesOfThePRStrikeOnSight runs the see-creature hooks of
// WizardFiendArcherAggressive and WizardSaintBasicAggressive. Settled in
// its territory with nobody on its hate list, the archer wants its fiend
// archer skill on the character it sees 100 away and the saint, the
// character 30 away, its short range DD magic, weight 1000000, and each
// hates it; 100 away the saint only hates it. One that cannot pay gives up
// its spells and wants to attack, weight 1000. Fresh, neither reacts.
func TestWizardAggressivesOfThePRStrikeOnSight(t *testing.T) {
	t.Parallel()
	for _, c := range []struct {
		id    int32
		skill modelskill.ID
		near  int
	}{{i6kFiendAggrID, i6kFiend, -100}, {i6kSaintAggrID, wShortDDID, -30}} {
		t.Run(strconv.Itoa(int(c.id)), func(t *testing.T) {
			t.Parallel()
			w := bootIndividual6k(t, i6kTemplate(c.id, nil))
			me := w.player.ObjectID()
			settled := w.i6kArmed(t, c.id, 1500, 0)
			broke := w.i6kArmed(t, c.id, -1500, 0, c.skill)
			w.i6kSettled(t, settled, broke)

			w.i6kLookFrom(t, settled, c.near)
			i6fWantCasts(t, "settled", settled, map[modelskill.ID]i6fCast{c.skill: {me, 1000000, false}})
			if got := w.hateListOf(settled); got <= 0 {
				t.Fatalf("hate list entry of the character it saw = %v, want some", got)
			}
			w.i6kLookFrom(t, broke, c.near)
			w.i6eGaveUp(t, broke, 1000)

			fresh := w.i6kArmed(t, c.id, 2500, 0)
			w.onQueue(t, fresh, func() { fresh.AI().Desires().Clear() })
			w.i6kLookFrom(t, fresh, c.near)
			i6fWantCasts(t, "fresh", fresh, nil)
		})
	}

	t.Run("saint 100 away", func(t *testing.T) {
		t.Parallel()
		w := bootIndividual6k(t, i6kTemplate(i6kSaintAggrID, nil))
		h := w.i6kArmed(t, i6kSaintAggrID, 1500, 0)
		w.i6kSettled(t, h)
		w.i6kLookFrom(t, h, -100)
		i6fWantCasts(t, "100 away", h, nil)
		if got := w.hateListOf(h); got <= 0 {
			t.Fatalf("hate list entry of the character it saw = %v, want some", got)
		}
	})
}

// i6kSelfBuffDone settles h and lets the self buff it cast as it appeared
// end, so it neither attacks nor casts.
func (w *individual6World) i6kSelfBuffDone(t *testing.T, h *npc.Hostile) {
	t.Helper()
	w.i6eSettled(t, h)
	for i := 0; h.AI().CurrentIntention() == actorai.IntentionCast; i++ {
		if i == 5 {
			t.Fatal("setup: the archer still casts")
		}
		w.srv.Advance(t, time.Second)
		w.ticks(t, 1)
	}
	w.onQueue(t, h, func() { h.AI().Desires().Clear() })
}

// TestWizardFiendArcherSelfBuffBuffsItself runs WizardFiendArcherSelfBuff's
// created and attacked hooks and its aggressive child's see-creature hook.
// As it appears it wants its self buff on itself, weight 1000000. A hit by
// the character, while it neither attacks nor casts and holds none of the
// buff's effect, wants the self buff again beside its parent's fiend
// archer skill on the character. Seeing the character, the aggressive one
// wants its self buff, the character holding none of the buff's effect,
// and wants to attack the character, weight 200.
func TestWizardFiendArcherSelfBuffBuffsItself(t *testing.T) {
	t.Parallel()
	w := bootIndividual6k(t, i6kTemplate(i6kFiendSelfAggID, nil))
	me := w.player.ObjectID()
	h := w.i6kArmed(t, i6kFiendSelfAggID, 1500, 0)
	i6fWantCasts(t, "created", h, map[modelskill.ID]i6fCast{i6hSelfBuff: {h.ObjectID(), 1000000, false}})

	w.i6kSelfBuffDone(t, h)
	w.hit(t, h, 10)
	i6fWantCasts(t, "hit", h, map[modelskill.ID]i6fCast{i6hSelfBuff: {h.ObjectID(), 1000000, false}, i6kFiend: {me, 1000000, false}})

	seer := w.i6kArmed(t, i6kFiendSelfAggID, -1500, 0)
	w.i6kSelfBuffDone(t, seer)
	w.i6kLookFrom(t, seer, -100)
	i6fWantCasts(t, "seeing the character", seer, map[modelskill.ID]i6fCast{i6hSelfBuff: {seer.ObjectID(), 1000000, false}})
	if got := i6cAttackWeight(seer, me); got != 200 {
		t.Fatalf("attack desire on the character it saw = %v, want 200", got)
	}
}

// TestWizardSaintBasicStrikesAndDebuffs runs WizardSaintBasic's created,
// attacked and skill-finished hooks, every draw 0. As it appears it wants
// its buff on itself, weight 1000000. A hit by the character wants its
// short range DD magic and its debuff on the character, weight 1000000;
// on a draw of 33 no debuff. Once it gave up its spells, a hit of 80 on
// the level 1 saint adds 80 / (1 + 7) * 100 = 1000 to its attack desire
// with no cast. The end of its cast brings the next one.
func TestWizardSaintBasicStrikesAndDebuffs(t *testing.T) {
	t.Parallel()
	w := bootIndividual6k(t, i6kTemplate(i6kSaintPlainID, nil))
	me := w.player.ObjectID()
	h := w.i6kArmed(t, i6kSaintPlainID, 300, 0)
	i6fWantCasts(t, "created", h, map[modelskill.ID]i6fCast{i6kBuff: {h.ObjectID(), 1000000, false}})
	if got := h.Scratch().Int(npc.IntAI4); got != 0 {
		t.Fatalf("int slot 4 once created = %d, want 0", got)
	}

	w.i6eSettled(t, h)
	w.hit(t, h, 10)
	i6fWantCasts(t, "a hit", h, map[modelskill.ID]i6fCast{wShortDDID: {me, 1000000, false}, i6gDebuff: {me, 1000000, false}})

	w.draw.Store(33)
	calm := w.i6kArmed(t, i6kSaintPlainID, -300, 0)
	w.i6eSettled(t, calm)
	w.hit(t, calm, 10)
	i6fWantCasts(t, "a hit on a draw of 33", calm, map[modelskill.ID]i6fCast{wShortDDID: {me, 1000000, false}})
	w.draw.Store(0)

	quit := w.i6kArmed(t, i6kSaintPlainID, 0, 300)
	w.i6eSettled(t, quit)
	w.onQueue(t, quit, func() { quit.Scratch().SetInt(npc.IntAI0, 1) })
	w.hit(t, quit, 80)
	before := i6cAttackWeight(quit, me)
	w.hit(t, quit, 80)
	if got := i6cAttackWeight(quit, me); got < before+1000 {
		t.Fatalf("given up saint attack desire after another hit = %v, want at least %v", got, before+1000)
	}
	i6fWantCasts(t, "given up", quit, nil)

	w.i6eCastsWithin(t, h, wShortDDID, 2)
}

// TestWizardSaintBasicCallsItsSilhouette runs WizardSaintBasic's attacked
// hook for a saint with a silhouette: a hit while it stands at 60% of its
// HP, on a draw under 30, calls the silhouette on x and y of it, told (1000,
// the attacker), and the saint leaves the world. On a draw of 30, or for a
// saint without a silhouette, it stays.
func TestWizardSaintBasicCallsItsSilhouette(t *testing.T) {
	t.Parallel()
	ai := npc.AIParams{"silhouette": strconv.Itoa(int(i6hFighterFormID))}
	w := bootIndividual6k(t, i6kTemplate(i6kSaintID, ai), i6kTemplate(i6kSaintPlainID, nil), warriorTemplate(i6hFighterFormID, nil))
	me := w.player.ObjectID()

	w.draw.Store(30)
	stays := w.i6kArmed(t, i6kSaintID, 300, 0)
	w.hit(t, stays, int(stays.MaxHP())*4/10)
	w.hit(t, stays, 10)
	if !w.inWorld(stays) {
		t.Fatal("the saint left the world on a draw of 30")
	}
	plain := w.i6kArmed(t, i6kSaintPlainID, 0, 300)
	w.draw.Store(0)
	w.hit(t, plain, int(plain.MaxHP())*4/10)
	w.hit(t, plain, 10)
	if !w.inWorld(plain) {
		t.Fatal("the saint without a silhouette left the world")
	}

	h := w.i6kArmed(t, i6kSaintID, -300, 0)
	x, y, _ := h.Position()
	w.hit(t, h, int(h.MaxHP())*4/10)
	if !w.inWorld(h) {
		t.Fatal("the saint left the world at its full HP")
	}
	w.hit(t, h, 10)
	w.srv.Settle(t)
	if w.inWorld(h) {
		t.Fatal("the saint stayed in the world after calling its silhouette")
	}
	called := w.liveNPCs(i6hFighterFormID)
	if len(called) != 1 {
		t.Fatalf("silhouettes = %d, want 1", len(called))
	}
	if sx, sy, _ := called[0].Position(); sx != x || sy != y {
		t.Fatalf("silhouette at %d,%d, want %d,%d", sx, sy, x, y)
	}
	if got := [2]int32{called[0].Scratch().Int(npc.IntParam1), called[0].Scratch().Int(npc.IntParam2)}; got != [2]int32{1000, me} {
		t.Fatalf("silhouette spawn parameters = %v, want 1000 and the character %d", got, me)
	}
}

// TestWizardSaintBasicAnswersItsClan runs WizardSaintBasic's clan-attacked
// hook for the character's hit on its clan member. A settled saint with an
// empty hate list wants its debuff on the attacker, weight 1000000, on a
// draw under 33; on a draw of 33 its short range DD magic. A fresh one
// does nothing.
func TestWizardSaintBasicAnswersItsClan(t *testing.T) {
	t.Parallel()
	w := bootIndividual6k(t, inClan(i6kTemplate(i6kSaintPlainID, nil)), inClan(warriorTemplate(i6eMateID, nil)))
	me := w.player.ObjectID()
	debuffs := w.i6kArmed(t, i6kSaintPlainID, 2000, 0)
	strikes := w.i6kArmed(t, i6kSaintPlainID, 2000, 300)
	mate := w.spawn(t, i6eMateID, 2150, 150)
	w.i6eSettled(t, debuffs, strikes)
	fresh := w.i6kArmed(t, i6kSaintPlainID, 2300, 0)
	w.onQueue(t, fresh, func() { fresh.AI().Desires().Clear() })

	w.onQueue(t, strikes, func() { script.NPCOf(strikes).HateList().Add(script.PlayerOf(w.player), 1) })
	w.hit(t, mate, 10)
	i6fWantCasts(t, "draw 0", debuffs, map[modelskill.ID]i6fCast{i6gDebuff: {me, 1000000, false}})
	i6fWantCasts(t, "hating", strikes, nil)
	i6fWantCasts(t, "fresh", fresh, nil)

	w.onQueue(t, strikes, func() { strikes.AI().Hates().Clear() })
	w.onQueue(t, debuffs, func() { debuffs.AI().Desires().Clear(); debuffs.AI().Hates().Clear() })
	w.draw.Store(33)
	w.hit(t, mate, 10)
	i6fWantCasts(t, "draw 33", strikes, map[modelskill.ID]i6fCast{wShortDDID: {me, 1000000, false}})
}

// TestWizardCorpseVampireTeleportJumpsToWhatItSees runs
// WizardCorpseVampireTeleport's see-creature hook. Settled, a vampire
// seeing the character 300 away jumps to it, wants its teleport effect on
// itself, weight 1000000; the jump leaves its territory, so it wants no
// attack. 100 away it only wants to attack the character, weight 200.
// Fresh, it does nothing.
func TestWizardCorpseVampireTeleportJumpsToWhatItSees(t *testing.T) {
	t.Parallel()
	w := bootIndividual6k(t, i6kTemplate(i6kTeleportID, nil))
	me := w.player.ObjectID()
	far := w.i6kArmed(t, i6kTeleportID, 1500, 0)
	near := w.i6kArmed(t, i6kTeleportID, -1500, 0)
	w.i6kSettled(t, far, near)

	w.i6kLookFrom(t, far, -300)
	px, py, _ := w.player.Position()
	if x, y, _ := far.Position(); x != px || y != py {
		t.Fatalf("vampire at %d,%d after seeing the character 300 away, want on it at %d,%d", x, y, px, py)
	}
	i6fWantCasts(t, "300 away", far, map[modelskill.ID]i6fCast{i6kTeleport: {far.ObjectID(), 1000000, false}})
	if got := i6cAttackWeight(far, me); got != 0 {
		t.Fatalf("attack desire on the character 300 away = %v, want none out of its territory", got)
	}

	w.i6kLookFrom(t, near, -100)
	i6fWantCasts(t, "100 away", near, nil)
	if got := i6cAttackWeight(near, me); got != 200 {
		t.Fatalf("attack desire on the character 100 away = %v, want 200", got)
	}

	fresh := w.i6kArmed(t, i6kTeleportID, 0, 2500)
	w.i6kLookFrom(t, fresh, -300)
	if got := i6cAttackWeight(fresh, me); got != 0 {
		t.Fatalf("fresh vampire attack desire = %v, want none", got)
	}
}

// TestWizardCorpseVampireTeleportLooksAroundEvery10s runs
// WizardCorpseVampireTeleport's created and timer hooks: its 10 s timer
// "3000" looks around, and the look tells its see-creature hook about the
// character beside it, which it then wants to attack, weight 200.
func TestWizardCorpseVampireTeleportLooksAroundEvery10s(t *testing.T) {
	t.Parallel()
	w := bootIndividual6k(t, i6kTemplate(i6kTeleportID, nil))
	me := w.player.ObjectID()
	h := w.i6kArmed(t, i6kTeleportID, 1500, 0)
	w.i6eSettled(t, h)
	w.i6fMoveNear(t, h, -100)
	w.srv.Settle(t)
	if got := i6cAttackWeight(h, me); got != 0 {
		t.Fatalf("setup: attack desire before the look = %v, want none", got)
	}
	for i := 0; i6cAttackWeight(h, me) != 200; i++ {
		if i == 10 {
			t.Fatalf("attack desire 10 s on = %v, want 200 from the look", i6cAttackWeight(h, me))
		}
		w.srv.Advance(t, time.Second)
		w.srv.Settle(t)
	}
}

// i6kGrowthAI are WizardGrowthStep2's AI parameters: a 1 s eating
// animation, its two silhouettes and its last one.
var i6kGrowthAI = npc.AIParams{
	"TakeSocial": "30", "MoveAroundSocial": "78",
	"silhouette1": strconv.Itoa(int(i6hFighterFormID)), "silhouette2": strconv.Itoa(int(i6hMysticFormID)),
	"silhouette_ex2": strconv.Itoa(int(i6eMateID)),
}

// TestWizardGrowthStep2GrowsIntoASilhouette runs WizardGrowthStep2's
// created, attacked and timer hooks. Called, every draw 0, with the
// character 300 away as its feeder, it hates it 100 and wants its self
// buff and its long range DD magic on the character, weight 1000000. Fed
// by the character it eats without its parent's answer to the hit and, for
// a draw within 33% of one feed, grows 1 s later into its first
// silhouette. On a draw of 50 it wants no self buff, one feed is not
// enough, and after the second it grows into its second silhouette.
func TestWizardGrowthStep2GrowsIntoASilhouette(t *testing.T) {
	t.Parallel()
	for _, c := range []struct {
		draw  int32
		feeds int
		into  int32
	}{{0, 1, i6hFighterFormID}, {50, 2, i6hMysticFormID}} {
		t.Run(strconv.Itoa(int(c.draw)), func(t *testing.T) {
			t.Parallel()
			w := bootIndividual6k(t, i6kTemplate(i6kStep2ID, i6kGrowthAI), warriorTemplate(i6hFighterFormID, nil),
				warriorTemplate(i6hMysticFormID, nil), warriorTemplate(i6gFoeID, nil))
			iw := &ind1World{warriorWorld: w.warriorWorld}
			me := w.player.ObjectID()
			w.draw.Store(c.draw)
			master := w.spawn(t, i6gFoeID, 1500, 0)
			n := iw.ind4cCallPrivate(t, master, i6kStep2ID, 300, 0, script.Params{P1: me, P2: int32(ind1gFood1)})
			iw.ind1gWantFeeder(t, "created", n, ind1gFood1, 1)
			if got := w.hateListOf(n); got != 100 {
				t.Fatalf("hate list entry of its feeder = %v, want 100", got)
			}
			want := map[modelskill.ID]i6fCast{wLongDDID: {me, 1000000, false}}
			if c.draw == 0 {
				want[i6hSelfBuff] = i6fCast{n.ObjectID(), 1000000, false}
			}
			i6fWantCasts(t, "created", n, want)

			w.onQueue(t, n, func() { n.AI().Desires().Clear() })
			for i := range c.feeds {
				iw.ind1gFeed(t, n, w.player, ind1gFood1)
				if i < c.feeds-1 {
					iw.ind1gWantFeeder(t, "after the first feed", n, ind1gFood1, 1)
					w.onQueue(t, n, func() { n.AI().Desires().Clear() })
				}
			}
			ind1gWantEat(t, "after the feed", n, 78*1000/30)
			i6fWantCasts(t, "after the feed", n, nil)
			iw.ind1gWantFeeder(t, "after the feed", n, ind1gFood1, 3)
			w.srv.Advance(t, time.Second)
			iw.ind1gGrown(t, n, c.into, ind1gFood1)
		})
	}
}

// TestWizardGrowthStep2AnswersAPlainHit runs WizardGrowthStep2's attacked
// hook for a hit with no food, every draw 0: its parent wants its long
// range DD magic on the character 300 away.
func TestWizardGrowthStep2AnswersAPlainHit(t *testing.T) {
	t.Parallel()
	w := bootIndividual6k(t, i6kTemplate(i6kStep2ID, i6kGrowthAI))
	me := w.player.ObjectID()
	h := w.i6kArmed(t, i6kStep2ID, 300, 0)
	w.i6eSettled(t, h)
	w.hit(t, h, 10)
	i6fWantCasts(t, "a plain hit", h, map[modelskill.ID]i6fCast{wLongDDID: {me, 1000000, false}})
}

// TestWizardGrowthStep2GrowsWhenANeighbourDoes runs WizardGrowthStep2's
// script-event hook, every draw 0: a fully grown beast appearing tells
// the beasts within 700 (event 10018), and the fed one grows into its last
// silhouette, keeping its feeder and food.
func TestWizardGrowthStep2GrowsWhenANeighbourDoes(t *testing.T) {
	t.Parallel()
	w := bootIndividual6k(t, i6kTemplate(i6kStep2ID, i6kGrowthAI), i6hTemplate(i6hStep4ID, nil), warriorTemplate(i6gFoeID, nil), warriorTemplate(i6eMateID, nil))
	iw := &ind1World{warriorWorld: w.warriorWorld}
	me := w.player.ObjectID()
	master := w.spawn(t, i6gFoeID, 1500, 0)
	fed := iw.ind4cCallPrivate(t, master, i6kStep2ID, 300, 0, script.Params{P1: me, P2: int32(ind1gFood1)})
	iw.ind4cCallPrivate(t, master, i6hStep4ID, 300, 150, script.Params{P1: me, P2: int32(ind1gFood1)})
	iw.ind1gGrown(t, fed, i6eMateID, ind1gFood1)
}
