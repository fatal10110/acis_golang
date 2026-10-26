package ai

import (
	"fmt"
	"slices"
	"strconv"
	"sync/atomic"
	"testing"
	"time"

	"github.com/fatal10110/acis_golang/internal/gameserver/model/actor/npc"
	modelskill "github.com/fatal10110/acis_golang/internal/gameserver/model/skill"
	"github.com/fatal10110/acis_golang/internal/gameserver/script"
	"github.com/fatal10110/acis_golang/internal/gameserver/script/ai/defaultnpc"
	"github.com/fatal10110/acis_golang/internal/gameserver/script/ai/monster"
	"github.com/fatal10110/acis_golang/internal/gameserver/script/ai/wizard"
	"github.com/fatal10110/acis_golang/internal/gameservertest"
)

// The NPC ids of the tenth PR of the sixth individual behavior batch: the
// datapack ids each class binds (WizardCorpseVampireBasicAggressive binds
// none; its child stands for it).
const (
	i6jNecroAggrID       = int32(21581) // WizardCorpseNecroAggressive
	i6jSummonID          = int32(21563) // WizardCorpseNecroSummon
	i6jSummonAggrID      = int32(21559) // WizardCorpseNecroSummonAggressive
	i6jVampireID         = int32(21576) // WizardCorpseVampireBasic
	i6jVampireCurseID    = int32(21585) // WizardCorpseVampireCurseAggressive
	i6jVampireRangeID    = int32(21591) // WizardCorpseVampireRange
	i6jVampireRangeAggID = int32(21586) // WizardCorpseVampireRangeAggressive
	i6jRaisedID          = int32(21548) // the necromancer's SummonPrivate
)

// i6jSelfRange is the fixture self range DD magic.
const i6jSelfRange = modelskill.ID(9480)

// individual6jScripts are the behavior chain down to WizardCorpseNecro and
// the classes of the PR in scripts.xml order.
func individual6jScripts() ([]script.Listing, script.Catalog) {
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
		{p + "WizardCorpseNecro.WizardCorpseNecro", wizard.CorpseNecro},
		{p + "WizardCorpseNecro.WizardCorpseNecroAggressive", wizard.CorpseNecroAggressive},
		{p + "WizardCorpseNecro.WizardCorpseNecroSummon", wizard.CorpseNecroSummon},
		{p + "WizardCorpseNecro.WizardCorpseNecroSummonAggressive", wizard.CorpseNecroSummonAggressive},
		{p + "WizardCorpseVampireBasic.WizardCorpseVampireBasic", wizard.CorpseVampireBasic},
		{p + "WizardCorpseVampireBasic.WizardCorpseVampireBasicAggressive", wizard.CorpseVampireBasicAggressive},
		{p + "WizardCorpseVampireBasic.WizardCorpseVampireCurseAggressive", wizard.CorpseVampireCurseAggressive},
		{p + "WizardCorpseVampireBasic.WizardCorpseVampireRange", wizard.CorpseVampireRange},
		{p + "WizardCorpseVampireBasic.WizardCorpseVampireRangeAggressive", wizard.CorpseVampireRangeAggressive},
	}
	var list []script.Listing
	catalog := script.Catalog{}
	for _, e := range entries {
		list = append(list, script.Listing{Path: e.path})
		catalog[e.path] = e.ctor
	}
	return list, catalog
}

// bootIndividual6j boots the PR's classes beside templates, every NPC
// casting i6jSkills, every draw the current value of the world's draw.
func bootIndividual6j(t *testing.T, templates ...*npc.Template) *individual6World {
	t.Helper()
	list, catalog := individual6jScripts()
	kinds := map[int32]script.NPCKind{}
	for _, tmpl := range templates {
		kinds[int32(tmpl.ID)] = script.KindHostile
	}
	draw := new(atomic.Int32)
	extra := raidBoot{opts: []gameservertest.Option{
		gameservertest.WithNPCScripts(kinds, list, catalog),
		gameservertest.WithScriptRand(func(n int) int { return min(int(draw.Load()), n-1) }),
		gameservertest.WithNpcSpawnSkills(i6jSkills()),
	}}
	return &individual6World{warriorWorld: bootRaid(t, extra, templates...), draw: draw}
}

// i6jTemplate is a fixture wizard of id listing every skill type the PR's
// classes cast, the mark as its first debuff, raising i6jRaisedID.
func i6jTemplate(id int32) *npc.Template {
	tmpl := i6iTemplate(id)
	tmpl.AIParams = npc.AIParams{"SummonPrivate": fmt.Sprint(i6jRaisedID)}
	tmpl.SkillsByType[npc.SkillTypeClearCorpse] = modelskill.Ref{ID: i6bClearCorpseID, Level: 1}
	tmpl.SkillsByType[npc.SkillTypeWSelfRangeDDMagic] = modelskill.Ref{ID: i6jSelfRange, Level: 1}
	return tmpl
}

// i6jSkills are the fixture skills of the PR's NPCs: harmless one-target
// magics, the cancel among them, the mark and the corpse clearing, each costing
// nothing, or 100000 MP for the costly ones.
func i6jSkills(costly ...modelskill.ID) *modelskill.Table {
	defs := []modelskill.Definition{i6hMarkSkill(), npcSkill(i6bClearCorpseID, 1, false)}
	for _, id := range []modelskill.ID{wLongDDID, wShortDDID, i6gDebuff, i6iDDMagic, i6iCancel, i6jSelfRange} {
		defs = append(defs, harmlessSkill(id))
	}
	for i := range defs {
		if slices.Contains(costly, defs[i].ID) {
			defs[i].MPConsume = 100000
		}
	}
	return modelskill.NewTable(defs)
}

// i6jArmed spawns an NPC of id dx, dy from the character, its skills
// i6jSkills with costly ones.
func (w *individual6World) i6jArmed(t *testing.T, id int32, dx, dy int, costly ...modelskill.ID) *npc.Hostile {
	t.Helper()
	h := w.spawn(t, id, dx, dy)
	w.srv.InstallNPCCastSeam(t, h, i6jSkills(costly...))
	return h
}

// TestWizardCorpseAggressivesStrikeOnSight runs the see-creature hooks of
// WizardCorpseNecroAggressive, WizardCorpseNecroSummonAggressive,
// WizardCorpseVampireBasicAggressive (through its cursing child) and
// WizardCorpseVampireRangeAggressive. Settled in its territory with nobody
// on its hate list, each wants its short range DD magic on the character
// it sees 100 away, weight 1000000, and hates it; one that cannot pay
// gives up its spells and wants to attack, weight 1000, instead. Hating
// another, or fresh, it does nothing.
func TestWizardCorpseAggressivesStrikeOnSight(t *testing.T) {
	t.Parallel()
	for _, id := range []int32{i6jNecroAggrID, i6jSummonAggrID, i6jVampireCurseID, i6jVampireRangeAggID} {
		t.Run(strconv.Itoa(int(id)), func(t *testing.T) {
			t.Parallel()
			w := bootIndividual6j(t, i6jTemplate(id), warriorTemplate(i6gFoeID, nil))
			me := w.player.ObjectID()
			settled := w.i6jArmed(t, id, 1500, 0)
			broke := w.i6jArmed(t, id, -1500, 0, wShortDDID)
			busy := w.i6jArmed(t, id, 0, 1500)
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

			fresh := w.i6jArmed(t, id, 2500, 0)
			w.i6fLookAt(t, fresh)
			i6fWantCasts(t, "fresh", fresh, nil)
		})
	}
}

// TestWizardCorpseNecroSummonRaisesTheDead runs WizardCorpseNecroSummon's
// created, timer and see-creature hooks: its 5 s timer "3456" looks
// around, and the look sees a dead NPC beside it. Hating someone, on a
// draw under 30, with the corpse within 200, it casts its corpse clearing
// at once and places its SummonPrivate on the corpse, told (1000, the
// corpse). A draw of 30, an empty hate list, a corpse 260 away or a
// template without the corpse clearing raise nothing and cast nothing.
func TestWizardCorpseNecroSummonRaisesTheDead(t *testing.T) {
	t.Parallel()
	cases := []struct {
		name    string
		draw    int32
		hates   bool
		apart   int
		noClear bool
		raises  bool
	}{
		{name: "raises", hates: true, apart: 100, raises: true},
		{name: "draw of 30", draw: 30, hates: true, apart: 100},
		{name: "hating nobody", apart: 100},
		{name: "corpse 260 away", hates: true, apart: 260},
		{name: "no corpse clearing", hates: true, apart: 100, noClear: true},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			t.Parallel()
			tmpl := i6jTemplate(i6jSummonID)
			if c.noClear {
				delete(tmpl.SkillsByType, npc.SkillTypeClearCorpse)
			}
			corpseTmpl := warriorTemplate(i6gFoeID, nil)
			corpseTmpl.CorpseTime = 60
			w := bootIndividual6j(t, tmpl, warriorTemplate(i6jRaisedID, nil), corpseTmpl)
			w.draw.Store(c.draw)
			h := w.i6jArmed(t, i6jSummonID, 1000, 0)
			corpse := w.spawn(t, i6gFoeID, 1000, c.apart)
			w.srv.Advance(t, 4*time.Second)
			w.onQueue(t, corpse, func() { script.NPCOf(corpse).DoDie(nil) })
			if c.hates {
				w.onQueue(t, h, func() { script.NPCOf(h).HateList().Add(script.PlayerOf(w.player), 100) })
			}
			w.srv.ReadQueued(t, w.srv.Client)

			w.srv.Advance(t, time.Second)
			frames := w.srv.ReadQueued(t, w.srv.Client)
			cleared := slices.Contains(skillUses(frames, h.ObjectID()), i6bClearCorpseID)
			if !c.raises {
				privatesOf(t, h, 0)
				if cleared {
					t.Fatal("the necromancer cleared the corpse")
				}
				return
			}
			if !cleared {
				t.Fatalf("the necromancer's casts after the look = %v, want its corpse clearing", skillUses(frames, h.ObjectID()))
			}
			raised := privatesOf(t, h, 1)[0]
			if got := int32(raised.Instance.Template.ID); got != i6jRaisedID {
				t.Fatalf("raised private = %d, want %d", got, i6jRaisedID)
			}
			cx, cy, _ := corpse.Position()
			if rx, ry, _ := raised.Position(); rx != cx || ry != cy {
				t.Fatalf("raised private at %d,%d, want on the corpse at %d,%d", rx, ry, cx, cy)
			}
			if got := [2]int32{raised.Scratch().Int(npc.IntParam1), raised.Scratch().Int(npc.IntParam2)}; got != [2]int32{1000, corpse.ObjectID()} {
				t.Fatalf("raised private spawn parameters = %v, want 1000 and the corpse %d", got, corpse.ObjectID())
			}
		})
	}
}

// TestWizardCorpseNecroSummonWantsToAttackByTheDamage runs
// WizardCorpseNecroSummon's attacked hook beside its parent's: a hit of 80
// from the character on the level 1 necromancer, which hates no class,
// adds 80 / (1 + 7) * 100 = 1000 to the attack desire its parent's hook
// leaves on the character.
func TestWizardCorpseNecroSummonWantsToAttackByTheDamage(t *testing.T) {
	t.Parallel()
	w := bootIndividual6j(t, i6jTemplate(i6jSummonID), i6jTemplate(i6iNecroID))
	me := w.player.ObjectID()
	summon := w.i6jArmed(t, i6jSummonID, 300, 0)
	parent := w.i6jArmed(t, i6iNecroID, -300, 0)
	w.hit(t, summon, 80)
	w.hit(t, parent, 80)
	if got, base := i6cAttackWeight(summon, me), i6cAttackWeight(parent, me); got != base+1000 {
		t.Fatalf("attack desire on the character = %v, want its parent's %v + 1000", got, base)
	}
}

// TestWizardCorpseVampireBasicDebuffsAndStrikesOnce runs
// WizardCorpseVampireBasic's attacked hook for hits from 300 away, after
// its parent's long range DD magic. With no top desire target it adds
// nothing. Above 90% of its HP with another top desire target, not
// attacking, it wants its debuff on the attacker, weight 1000000. Below
// 50% with the attacker its top desire target, on a draw under 33, it
// wants its DD magic on it and gives up its spells (int slot 0), so its
// parent wants no DD magic; a later such hit wants no DD magic again.
func TestWizardCorpseVampireBasicDebuffsAndStrikesOnce(t *testing.T) {
	t.Parallel()
	w := bootIndividual6j(t, i6jTemplate(i6jVampireID), warriorTemplate(i6gFoeID, nil))
	me := w.player.ObjectID()
	foe := w.spawn(t, i6gFoeID, 0, 2500)
	h := w.i6jArmed(t, i6jVampireID, 300, 0)
	if got := h.Scratch().Int(npc.IntAI0); got != 0 {
		t.Fatalf("int slot 0 once created = %d, want 0", got)
	}
	w.hit(t, h, 10)
	i6fWantCasts(t, "no top desire target", h, map[modelskill.ID]i6fCast{wLongDDID: {me, 1000000, false}})

	w.onQueue(t, h, func() { h.AI().Desires().Clear(); script.NPCOf(h).SetTopDesireTarget(script.NPCOf(foe)) })
	w.hit(t, h, 10)
	i6fWantCasts(t, "above 90%", h, map[modelskill.ID]i6fCast{wLongDDID: {me, 1000000, false}, i6gDebuff: {me, 1000000, false}})

	w.draw.Store(99)
	w.hit(t, h, int(h.MaxHP())*6/10)
	w.onQueue(t, h, func() { h.AI().Desires().Clear(); script.NPCOf(h).SetTopDesireTarget(script.PlayerOf(w.player)) })
	w.draw.Store(0)
	w.hit(t, h, 10)
	i6fWantCasts(t, "below 50%", h, map[modelskill.ID]i6fCast{i6iDDMagic: {me, 1000000, false}})
	if got := h.Scratch().Int(npc.IntAI0); got != 1 {
		t.Fatalf("int slot 0 after the DD magic = %d, want 1", got)
	}

	w.onQueue(t, h, func() { h.AI().Desires().Clear(); script.NPCOf(h).SetTopDesireTarget(script.PlayerOf(w.player)) })
	w.hit(t, h, 10)
	i6fWantCasts(t, "below 50% once more", h, nil)
}

// TestWizardCorpseVampireCurseAggressiveCursesThenCancels runs
// WizardCorpseVampireCurseAggressive's created and attacked hooks. The
// first hit wants its first debuff on the character, weight 1000000, and
// remembers the character (creature slot 0, int slot 4 at 1). Later hits
// want no first debuff; below 10% of its HP, the character 300 away, no
// cancel; 50 away, its cancel on the character, weight 1000000 (int slot
// 4 at 2), and never again.
func TestWizardCorpseVampireCurseAggressiveCursesThenCancels(t *testing.T) {
	t.Parallel()
	w := bootIndividual6j(t, i6jTemplate(i6jVampireCurseID))
	me := w.player.ObjectID()
	h := w.i6jArmed(t, i6jVampireCurseID, 300, 0)
	stage := func() int32 { return h.Scratch().Int(npc.IntAI4) }
	clearCasts := func() { w.onQueue(t, h, func() { h.AI().Desires().Clear() }) }
	if got := stage(); got != 0 {
		t.Fatalf("int slot 4 once created = %d, want 0", got)
	}

	w.hit(t, h, 10)
	if !i6bCastWeighs(h, i6hMark, me, 1000000) {
		d, ok := castDesireOf(h, i6hMark)
		t.Fatalf("first debuff desire = %+v (%v), want on the character, weight 1000000", d, ok)
	}
	if got := script.NPCOf(h).Slot(script.SlotCreature0); stage() != 1 || got == nil || got.ObjectID() != me {
		t.Fatalf("after the first hit: int slot 4 = %d, creature slot 0 = %v, want 1 and the character", stage(), got)
	}

	clearCasts()
	w.hit(t, h, 10)
	for _, id := range []modelskill.ID{i6hMark, i6iCancel} {
		if d, ok := castDesireOf(h, id); ok {
			t.Fatalf("cast desire of %d on a later hit at full HP = %+v, want none", id, d)
		}
	}

	w.onQueue(t, h, func() { h.SetHP(h.MaxHPValue() * 0.08); h.AI().Desires().Clear() })
	w.hit(t, h, 10)
	if d, ok := castDesireOf(h, i6iCancel); ok || stage() != 1 {
		t.Fatalf("below 10%% 300 away: cancel desire %+v (%v), int slot 4 = %d, want none and 1", d, ok, stage())
	}

	w.moveNextTo(t, h, 50, 0)
	clearCasts()
	w.hit(t, h, 10)
	if !i6bCastWeighs(h, i6iCancel, me, 1000000) || stage() != 2 {
		d, ok := castDesireOf(h, i6iCancel)
		t.Fatalf("below 10%% 50 away: cancel desire %+v (%v), int slot 4 = %d, want on the character, weight 1000000, and 2", d, ok, stage())
	}

	clearCasts()
	w.hit(t, h, 10)
	if d, ok := castDesireOf(h, i6iCancel); ok {
		t.Fatalf("cancel desire after the cancel = %+v, want none", d)
	}
}

// TestWizardCorpseVampireRangeStrikesAPartyMember runs
// WizardCorpseVampireRange's attacked hook for hits from 300 away, with
// another top desire target, beside its parents' debuff and long range DD
// magic. Out of a party the character draws nothing more. In a party, on a
// draw under 33, it wants its self range DD magic on the character,
// unchecked, weight 1000000; on a draw of 33, not. One that cannot pay
// gives up its spells and wants to attack, weight 1000, instead, which its
// parent then raises by the damage.
func TestWizardCorpseVampireRangeStrikesAPartyMember(t *testing.T) {
	t.Parallel()
	w := bootIndividual6j(t, i6jTemplate(i6jVampireRangeID), warriorTemplate(i6gFoeID, nil))
	me := w.player.ObjectID()
	foe := w.spawn(t, i6gFoeID, 0, 2500)
	h := w.i6jArmed(t, i6jVampireRangeID, 300, 0)
	broke := w.i6jArmed(t, i6jVampireRangeID, -300, 0, i6jSelfRange)
	aim := func(n *npc.Hostile) {
		w.onQueue(t, n, func() { n.AI().Desires().Clear(); script.NPCOf(n).SetTopDesireTarget(script.NPCOf(foe)) })
	}
	parents := map[modelskill.ID]i6fCast{wLongDDID: {me, 1000000, false}, i6gDebuff: {me, 1000000, false}}

	aim(h)
	w.hit(t, h, 10)
	i6fWantCasts(t, "out of a party", h, parents)

	w.joinMate(t)
	w.draw.Store(33)
	aim(h)
	w.hit(t, h, 10)
	i6fWantCasts(t, "in a party on a draw of 33", h, parents)

	w.draw.Store(0)
	aim(h)
	w.hit(t, h, 10)
	want := map[modelskill.ID]i6fCast{i6jSelfRange: {me, 1000000, false}}
	for id, c := range parents {
		want[id] = c
	}
	i6fWantCasts(t, "in a party", h, want)

	aim(broke)
	w.hit(t, broke, 80)
	w.i6eGaveUp(t, broke, 1000+80.0/8*100)
}
