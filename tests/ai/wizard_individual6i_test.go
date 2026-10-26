package ai

import (
	"fmt"
	"os"
	"path/filepath"
	"slices"
	"strconv"
	"sync/atomic"
	"testing"
	"time"

	gamexml "github.com/fatal10110/acis_golang/internal/gameserver/data/xml"
	actorai "github.com/fatal10110/acis_golang/internal/gameserver/model/actor/ai"
	"github.com/fatal10110/acis_golang/internal/gameserver/model/actor/move"
	"github.com/fatal10110/acis_golang/internal/gameserver/model/actor/npc"
	"github.com/fatal10110/acis_golang/internal/gameserver/model/location"
	modelskill "github.com/fatal10110/acis_golang/internal/gameserver/model/skill"
	"github.com/fatal10110/acis_golang/internal/gameserver/script"
	"github.com/fatal10110/acis_golang/internal/gameserver/script/ai/defaultnpc"
	"github.com/fatal10110/acis_golang/internal/gameserver/script/ai/monster"
	"github.com/fatal10110/acis_golang/internal/gameserver/script/ai/wizard"
	"github.com/fatal10110/acis_golang/internal/gameservertest"
	"github.com/rs/zerolog"
)

// The NPC ids of the ninth PR of the sixth individual behavior batch: the
// datapack ids each class binds.
const (
	i6iHealID        = int32(20292) // WizardDDMagic2Heal
	i6iHealAggrID    = int32(21423) // WizardDDMagic2HealAggressive
	i6iSolinaID      = int32(22134) // WizardDDMagic2Solina
	i6iHealerID      = int32(21300) // WizardHealer
	i6iHealerAggrID  = int32(21412) // WizardHealerAggressive
	i6iHereticAggrID = int32(22144) // WizardHereticDDMagic2Aggressive
	i6iHereticID     = int32(22150) // WizardHereticDDMagicCurse
	i6iNecroID       = int32(21565) // WizardCorpseNecro
)

// The skills the PR's classes list by type beside the batch's long and
// short range DD magics, the debuff and the mark, which stands for the
// first debuff, and the cancel whose effect marks the character.
const (
	i6iMagicHeal = modelskill.ID(9470)
	i6iHeal      = modelskill.ID(9471)
	i6iBuff      = modelskill.ID(9472)
	i6iDDMagic   = modelskill.ID(9473)
	i6iCancel    = modelskill.ID(9474)
)

// individual6iScripts are the behavior chain down to WizardDDMagic2Curse,
// the classes of the PR in scripts.xml order, and the director the tests
// send script events through.
func individual6iScripts() ([]script.Listing, script.Catalog) {
	const p = "script.ai.individual.Monster.WizardBase.Wizard.WizardDDMagic2."
	entries := []struct {
		path string
		ctor func() script.Script
	}{
		{"script.ai.individual.DefaultNpc", defaultnpc.New},
		{"script.ai.individual.Monster.MonsterAI", monster.New},
		{"script.ai.individual.Monster.WizardBase.WizardBase", wizard.Base},
		{"script.ai.individual.Monster.WizardBase.Wizard.Wizard", wizard.Wizard},
		{p + "WizardDDMagic2", wizard.DDMagic2},
		{p + "WizardDDMagic2Curse", wizard.DDMagic2Curse},
		{p + "WizardCorpseNecro.WizardCorpseNecro", wizard.CorpseNecro},
		{p + "WizardDDMagic2Heal", wizard.DDMagic2Heal},
		{p + "WizardDDMagic2HealAggressive", wizard.DDMagic2HealAggressive},
		{p + "WizardDDMagic2Solina", wizard.DDMagic2Solina},
		{p + "WizardHealer", wizard.Healer},
		{p + "WizardHealerAggressive", wizard.HealerAggressive},
		{p + "WizardHereticDDMagic2Aggressive", wizard.HereticDDMagic2Aggressive},
		{p + "WizardHereticDDMagicCurse", wizard.HereticDDMagicCurse},
		{i6dDirectorPath, func() script.Script { return script.Script{} }},
	}
	var list []script.Listing
	catalog := script.Catalog{}
	for _, e := range entries {
		list = append(list, script.Listing{Path: e.path})
		catalog[e.path] = e.ctor
	}
	return list, catalog
}

// bootIndividual6i boots the PR's classes beside templates, every NPC
// casting i6iSkills, the scripts reading the mark and the cancel, every
// draw the current value of the world's draw.
func bootIndividual6i(t *testing.T, extra raidBoot, templates ...*npc.Template) *individual6World {
	t.Helper()
	list, catalog := individual6iScripts()
	kinds := map[int32]script.NPCKind{}
	for _, tmpl := range templates {
		kinds[int32(tmpl.ID)] = script.KindHostile
	}
	draw := new(atomic.Int32)
	extra.opts = append(extra.opts,
		gameservertest.WithNPCScripts(kinds, list, catalog),
		gameservertest.WithScriptRand(func(n int) int { return min(int(draw.Load()), n-1) }),
		gameservertest.WithNpcSpawnSkills(i6iSkills()),
		gameservertest.WithScriptSkills(i6hMarkSkill(), i6iCancelSkill()),
	)
	return &individual6World{warriorWorld: bootRaid(t, extra, templates...), draw: draw}
}

// i6iTemplate is a fixture wizard of id listing every skill type the PR's
// classes cast, the mark as its first debuff.
func i6iTemplate(id int32) *npc.Template {
	return i6bTemplate(id, nil, map[npc.SkillType]modelskill.ID{
		npc.SkillTypeWLongRangeDDMagic: wLongDDID, npc.SkillTypeWShortRangeDDMagic: wShortDDID,
		npc.SkillTypeDebuff: i6gDebuff, npc.SkillTypeDebuff1: i6hMark,
		npc.SkillTypeMagicHeal: i6iMagicHeal, npc.SkillTypeHeal: i6iHeal, npc.SkillTypeBuff: i6iBuff,
		npc.SkillTypeDDMagic: i6iDDMagic, npc.SkillTypeCancel: i6iCancel,
	})
}

// i6iSkills are the fixture skills of the PR's NPCs: harmless one-target
// magics, the mark and the cancel, each costing nothing, or 100000 MP for
// the costly ones.
func i6iSkills(costly ...modelskill.ID) *modelskill.Table {
	defs := []modelskill.Definition{i6hMarkSkill(), i6iCancelSkill()}
	for _, id := range []modelskill.ID{wLongDDID, wShortDDID, i6gDebuff, i6iMagicHeal, i6iHeal, i6iBuff, i6iDDMagic} {
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

// i6iCancelSkill is the cancel whose effect, of its own stack, lasts a
// minute.
func i6iCancelSkill() modelskill.Definition {
	d := ind1Skill(i6iCancel, 0, true)
	d.Effects = []modelskill.EffectTemplate{{Name: "Buff", Time: 60, Icon: true, StackType: "i6i_cancel", StackOrder: 1}}
	return d
}

// i6iArmed spawns an NPC of id dx, dy from the character, its skills
// i6iSkills with costly ones.
func (w *individual6World) i6iArmed(t *testing.T, id int32, dx, dy int, costly ...modelskill.ID) *npc.Hostile {
	t.Helper()
	h := w.spawn(t, id, dx, dy)
	w.srv.InstallNPCCastSeam(t, h, i6iSkills(costly...))
	return h
}

// i6iClear drops h's desires.
func (w *individual6World) i6iClear(t *testing.T, h *npc.Hostile) {
	t.Helper()
	w.onQueue(t, h, func() { h.AI().Desires().Clear() })
}

// i6iForget drops h's desires and empties its hate list.
func (w *individual6World) i6iForget(t *testing.T, h *npc.Hostile) {
	t.Helper()
	w.onQueue(t, h, func() { h.AI().Desires().Clear(); h.AI().Hates().Clear() })
}

// i6iFlee reports the weight with which h wants to run from target, 0 for
// none.
func i6iFlee(h *npc.Hostile, target int32) float64 {
	d, ok := desire(h, actorai.IntentionFlee)
	if !ok || d.FinalTarget == nil || d.FinalTarget.ObjectID() != target {
		return 0
	}
	return d.Weight
}

// TestWizardDDMagic2HealHealsItselfBelow70 runs WizardDDMagic2Heal's
// attacked hook for hits from 300 away, after its parent's long range DD
// magic. Hit at full HP it heals nothing; below 70% of its HP, on a draw
// under 33, it wants its magic heal on itself, weight 1000000; on a draw
// of 33, not.
func TestWizardDDMagic2HealHealsItselfBelow70(t *testing.T) {
	t.Parallel()
	w := bootIndividual6i(t, raidBoot{}, i6iTemplate(i6iHealID))
	me := w.player.ObjectID()
	h := w.i6iArmed(t, i6iHealID, 300, 0)
	w.hit(t, h, 10)
	i6fWantCasts(t, "full HP", h, map[modelskill.ID]i6fCast{wLongDDID: {me, 1000000, false}})

	w.draw.Store(99)
	w.hit(t, h, int(h.MaxHP())*4/10)
	w.i6iClear(t, h)
	w.draw.Store(0)
	w.hit(t, h, 10)
	i6fWantCasts(t, "below 70%", h, map[modelskill.ID]i6fCast{wLongDDID: {me, 1000000, false}, i6iMagicHeal: {h.ObjectID(), 1000000, false}})

	w.i6iClear(t, h)
	w.draw.Store(33)
	w.hit(t, h, 10)
	i6fWantCasts(t, "below 70% on a draw of 33", h, map[modelskill.ID]i6fCast{wLongDDID: {me, 1000000, false}})
}

// TestWizardDDMagic2HealHealsItsClan runs WizardDDMagic2Heal's
// clan-attacked hook for a hit on its clan member. Settled with an empty
// hate list, on a draw under 33, it wants its magic heal on the member,
// weight 1000000, beside its parent's long range DD magic on the attacker
// 300 away; on a draw of 33 only the DD magic. Fresh, nothing.
func TestWizardDDMagic2HealHealsItsClan(t *testing.T) {
	t.Parallel()
	w := bootIndividual6i(t, raidBoot{}, i6gFaction(i6iTemplate(i6iHealID)), i6gFaction(warriorTemplate(i6gMateID, nil)))
	me := w.player.ObjectID()
	h := w.i6iArmed(t, i6iHealID, 300, 0)
	mate := w.spawn(t, i6gMateID, 150, 150)
	w.i6eSettled(t, h)
	fresh := w.i6iArmed(t, i6iHealID, 0, 300)
	w.hit(t, mate, 10)
	i6fWantCasts(t, "settled", h, map[modelskill.ID]i6fCast{wLongDDID: {me, 1000000, false}, i6iMagicHeal: {mate.ObjectID(), 1000000, false}})
	i6fWantCasts(t, "fresh", fresh, nil)

	w.draw.Store(33)
	w.i6iForget(t, h)
	w.hit(t, mate, 10)
	i6fWantCasts(t, "a draw of 33", h, map[modelskill.ID]i6fCast{wLongDDID: {me, 1000000, false}})
}

// TestWizardHealAndHereticAggressivesStrikeOnSight runs the see-creature
// hooks of WizardDDMagic2HealAggressive and
// WizardHereticDDMagic2Aggressive. Settled in its territory with nobody on
// its hate list, each wants its short range DD magic on the character it
// sees 100 away, weight 1000000, and hates it; one that cannot pay gives
// up its spells and wants to attack instead. Hating another, or fresh, it
// does nothing.
func TestWizardHealAndHereticAggressivesStrikeOnSight(t *testing.T) {
	t.Parallel()
	for _, id := range []int32{i6iHealAggrID, i6iHereticAggrID} {
		t.Run(strconv.Itoa(int(id)), func(t *testing.T) {
			t.Parallel()
			w := bootIndividual6i(t, raidBoot{}, i6iTemplate(id), warriorTemplate(i6gFoeID, nil))
			me := w.player.ObjectID()
			settled := w.i6iArmed(t, id, 1500, 0)
			broke := w.i6iArmed(t, id, -1500, 0, wShortDDID)
			busy := w.i6iArmed(t, id, 0, 1500)
			foe := w.spawn(t, i6gFoeID, 0, 2500)
			w.i6eSettled(t, settled, broke, busy)
			w.onQueue(t, busy, func() { script.NPCOf(busy).HateList().Add(script.NPCOf(foe), 100) })
			w.ticks(t, 1)

			w.i6fLookAt(t, settled)
			i6fWantCasts(t, "settled", settled, map[modelskill.ID]i6fCast{wShortDDID: {me, 1000000, false}})
			if got := w.hateListOf(settled); got <= 0 {
				t.Fatalf("hate list entry of the character it saw = %v, want some", got)
			}
			w.i6fLookAt(t, broke)
			w.i6eGaveUp(t, broke, 1000)
			w.i6fLookAt(t, busy)
			i6fWantCasts(t, "busy", busy, nil)
			if got := w.hateListOf(busy); got != 0 {
				t.Fatalf("busy hate list entry of the character it saw = %v, want none", got)
			}

			fresh := w.i6iArmed(t, id, 2500, 0)
			w.i6fLookAt(t, fresh)
			i6fWantCasts(t, "fresh", fresh, nil)
		})
	}
}

// TestWizardDDMagic2SolinaGreetsAndReproachesLightBearers runs
// WizardDDMagic2Solina's see-creature, attacked, created and no-desire
// hooks. A character of a light occupation (classes 5, 16, 30, 90, 97 and
// 105) it sees gets its first blessing (10075) on a draw under 50, else
// the second (10076), with no spell, and goes on its hate list; any other
// character (classes 0, 17 and 98) it sees 100 away gets its short range DD
// magic, weight 1000000, and one seen 300 away its long range one on a
// draw of 79, its short range one on a draw of 80; each goes on its hate
// list. Hit by a bishop it
// says its first reproach (10077) on a draw under 50, else the second
// (10078), once: the next hit says nothing. Created, and when it has
// nothing left to do, both lines may come again (int slots 3 and 4 clear).
func TestWizardDDMagic2SolinaGreetsAndReproachesLightBearers(t *testing.T) {
	t.Parallel()
	w := bootIndividual6i(t, raidBoot{}, i6iTemplate(i6iSolinaID))
	iw := &ind1World{warriorWorld: w.warriorWorld}
	me := w.player.ObjectID()
	saysOf := func(frames [][]byte, h *npc.Hostile) []string { return says(frames, h.ObjectID()) }

	// Every light occupation gets a blessing, the two lines by turns.
	for i, class := range []int{5, 16, 30, 90, 97, 105} {
		draw, line := int32(0), int32(10075)
		if i%2 == 1 {
			draw, line = 50, 10076
		}
		iw.setClass(t, w.player, class)
		w.draw.Store(draw)
		h := w.i6iArmed(t, i6iSolinaID, 1500, 0)
		if got := []int32{h.Scratch().Int(npc.IntAI3), h.Scratch().Int(npc.IntAI4)}; !slices.Equal(got, []int32{0, 0}) {
			t.Fatalf("int slots 3 and 4 as created = %v, want 0 0", got)
		}
		w.i6iClear(t, h)
		if got := saysOf(w.i6fLookAt(t, h), h); !slices.Equal(got, []string{script.NpcString(line, "Hunter")}) {
			t.Fatalf("class %d, draw %d: lines on seeing it = %q, want the %d blessing", class, draw, got, line)
		}
		i6fWantCasts(t, "seeing a light occupation", h, nil)
		if got := w.hateListOf(h); got <= 0 {
			t.Fatalf("class %d: hate list entry of the character it saw = %v, want some", class, got)
		}
		if got := h.Scratch().Int(npc.IntAI3); got != 1 {
			t.Fatalf("class %d: int slot 3 after the blessing = %d, want 1", class, got)
		}
	}

	// Any other occupation, seen 100 away, gets its short range DD magic.
	w.draw.Store(0)
	for _, class := range []int{0, 17, 98} {
		iw.setClass(t, w.player, class)
		h := w.i6iArmed(t, i6iSolinaID, -1500, 0)
		w.i6iClear(t, h)
		if got := saysOf(w.i6fLookAt(t, h), h); len(got) != 0 {
			t.Fatalf("class %d: lines on seeing it = %q, want none", class, got)
		}
		i6fWantCasts(t, "seeing another occupation", h, map[modelskill.ID]i6fCast{wShortDDID: {me, 1000000, false}})
		if got := w.hateListOf(h); got <= 0 {
			t.Fatalf("class %d: hate list entry of the character it saw = %v, want some", class, got)
		}
	}

	// Seen 300 away, its long range DD magic on a draw under 80, else its
	// short range one.
	iw.setClass(t, w.player, 0)
	for _, c := range []struct {
		draw int32
		want modelskill.ID
	}{{79, wLongDDID}, {80, wShortDDID}} {
		w.draw.Store(c.draw)
		h := w.i6iArmed(t, i6iSolinaID, 1500, 0)
		w.i6iClear(t, h)
		w.i6fMoveNear(t, h, -300)
		frames := w.ticks(t, 1)
		w.srv.Settle(t)
		frames = append(frames, w.srv.ReadQueued(t, w.srv.Client)...)
		if got := saysOf(frames, h); len(got) != 0 {
			t.Fatalf("draw %d: lines on seeing a fighter 300 away = %q, want none", c.draw, got)
		}
		i6fWantCasts(t, "seeing a fighter 300 away", h, map[modelskill.ID]i6fCast{c.want: {me, 1000000, false}})
		if got := w.hateListOf(h); got <= 0 {
			t.Fatalf("draw %d: hate list entry of the fighter it saw 300 away = %v, want some", c.draw, got)
		}
	}
	w.draw.Store(0)

	iw.setClass(t, w.player, 16)
	for _, c := range []struct {
		draw int32
		line int32
	}{{0, 10077}, {50, 10078}} {
		w.draw.Store(c.draw)
		hit := w.i6iArmed(t, i6iSolinaID, 300, int(c.draw)*4)
		w.hit(t, hit, 10)
		if got := saysOf(w.srv.ReadQueued(t, w.srv.Client), hit); !slices.Equal(got, []string{script.NpcString(c.line, "Hunter")}) {
			t.Fatalf("draw %d: lines on a bishop's hit = %q, want the %d reproach", c.draw, got, c.line)
		}
		w.hit(t, hit, 10)
		if got := saysOf(w.srv.ReadQueued(t, w.srv.Client), hit); len(got) != 0 {
			t.Fatalf("draw %d: lines on a bishop's second hit = %q, want none", c.draw, got)
		}
	}

	idle := w.i6iArmed(t, i6iSolinaID, 0, 1500)
	w.onQueue(t, idle, func() {
		idle.AI().Desires().Clear()
		idle.Scratch().SetInt(npc.IntAI3, 1)
		idle.Scratch().SetInt(npc.IntAI4, 1)
	})
	w.ticks(t, 3)
	if got := idle.AI().CurrentIntention(); got != actorai.IntentionWander {
		t.Fatalf("setup: intention with nothing left to do = %v, want wander", got)
	}
	if got := []int32{idle.Scratch().Int(npc.IntAI3), idle.Scratch().Int(npc.IntAI4)}; !slices.Equal(got, []int32{0, 0}) {
		t.Fatalf("int slots 3 and 4 with nothing left to do = %v, want 0 0", got)
	}
}

// TestWizardHealerBuffsItselfAndRunsFromHits runs WizardHealer's created
// and attacked hooks for hits from 300 away. Created, it wants its buff on
// itself, weight 1000000, clearing int slots 1 and 2. Hit above half its
// HP on a draw under 33 it wants to run from the attacker, 500, weight
// 1000000, and casts nothing; with int slot 2 set, or on a draw of 33, its
// parent answers with its long range DD magic instead.
func TestWizardHealerBuffsItselfAndRunsFromHits(t *testing.T) {
	t.Parallel()
	w := bootIndividual6i(t, raidBoot{}, i6iTemplate(i6iHealerID))
	me := w.player.ObjectID()
	h := w.i6iArmed(t, i6iHealerID, 300, 0)
	i6fWantCasts(t, "created", h, map[modelskill.ID]i6fCast{i6iBuff: {h.ObjectID(), 1000000, false}})
	if got := []int32{h.Scratch().Int(npc.IntAI1), h.Scratch().Int(npc.IntAI2)}; !slices.Equal(got, []int32{0, 0}) {
		t.Fatalf("int slots 1 and 2 as created = %v, want 0 0", got)
	}

	w.i6iClear(t, h)
	w.hit(t, h, 10)
	if got := i6iFlee(h, me); got != 1000000 {
		t.Fatalf("flee desire from the attacker = %v, want 1000000", got)
	}
	i6fWantCasts(t, "running", h, nil)

	w.onQueue(t, h, func() { h.AI().Desires().Clear(); h.Scratch().SetInt(npc.IntAI2, 1) })
	w.hit(t, h, 10)
	i6fWantCasts(t, "int slot 2 set", h, map[modelskill.ID]i6fCast{wLongDDID: {me, 1000000, false}})
	if got := i6iFlee(h, me); got != 0 {
		t.Fatalf("flee desire with int slot 2 set = %v, want none", got)
	}

	w.onQueue(t, h, func() { h.AI().Desires().Clear(); h.Scratch().SetInt(npc.IntAI2, 0) })
	w.draw.Store(33)
	w.hit(t, h, 10)
	i6fWantCasts(t, "a draw of 33", h, map[modelskill.ID]i6fCast{wLongDDID: {me, 1000000, false}})
	if got := i6iFlee(h, me); got != 0 {
		t.Fatalf("flee desire on a draw of 33 = %v, want none", got)
	}
}

// TestWizardHealerTendsItsClanAndRunsFromItsKiller runs WizardHealer's
// clan-attacked and clan-died hooks for hits on its clan member. Above
// half the member's HP, on a draw under 33, it wants its buff on the
// member and its debuff on the attacker, weight 1000000 each; on a draw of
// 33 nothing. At half or less, on a draw under 50, it wants its heal on
// the member. When the character kills the member it wants to run from
// the character, 500, weight 1000000.
func TestWizardHealerTendsItsClanAndRunsFromItsKiller(t *testing.T) {
	t.Parallel()
	w := bootIndividual6i(t, raidBoot{}, i6gFaction(i6iTemplate(i6iHealerID)), i6gFaction(warriorTemplate(i6gMateID, nil)))
	me := w.player.ObjectID()
	h := w.i6iArmed(t, i6iHealerID, 300, 0)
	mate := w.spawn(t, i6gMateID, 150, 150)
	w.i6iClear(t, h)
	w.hit(t, mate, 10)
	i6fWantCasts(t, "member above half", h, map[modelskill.ID]i6fCast{i6iBuff: {mate.ObjectID(), 1000000, false}, i6gDebuff: {me, 1000000, false}})

	w.i6iClear(t, h)
	w.draw.Store(33)
	w.hit(t, mate, 10)
	i6fWantCasts(t, "member above half on a draw of 33", h, nil)

	w.draw.Store(99)
	w.hit(t, mate, int(mate.MaxHP())*6/10)
	w.i6iClear(t, h)
	w.draw.Store(0)
	w.hit(t, mate, 10)
	i6fWantCasts(t, "member below half", h, map[modelskill.ID]i6fCast{i6iHeal: {mate.ObjectID(), 1000000, false}})

	w.i6iClear(t, h)
	w.hit(t, mate, int(mate.MaxHP()))
	if !mate.Dead() {
		t.Fatal("setup: the clan member survived")
	}
	w.srv.AdvanceUntil(t, "the healer runs from the killer", func() bool { return i6iFlee(h, me) == 1000000 })
}

// TestWizardHealerAggressiveAttacksWhatItSees runs WizardHealerAggressive's
// see-creature hook: the character it sees in its territory brings an
// attack desire, weight 200, where its parent WizardHealer wants none.
func TestWizardHealerAggressiveAttacksWhatItSees(t *testing.T) {
	t.Parallel()
	w := bootIndividual6i(t, raidBoot{}, i6iTemplate(i6iHealerAggrID), i6iTemplate(i6iHealerID))
	me := w.player.ObjectID()
	aggr := w.i6iArmed(t, i6iHealerAggrID, 1500, 0)
	calm := w.i6iArmed(t, i6iHealerID, -1500, 0)
	w.i6fLookAt(t, aggr)
	if got := i6cAttackWeight(aggr, me); got != 200 {
		t.Fatalf("attack desire on the character it saw = %v, want 200", got)
	}
	w.i6fLookAt(t, calm)
	if got := i6cAttackWeight(calm, me); got != 0 {
		t.Fatalf("WizardHealer attack desire on the character it saw = %v, want none", got)
	}
}

// TestWizardHereticsEngageAndAnswerCalls runs the created, attacked,
// clan-attacked and script-event hooks of WizardHereticDDMagic2Aggressive
// and WizardHereticDDMagicCurse, every draw 0. Each is created unengaged;
// a hit on it or on its clan member while it neither attacks nor casts
// engages it (int slot 4). A shouted target (10002) or a call for help
// (10033) naming the character 300 away drops its attack desires and
// wants its long range DD magic on the character, weight 1000000, hating
// it 200; the call for help also engages it. A call to walk home (10035)
// disengages it, drops its attack desires and wants its spawn point,
// weight 100. A call naming no creature does nothing.
func TestWizardHereticsEngageAndAnswerCalls(t *testing.T) {
	t.Parallel()
	for _, id := range []int32{i6iHereticAggrID, i6iHereticID} {
		t.Run(strconv.Itoa(int(id)), func(t *testing.T) {
			t.Parallel()
			w := bootIndividual6i(t, raidBoot{}, i6gFaction(i6iTemplate(id)), i6gFaction(warriorTemplate(i6gMateID, nil)))
			me := w.player.ObjectID()
			h := w.i6iArmed(t, id, 300, 0)
			if got := i6dSlot4(h); got != 0 {
				t.Fatalf("int slot 4 as created = %d, want 0", got)
			}
			w.hit(t, h, 10)
			if got := i6dSlot4(h); got != 1 {
				t.Fatalf("int slot 4 after a hit = %d, want 1", got)
			}
			called := w.i6iArmed(t, id, 1500, 1500)
			mate := w.spawn(t, i6gMateID, 1500, 1400)
			w.hit(t, mate, 10)
			if got := i6dSlot4(called); got != 1 {
				t.Fatalf("int slot 4 after a hit on its clan member = %d, want 1", got)
			}

			n := w.i6iArmed(t, id, 0, 300)
			struck := map[modelskill.ID]i6fCast{wLongDDID: {me, 1000000, false}}
			for _, ev := range []struct {
				id    int32
				slot4 int32
			}{{10002, 0}, {10033, 1}} {
				w.onQueue(t, n, func() {
					n.AI().Desires().Clear()
					n.AI().Hates().Clear()
					n.Scratch().SetInt(npc.IntAI4, 0)
					script.NPCOf(n).AddAttackDesire(script.NPCOf(mate), 500)
				})
				w.sendEvent6d(t, n, ev.id, me, 0)
				i6fWantCasts(t, "event "+strconv.Itoa(int(ev.id)), n, struck)
				if got := i6cAttackWeight(n, mate.ObjectID()); got != 0 {
					t.Fatalf("event %d: attack desire on the clan member = %v, want dropped", ev.id, got)
				}
				if got := w.hateListOf(n); got != 200 {
					t.Fatalf("event %d: hate list entry of the character = %v, want 200", ev.id, got)
				}
				if got := i6dSlot4(n); got != ev.slot4 {
					t.Fatalf("event %d: int slot 4 = %d, want %d", ev.id, got, ev.slot4)
				}
			}

			home := script.NPCOf(n).SpawnLocation()
			w.i6iClear(t, n)
			w.sendEvent6d(t, n, 10035, 0, 0)
			if got := i6dSlot4(n); got != 0 {
				t.Fatalf("int slot 4 after event 10035 = %d, want 0", got)
			}
			d, ok := desire(n, actorai.IntentionMoveTo)
			if !ok || d.Weight != 100 || d.Location.X != home.X || d.Location.Y != home.Y {
				t.Fatalf("move desire after event 10035 = %+v (%v), want its spawn point %d,%d with weight 100", d, ok, home.X, home.Y)
			}

			w.i6iClear(t, n)
			w.sendEvent6d(t, n, 10033, 0, 0)
			if got := n.AI().Desires().Snapshot(); len(got) != 0 || i6dSlot4(n) != 0 {
				t.Fatalf("desires and int slot 4 after a call naming no one = %+v, %d, want none and 0", got, i6dSlot4(n))
			}
		})
	}
}

// i6iHereticDen is an npcmaker spawning one NPC of the id it is formatted
// with in a 400 by 400 territory.
const i6iHereticDen = `<?xml version="1.0" encoding="utf-8"?>
<list>
	<territory name="heretic_den" minZ="-1000" maxZ="1000"><node x="0" y="0"/><node x="400" y="0"/><node x="400" y="400"/><node x="0" y="400"/></territory>
	<npcmaker name="heretic_den" territory="heretic_den" maximumNpcs="1">
		<ai type="default_maker"/>
		<npc id="%d" total="1" pos="40;20;30;0" respawn="60sec"/>
	</npcmaker>
</list>`

// TestWizardHereticsWalkHomeFromOutside runs the out-of-territory hooks of
// WizardHereticDDMagic2Aggressive and WizardHereticDDMagicCurse on one its
// npcmaker spawned: arriving outside its territory unengaged, it wants its
// spawn point, weight 100; engaged (int slot 4 set), it stays.
func TestWizardHereticsWalkHomeFromOutside(t *testing.T) {
	t.Parallel()
	for _, id := range []int32{i6iHereticAggrID, i6iHereticID} {
		for _, engaged := range []bool{false, true} {
			t.Run(fmt.Sprintf("%d/%v", id, engaged), func(t *testing.T) {
				t.Parallel()
				dir := t.TempDir()
				if err := os.WriteFile(filepath.Join(dir, "den.xml"), fmt.Appendf(nil, i6iHereticDen, id), 0o644); err != nil {
					t.Fatalf("write spawnlist: %v", err)
				}
				table, err := gamexml.LoadSpawnlist(dir, zerolog.Nop(), 1)
				if err != nil {
					t.Fatalf("load spawnlist: %v", err)
				}
				w := bootIndividual6i(t, raidBoot{opts: []gameservertest.Option{gameservertest.WithNpcSpawns(table)}}, i6iTemplate(id))
				var h *npc.Hostile
				for _, obj := range w.srv.State.Objects() {
					if o, ok := obj.(*npc.Hostile); ok && o.NpcID() == int(id) {
						h = o
					}
				}
				if h == nil {
					t.Fatal("the den spawned no heretic")
				}
				home := script.NPCOf(h).SpawnLocation()
				if engaged {
					w.onQueue(t, h, func() { h.Scratch().SetInt(npc.IntAI4, 1) })
				}

				to := location.Location{X: 40, Y: 700, Z: 30}
				if !h.AI().AddMoveToDesire(to, 1e6) {
					t.Fatalf("AddMoveToDesire(%+v) = false, want the walk queued", to)
				}
				for i := 0; !h.IsMoving(); i++ {
					if i == 5 {
						t.Fatalf("no walk to %+v after five cycles", to)
					}
					cycle(t, w.srv)
				}
				for i := 0; h.IsMoving(); i++ {
					if i >= int(20*time.Second/move.PositionUpdateInterval) {
						t.Fatal("walk never arrived")
					}
					w.srv.TickPositions()
				}
				w.srv.Settle(t)
				if h.InTerritory() {
					t.Fatal("setup: the walk ended inside the territory")
				}
				d, ok := desire(h, actorai.IntentionMoveTo)
				if engaged {
					if ok {
						t.Fatalf("engaged heretic move desire after arriving outside = %+v, want none", d)
					}
					return
				}
				if !ok || d.Weight != 100 || d.Location.X != home.X || d.Location.Y != home.Y {
					t.Fatalf("move desire after arriving outside = %+v (%v), want its spawn point %d,%d with weight 100", d, ok, home.X, home.Y)
				}
			})
		}
	}
}

// TestWizardHereticDDMagicCurseLaysItsFirstDebuffOnHits runs
// WizardHereticDDMagicCurse's attacked hook for hits from 300 away. Its
// parent runs before and after its own part, wanting its long range DD
// magic, each of its two calls running its own parent twice (weight
// 4000000), and its debuff twice (weight 2000000). Hated, on a draw under 33,
// a hit also wants its first debuff on the attacker, weight 1000000, while
// the attacker holds no effect of that debuff; once the character holds
// it, not; on a draw of 33, not. One that cannot pay for the first debuff
// gives up its spells and wants to attack, weight 1000, to which the two
// DD magic answers of its parent's second call add the damage's 125 each.
func TestWizardHereticDDMagicCurseLaysItsFirstDebuffOnHits(t *testing.T) {
	t.Parallel()
	w := bootIndividual6i(t, raidBoot{}, i6iTemplate(i6iHereticID))
	me := w.player.ObjectID()
	parent := map[modelskill.ID]i6fCast{wLongDDID: {me, 4000000, false}, i6gDebuff: {me, 2000000, false}}
	cursed := map[modelskill.ID]i6fCast{wLongDDID: {me, 4000000, false}, i6gDebuff: {me, 2000000, false}, i6hMark: {me, 1000000, false}}
	h := w.i6iArmed(t, i6iHereticID, 300, 0)
	w.hit(t, h, 10)
	i6fWantCasts(t, "unmarked", h, cursed)

	broke := w.i6iArmed(t, i6iHereticID, -300, -300, i6hMark)
	w.hit(t, broke, 10)
	w.i6eGaveUp(t, broke, 1250)

	w.draw.Store(33)
	calm := w.i6iArmed(t, i6iHereticID, 300, 300)
	w.hit(t, calm, 10)
	if d, ok := castDesireOf(calm, i6hMark); ok {
		t.Fatalf("first debuff desire on a draw of 33 = %+v, want none", d)
	}

	w.draw.Store(0)
	w.i6hMarkCharacter(t, h)
	marked := w.i6iArmed(t, i6iHereticID, -300, 0)
	w.hit(t, marked, 10)
	i6fWantCasts(t, "marked", marked, parent)
}

// TestWizardHereticDDMagicCurseDebuffsWhatItSees runs
// WizardHereticDDMagicCurse's see-creature hook: on a draw under 33 it
// wants its first debuff on the character it sees, weight 1000000, and
// nothing else; on a draw of 33 nothing.
func TestWizardHereticDDMagicCurseDebuffsWhatItSees(t *testing.T) {
	t.Parallel()
	for _, c := range []struct {
		draw int32
		want map[modelskill.ID]i6fCast
	}{{0, map[modelskill.ID]i6fCast{i6hMark: {0, 1000000, false}}}, {33, nil}} {
		t.Run(strconv.Itoa(int(c.draw)), func(t *testing.T) {
			t.Parallel()
			w := bootIndividual6i(t, raidBoot{}, i6iTemplate(i6iHereticID))
			w.draw.Store(c.draw)
			h := w.i6iArmed(t, i6iHereticID, 1500, 0)
			w.i6iClear(t, h)
			w.i6fLookAt(t, h)
			for id, cast := range c.want {
				cast.target = w.player.ObjectID()
				c.want[id] = cast
			}
			i6fWantCasts(t, "seeing the character", h, c.want)
			if got := w.hateListOf(h); got != 0 {
				t.Fatalf("hate list entry of the character it saw = %v, want none", got)
			}
		})
	}
}

// TestWizardHereticDDMagicCurseAnswersItsClan runs
// WizardHereticDDMagicCurse's clan-attacked hook for a hit on its clan
// member. Settled with an empty hate list, on a draw under 33, it wants
// its first debuff on the attacker, weight 1000000, beside its parent's
// long range DD magic on the attacker 300 away; fresh, nothing.
func TestWizardHereticDDMagicCurseAnswersItsClan(t *testing.T) {
	t.Parallel()
	w := bootIndividual6i(t, raidBoot{}, i6gFaction(i6iTemplate(i6iHereticID)), i6gFaction(warriorTemplate(i6gMateID, nil)))
	me := w.player.ObjectID()
	h := w.i6iArmed(t, i6iHereticID, 300, 0)
	mate := w.spawn(t, i6gMateID, 150, 150)
	w.i6eSettled(t, h)
	fresh := w.i6iArmed(t, i6iHereticID, 0, 300)
	w.i6iClear(t, fresh)
	w.hit(t, mate, 10)
	i6fWantCasts(t, "settled", h, map[modelskill.ID]i6fCast{wLongDDID: {me, 1000000, false}, i6hMark: {me, 1000000, false}})
	i6fWantCasts(t, "fresh", fresh, nil)

	w.draw.Store(33)
	w.i6iForget(t, h)
	w.hit(t, mate, 10)
	i6fWantCasts(t, "a draw of 33", h, map[modelskill.ID]i6fCast{wLongDDID: {me, 1000000, false}})
}

// TestWizardCorpseNecroDebuffsItsAttackerAndCancelsOnDeath runs
// WizardCorpseNecro's attacked and dying hooks for hits from 300 away,
// after its parent's long range DD magic. With no top desire target it
// adds nothing. Above 90% of its HP with another top desire target, not
// attacking, it wants its debuff on the attacker, weight 1000000, and
// remembers it (creature slot 0). Below 40% with the attacker its top
// desire target, on a draw under 33, it wants its DD magic on it instead.
// Dying 300 from the remembered character it lands nothing; 50 from it,
// its cancel lands on the character.
func TestWizardCorpseNecroDebuffsItsAttackerAndCancelsOnDeath(t *testing.T) {
	t.Parallel()
	w := bootIndividual6i(t, raidBoot{}, i6iTemplate(i6iNecroID), warriorTemplate(i6gFoeID, nil))
	me := w.player.ObjectID()
	foe := w.spawn(t, i6gFoeID, 0, 2500)
	h := w.i6iArmed(t, i6iNecroID, 300, 0)
	w.hit(t, h, 10)
	i6fWantCasts(t, "no top desire target", h, map[modelskill.ID]i6fCast{wLongDDID: {me, 1000000, false}})
	if got := script.NPCOf(h).Slot(script.SlotCreature0); got != nil {
		t.Fatalf("creature slot 0 with no top desire target = %v, want none", got)
	}

	w.onQueue(t, h, func() { h.AI().Desires().Clear(); script.NPCOf(h).SetTopDesireTarget(script.NPCOf(foe)) })
	w.hit(t, h, 10)
	i6fWantCasts(t, "above 90%", h, map[modelskill.ID]i6fCast{wLongDDID: {me, 1000000, false}, i6gDebuff: {me, 1000000, false}})
	if got := script.NPCOf(h).Slot(script.SlotCreature0); got == nil || got.ObjectID() != me {
		t.Fatalf("creature slot 0 after a hit above 90%% = %v, want the character", got)
	}

	w.draw.Store(99)
	w.hit(t, h, int(h.MaxHP())*7/10)
	w.onQueue(t, h, func() { h.AI().Desires().Clear(); script.NPCOf(h).SetTopDesireTarget(script.PlayerOf(w.player)) })
	w.draw.Store(0)
	w.hit(t, h, 10)
	i6fWantCasts(t, "below 40%", h, map[modelskill.ID]i6fCast{wLongDDID: {me, 1000000, false}, i6iDDMagic: {me, 1000000, false}})

	// Its dying hooks run 3 s after its death.
	ref := modelskill.Ref{ID: i6iCancel, Level: 1}
	cancelLevel := func() int32 {
		var level int32
		w.srv.RunScript(t, me, "I6dDirector", func(s *script.Script, p *script.Player) {
			level = s.AbnormalLevel(p, ref)
		})
		return level
	}
	w.onQueue(t, h, func() { script.NPCOf(h).DoDie(nil) })
	w.srv.Advance(t, 4*time.Second)
	if got := cancelLevel(); got > 0 {
		t.Fatalf("cancel level after a death 300 away = %d, want none", got)
	}

	near := w.i6iArmed(t, i6iNecroID, 50, 0)
	w.onQueue(t, near, func() { script.NPCOf(near).SetSlot(script.SlotCreature0, script.PlayerOf(w.player)) })
	w.onQueue(t, near, func() { script.NPCOf(near).DoDie(nil) })
	w.srv.AdvanceUntil(t, "the cancel landed", func() bool { return cancelLevel() > 0 })
}
