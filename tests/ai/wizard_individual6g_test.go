package ai

import (
	"context"
	"sync/atomic"
	"testing"
	"time"

	gamesql "github.com/fatal10110/acis_golang/internal/gameserver/data/sql"
	"github.com/fatal10110/acis_golang/internal/gameserver/data/sql/sqltest"
	actorai "github.com/fatal10110/acis_golang/internal/gameserver/model/actor/ai"
	"github.com/fatal10110/acis_golang/internal/gameserver/model/actor/npc"
	modelskill "github.com/fatal10110/acis_golang/internal/gameserver/model/skill"
	"github.com/fatal10110/acis_golang/internal/gameserver/network/serverpackets"
	"github.com/fatal10110/acis_golang/internal/gameserver/script"
	"github.com/fatal10110/acis_golang/internal/gameserver/script/ai/defaultnpc"
	"github.com/fatal10110/acis_golang/internal/gameserver/script/ai/monster"
	"github.com/fatal10110/acis_golang/internal/gameserver/script/ai/wizard"
	skillstate "github.com/fatal10110/acis_golang/internal/gameserver/skill"
	"github.com/fatal10110/acis_golang/internal/gameservertest"
)

// The NPC ids of the seventh PR of the sixth individual behavior batch: the
// datapack ids each class binds, and two fixtures with no behavior.
const (
	i6gCurseStoneID = int32(21338) // WizardCastClanBuffCurseStoneAggressive
	i6gRangeCurseID = int32(21430) // WizardCastClanBuffRangeCurse
	i6gRangeDDID    = int32(21334) // WizardCastClanBuffRangeDDCurseStoneAggressive
	i6gAggressiveID = int32(21311) // WizardDDMagic2Aggressive
	i6gDispellID    = int32(21663) // WizardDDMagic2DispellAggressive
	i6gHallID       = int32(18330) // WizardHallDDMagic2Aggressive
	i6gMateID       = int32(29570) // no behavior
	i6gFoeID        = int32(29571) // no behavior
)

// The skills the PR's classes list by type, beside the batch's long and
// short range DD magics.
const (
	i6gBuff1       = modelskill.ID(9430)
	i6gBuff2       = modelskill.ID(9431)
	i6gDebuff      = modelskill.ID(9432)
	i6gRangeDebuff = modelskill.ID(9433)
	i6gRangeDD     = modelskill.ID(9434)
	i6gDispell     = modelskill.ID(9435)
	i6gClericBuff  = modelskill.ID(9436)
)

// individual6gScripts are the behavior chain down to WizardDDMagic2, the
// classes of the PR in scripts.xml order, and a director the tests send
// script events through.
func individual6gScripts() ([]script.Listing, script.Catalog) {
	entries := []struct {
		path string
		ctor func() script.Script
	}{
		{"script.ai.individual.DefaultNpc", defaultnpc.New},
		{"script.ai.individual.Monster.MonsterAI", monster.New},
		{"script.ai.individual.Monster.WizardBase.WizardBase", wizard.Base},
		{"script.ai.individual.Monster.WizardBase.Wizard.Wizard", wizard.Wizard},
		{"script.ai.individual.Monster.WizardBase.Wizard.WizardDDMagic2.WizardCastClanBuffCurseStoneAggressive", wizard.CastClanBuffCurseStoneAggressive},
		{"script.ai.individual.Monster.WizardBase.Wizard.WizardDDMagic2.WizardCastClanBuffRangeCurse", wizard.CastClanBuffRangeCurse},
		{"script.ai.individual.Monster.WizardBase.Wizard.WizardDDMagic2.WizardCastClanBuffRangeDDCurseStoneAggressive", wizard.CastClanBuffRangeDDCurseStoneAggressive},
		{"script.ai.individual.Monster.WizardBase.Wizard.WizardDDMagic2.WizardDDMagic2", wizard.DDMagic2},
		{"script.ai.individual.Monster.WizardBase.Wizard.WizardDDMagic2.WizardDDMagic2Aggressive", wizard.DDMagic2Aggressive},
		{"script.ai.individual.Monster.WizardBase.Wizard.WizardDDMagic2.WizardDDMagic2DispellAggressive", wizard.DDMagic2DispellAggressive},
		{"script.ai.individual.Monster.WizardBase.Wizard.WizardDDMagic2.WizardHallDDMagic2Aggressive", wizard.HallDDMagic2Aggressive},
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

func bootIndividual6g(t *testing.T, extra raidBoot, templates ...*npc.Template) *individual6World {
	t.Helper()
	list, catalog := individual6gScripts()
	kinds := map[int32]script.NPCKind{}
	for _, tmpl := range templates {
		kinds[int32(tmpl.ID)] = script.KindHostile
	}
	draw := new(atomic.Int32)
	extra.opts = append(extra.opts,
		gameservertest.WithNPCScripts(kinds, list, catalog),
		gameservertest.WithScriptRand(func(n int) int { return min(int(draw.Load()), n-1) }),
	)
	return &individual6World{warriorWorld: bootRaid(t, extra, templates...), draw: draw}
}

// i6gTemplate is a fixture wizard of id listing every skill type the PR's
// classes cast.
func i6gTemplate(id int32, ai npc.AIParams) *npc.Template {
	return i6bTemplate(id, ai, map[npc.SkillType]modelskill.ID{
		npc.SkillTypeWLongRangeDDMagic: wLongDDID, npc.SkillTypeWShortRangeDDMagic: wShortDDID,
		npc.SkillTypeBuff1: i6gBuff1, npc.SkillTypeBuff2: i6gBuff2, npc.SkillTypeDebuff: i6gDebuff,
		npc.SkillTypeRangeDebuff: i6gRangeDebuff, npc.SkillTypeRangeDD: i6gRangeDD, npc.SkillTypeDispell: i6gDispell,
	})
}

// i6gFaction puts tmpl in the Ketra clan, with a clan range of 500.
func i6gFaction(tmpl *npc.Template) *npc.Template {
	tmpl.Clans, tmpl.ClanRange = []string{"ketra_orc_clan"}, 500
	return tmpl
}

// i6gArmed spawns an NPC of id dx, dy from the character, its one-target
// skills harmless magics and its buffs and range debuff self buffs, each
// costing nothing, or 100000 MP for the costly ones.
func (w *individual6World) i6gArmed(t *testing.T, id int32, dx, dy int, costly ...modelskill.ID) *npc.Hostile {
	t.Helper()
	h := w.spawn(t, id, dx, dy)
	w.srv.InstallNPCCastSeam(t, h, i6gSkills(costly...))
	return h
}

// i6gSkills are the fixture skills of i6gArmed.
func i6gSkills(costly ...modelskill.ID) *modelskill.Table {
	defs := []modelskill.Definition{npcSkill(petrifyID, 1, true)}
	for _, id := range []modelskill.ID{wLongDDID, wShortDDID, i6gDebuff, i6gRangeDD, i6gDispell} {
		defs = append(defs, harmlessSkill(id))
	}
	for _, id := range []modelskill.ID{i6gBuff1, i6gBuff2, i6gRangeDebuff} {
		defs = append(defs, npcSkill(id, 1, false))
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

// TestWizardCurseStonePetrifiesAllies runs
// WizardCastClanBuffCurseStoneAggressive's see-creature, attacked and
// clan-attacked hooks for a Ketra character. It is not attacked on sight;
// its hit makes the stone want to petrify it (4515, weight 1000000 from the
// attacked hook and as much again from its own clan call), stop hating it
// and cast no buff nor DD magic. A character with no alliance is attacked
// on sight with weight 200.
func TestWizardCurseStonePetrifiesAllies(t *testing.T) {
	t.Parallel()
	w := bootIndividual6g(t, raidBoot{}, i6gFaction(i6gTemplate(i6gCurseStoneID, nil)))
	ch := i6bCharacter(t, w)
	ch.SetVarkaKetraAlliance(1)
	allied := w.i6gArmed(t, i6gCurseStoneID, 150, 0)
	w.ticks(t, 2)
	if got := allied.AggroHate(w.player); got != 0 {
		t.Fatalf("hate of an ally it sees = %v, want none", got)
	}
	w.hit(t, allied, 10)
	i6fWantCasts(t, "after an ally's hit", allied, map[modelskill.ID]i6fCast{petrifyID: {w.player.ObjectID(), 2000000, false}})
	if got := allied.AggroHate(w.player); got != 0 {
		t.Fatalf("hate of the ally that hit it = %v, want none", got)
	}

	ch.SetVarkaKetraAlliance(0)
	open := w.i6gArmed(t, i6gCurseStoneID, 0, 150)
	w.ticks(t, 2)
	if got := open.AggroHate(w.player); got != 200 {
		t.Fatalf("hate of a character with no alliance it sees = %v, want 200", got)
	}
}

// TestWizardCurseStoneBuffsAndCursesItsAttacker runs
// WizardCastClanBuffCurseStoneAggressive's attacked hook, every draw 0, for
// a character with no alliance hitting from 300 away. Its parent runs
// before and after its own part: the character fills its hate list and
// draws the long range DD magic twice (weight 2000000). The first hit,
// above half of its HP, wants both buffs on itself and sets int slot 2, and
// wants its debuff on its most hated attacker, all weight 1000000; a later
// hit wants no buff. One that cannot pay for its debuff gives up its
// spells and wants to attack, weight 1000, to which its parent's second
// call adds the damage's 125.
func TestWizardCurseStoneBuffsAndCursesItsAttacker(t *testing.T) {
	t.Parallel()
	w := bootIndividual6g(t, raidBoot{}, i6gFaction(i6gTemplate(i6gCurseStoneID, nil)))
	me := w.player.ObjectID()
	h := w.i6gArmed(t, i6gCurseStoneID, 300, 0)
	w.hit(t, h, 10)
	i6fWantCasts(t, "first hit", h, map[modelskill.ID]i6fCast{
		wLongDDID: {me, 2000000, false}, i6gBuff1: {h.ObjectID(), 1000000, false},
		i6gBuff2: {h.ObjectID(), 1000000, false}, i6gDebuff: {me, 1000000, false},
	})
	if got := h.Scratch().Int(npc.IntAI2); got != 1 {
		t.Fatalf("int slot 2 after the buffs = %d, want 1", got)
	}
	w.onQueue(t, h, func() { h.AI().Desires().Clear() })
	w.hit(t, h, 10)
	i6fWantCasts(t, "second hit", h, map[modelskill.ID]i6fCast{wLongDDID: {me, 2000000, false}, i6gDebuff: {me, 1000000, false}})

	broke := w.i6gArmed(t, i6gCurseStoneID, -300, 0, i6gDebuff)
	w.hit(t, broke, 10)
	w.i6eGaveUp(t, broke, 1125)
}

// TestWizardCurseStoneAnswersItsClan runs
// WizardCastClanBuffCurseStoneAggressive's clan-attacked hook, every draw
// 0. Settled, a hit on its clan member by a character with no alliance
// wants both buffs and its debuff on itself, weight 1000000, and, through
// its parent, its long range DD magic on the character 300 away. A Ketra
// character's hit on the member wants the character petrified and nothing
// else.
func TestWizardCurseStoneAnswersItsClan(t *testing.T) {
	t.Parallel()
	w := bootIndividual6g(t, raidBoot{}, i6gFaction(i6gTemplate(i6gCurseStoneID, nil)), i6gFaction(warriorTemplate(i6gMateID, nil)))
	me := w.player.ObjectID()
	h := w.i6gArmed(t, i6gCurseStoneID, 300, 0)
	mate := w.spawn(t, i6gMateID, 150, 150)
	w.i6eSettled(t, h)
	w.hit(t, mate, 10)
	i6fWantCasts(t, "settled, after a hit on its clan member", h, map[modelskill.ID]i6fCast{
		wLongDDID: {me, 1000000, false}, i6gBuff1: {h.ObjectID(), 1000000, false},
		i6gBuff2: {h.ObjectID(), 1000000, false}, i6gDebuff: {h.ObjectID(), 1000000, false},
	})

	w.onQueue(t, h, func() { h.AI().Desires().Clear(); h.AI().Hates().Clear() })
	i6bCharacter(t, w).SetVarkaKetraAlliance(1)
	w.hit(t, mate, 10)
	i6fWantCasts(t, "after an ally's hit on its clan member", h, map[modelskill.ID]i6fCast{petrifyID: {me, 1000000, false}})
}

// TestWizardRangeCurseBuffsAndCursesAround runs
// WizardCastClanBuffRangeCurse's attacked and clan-attacked hooks, every
// draw 0. The first playable hit, above half of its HP, wants both buffs
// on itself, weight 1000000, and sets int slot 3; with the character its
// top desire target no range debuff. With another top desire target a hit
// wants its range debuff on itself; one that cannot pay for it gives up
// its spells and wants to attack. Settled, a hit on its clan member wants
// its range debuff on itself whatever its top desire target.
func TestWizardRangeCurseBuffsAndCursesAround(t *testing.T) {
	t.Parallel()
	w := bootIndividual6g(t, raidBoot{}, inClan(i6gTemplate(i6gRangeCurseID, nil)), inClan(warriorTemplate(i6gMateID, nil)), warriorTemplate(i6gFoeID, nil))
	me := w.player.ObjectID()
	h := w.i6gArmed(t, i6gRangeCurseID, 300, 0)
	foe := w.spawn(t, i6gFoeID, 1500, 0)
	w.hit(t, h, 10)
	i6fWantCasts(t, "first hit", h, map[modelskill.ID]i6fCast{
		wLongDDID: {me, 1000000, false}, i6gBuff1: {h.ObjectID(), 1000000, false}, i6gBuff2: {h.ObjectID(), 1000000, false},
	})
	if got := h.Scratch().Int(npc.IntAI3); got != 1 {
		t.Fatalf("int slot 3 after the buffs = %d, want 1", got)
	}

	w.onQueue(t, h, func() {
		h.AI().Desires().Clear()
		script.NPCOf(h).SetTopDesireTarget(script.NPCOf(foe))
	})
	w.hit(t, h, 10)
	i6fWantCasts(t, "hit with another top desire target", h, map[modelskill.ID]i6fCast{
		wLongDDID: {me, 1000000, false}, i6gRangeDebuff: {h.ObjectID(), 1000000, false},
	})

	broke := w.i6gArmed(t, i6gRangeCurseID, -300, 0, i6gRangeDebuff)
	w.onQueue(t, broke, func() { script.NPCOf(broke).SetTopDesireTarget(script.NPCOf(foe)) })
	w.hit(t, broke, 10)
	if got := broke.Scratch().Int(npc.IntAI0); got != 1 {
		t.Fatalf("short of MP int slot 0 = %d, want 1", got)
	}
	if d, ok := castDesireOf(broke, i6gRangeDebuff); ok {
		t.Fatalf("short of MP range debuff desire = %+v, want none", d)
	}

	clan := w.i6gArmed(t, i6gRangeCurseID, 0, 300)
	mate := w.spawn(t, i6gMateID, 150, 150)
	w.i6eSettled(t, clan)
	w.hit(t, mate, 10)
	i6fWantCasts(t, "settled, after a hit on its clan member", clan, map[modelskill.ID]i6fCast{
		wLongDDID: {me, 1000000, false}, i6gBuff1: {clan.ObjectID(), 1000000, false},
		i6gBuff2: {clan.ObjectID(), 1000000, false}, i6gRangeDebuff: {clan.ObjectID(), 1000000, false},
	})
}

// TestWizardRangeDDCurseStoneStrikesAndPetrifies runs
// WizardCastClanBuffRangeDDCurseStoneAggressive's attacked and
// clan-attacked hooks. With another top desire target, on a draw under 3,
// a hit wants its range DD magic on the attacker, weight 1000000; on a
// draw of 3 it does not. A Ketra character's hit, on it or on its clan
// member, wants the character petrified and no DD magic.
func TestWizardRangeDDCurseStoneStrikesAndPetrifies(t *testing.T) {
	t.Parallel()
	w := bootIndividual6g(t, raidBoot{}, i6gFaction(i6gTemplate(i6gRangeDDID, nil)), i6gFaction(warriorTemplate(i6gMateID, nil)), warriorTemplate(i6gFoeID, nil))
	me := w.player.ObjectID()
	foe := w.spawn(t, i6gFoeID, 1500, 0)
	for _, row := range []struct {
		draw int32
		want bool
	}{{0, true}, {3, false}} {
		w.draw.Store(row.draw)
		h := w.i6gArmed(t, i6gRangeDDID, 300, int(row.draw)*100)
		w.onQueue(t, h, func() { script.NPCOf(h).SetTopDesireTarget(script.NPCOf(foe)) })
		w.hit(t, h, 10)
		d, ok := castDesireOf(h, i6gRangeDD)
		if row.want != ok || (ok && (d.Weight != 1000000 || d.FinalTarget.ObjectID() != me)) {
			t.Fatalf("draw %d: range DD desire = %+v (%v), want %v on the character, weight 1000000", row.draw, d, ok, row.want)
		}
	}

	w.draw.Store(0)
	i6bCharacter(t, w).SetVarkaKetraAlliance(1)
	h := w.i6gArmed(t, i6gRangeDDID, -300, 0)
	w.onQueue(t, h, func() { script.NPCOf(h).SetTopDesireTarget(script.NPCOf(foe)) })
	w.hit(t, h, 10)
	i6fWantCasts(t, "after an ally's hit", h, map[modelskill.ID]i6fCast{petrifyID: {me, 2000000, false}})

	mate := w.spawn(t, i6gMateID, -300, 150)
	w.i6eSettled(t, h)
	w.hit(t, mate, 10)
	i6fWantCasts(t, "after an ally's hit on its clan member", h, map[modelskill.ID]i6fCast{petrifyID: {me, 1000000, false}})
}

// TestWizardDDMagic2AggressiveStrikesOnSight runs WizardDDMagic2Aggressive's
// see-creature hook. Settled in its territory with an empty hate list, it
// wants its short range DD magic on the character it sees 100 away,
// weight 1000000, and gives it its default hate list entry, 300; short of
// MP for it, it gives up its spells and wants to attack, weight 1000. A
// fresh one, one with someone on its hate list, and one out of its
// territory want nothing and hate no one.
func TestWizardDDMagic2AggressiveStrikesOnSight(t *testing.T) {
	t.Parallel()
	w := bootIndividual6g(t, raidBoot{}, i6gTemplate(i6gAggressiveID, nil), warriorTemplate(i6gFoeID, nil))
	me := w.player.ObjectID()
	settled := w.i6gArmed(t, i6gAggressiveID, 1500, 0)
	broke := w.i6gArmed(t, i6gAggressiveID, -1500, 0, wShortDDID)
	busy := w.i6gArmed(t, i6gAggressiveID, 0, 1500)
	away := w.i6gArmed(t, i6gAggressiveID, 0, -1500)
	foe := w.spawn(t, i6gFoeID, 0, 2500)
	w.i6eSettled(t, settled, broke, busy, away)
	w.onQueue(t, busy, func() { script.NPCOf(busy).HateList().Add(script.NPCOf(foe), 100) })
	w.teleport(t, away, 3000, 0)
	// One more cycle keeps the next look off the desire queue's decay,
	// which runs every third cycle.
	w.ticks(t, 1)

	w.i6fLookAt(t, settled)
	i6fWantCasts(t, "settled", settled, map[modelskill.ID]i6fCast{wShortDDID: {me, 1000000, false}})
	if got := w.hateListOf(settled); got != 300 {
		t.Fatalf("hate list entry of the character it saw = %v, want 300", got)
	}
	w.i6fLookAt(t, broke)
	w.i6eGaveUp(t, broke, 1000)
	for _, h := range []*npc.Hostile{busy, away} {
		w.i6fLookAt(t, h)
		i6fWantCasts(t, "busy or away", h, nil)
		if got := w.hateListOf(h); got != 0 {
			t.Fatalf("npc %d hate list entry of the character it saw = %v, want none", h.ObjectID(), got)
		}
	}

	fresh := w.i6gArmed(t, i6gAggressiveID, 2500, 0)
	w.i6fLookAt(t, fresh)
	i6fWantCasts(t, "fresh", fresh, nil)
	if got := w.hateListOf(fresh); got != 0 {
		t.Fatalf("fresh hate list entry of the character it saw = %v, want none", got)
	}
}

// TestWizardDDMagic2DispellAggressiveDispellsItsAttacker runs
// WizardDDMagic2DispellAggressive's attacked hook for a hit from 300 away.
// Its parent runs before and after its own part, drawing the long range
// DD magic twice (weight 2000000). Hated, on a draw under 33, the hit
// wants its dispell on the attacker, weight 1000000; on a draw of 33, or
// once it gave up its spells, no dispell. One that cannot pay for the
// dispell gives up its spells and wants to attack, weight 1000, to which
// its parent's second call adds the damage's 125.
func TestWizardDDMagic2DispellAggressiveDispellsItsAttacker(t *testing.T) {
	t.Parallel()
	w := bootIndividual6g(t, raidBoot{}, i6gTemplate(i6gDispellID, nil))
	me := w.player.ObjectID()
	h := w.i6gArmed(t, i6gDispellID, 300, 0)
	w.hit(t, h, 10)
	i6fWantCasts(t, "hit", h, map[modelskill.ID]i6fCast{wLongDDID: {me, 2000000, false}, i6gDispell: {me, 1000000, false}})

	w.draw.Store(33)
	calm := w.i6gArmed(t, i6gDispellID, 0, 300)
	w.hit(t, calm, 10)
	if d, ok := castDesireOf(calm, i6gDispell); ok {
		t.Fatalf("dispell desire on a draw of 33 = %+v, want none", d)
	}

	w.draw.Store(0)
	quit := w.i6gArmed(t, i6gDispellID, 0, -300)
	w.onQueue(t, quit, func() { quit.Scratch().SetInt(npc.IntAI0, 1) })
	w.hit(t, quit, 10)
	i6fWantCasts(t, "given up", quit, nil)

	broke := w.i6gArmed(t, i6gDispellID, -300, 0, i6gDispell)
	w.hit(t, broke, 10)
	w.i6eGaveUp(t, broke, 1125)
}

// i6gSpawnFrames spawns an NPC of id dx, dy from the character, its skills
// i6gSkills, and returns the frames the character was sent as it appeared.
func (w *individual6World) i6gSpawnFrames(t *testing.T, id int32, dx, dy int) (*npc.Hostile, [][]byte) {
	t.Helper()
	n, err := w.srv.NpcSpawns.AddSpawn(id, w.at.X+dx, w.at.Y+dy, w.at.Z, 0, false, 0)
	if err != nil {
		t.Fatalf("spawn %d: %v", id, err)
	}
	w.srv.Settle(t)
	h := n.(*npc.Hostile)
	w.srv.InstallNPCCastSeam(t, h, i6gSkills())
	return h, w.srv.ReadQueued(t, w.srv.Client)
}

// TestWizardHallDDMagic2AggressiveSkipsItsParentsCreatedAndIdle runs
// WizardHallDDMagic2Aggressive's created and no-desire hooks, which do
// nothing: unlike a WizardDDMagic2Aggressive with the same ShoutMsg1, it
// shouts nothing as it spawns, and with nothing to do it does not wander.
func TestWizardHallDDMagic2AggressiveSkipsItsParentsCreatedAndIdle(t *testing.T) {
	t.Parallel()
	shout := npc.AIParams{"ShoutMsg1": "1000290"}
	w := bootIndividual6g(t, raidBoot{}, i6gTemplate(i6gHallID, shout), i6gTemplate(i6gAggressiveID, shout))
	plain, frames := w.i6gSpawnFrames(t, i6gAggressiveID, 300, 0)
	if got := framesOf(frames, serverpackets.OpcodeNpcSay, plain.ObjectID()); len(got) != 1 {
		t.Fatalf("WizardDDMagic2Aggressive shouts as it spawns = %d, want 1", len(got))
	}
	hall, frames := w.i6gSpawnFrames(t, i6gHallID, 0, 300)
	if got := framesOf(frames, serverpackets.OpcodeNpcSay, hall.ObjectID()); len(got) != 0 {
		t.Fatalf("hall wizard shouts as it spawns = %d, want none", len(got))
	}
	w.teleport(t, hall, 3000, 0)
	w.onQueue(t, hall, func() { hall.AI().Desires().Clear() })
	w.ticks(t, 3)
	if d, ok := desire(hall, actorai.IntentionWander); ok {
		t.Fatalf("hall wizard wander desire with nothing to do = %+v, want none", d)
	}
}

// TestWizardHallDDMagic2AggressiveStrikesByDistance runs
// WizardHallDDMagic2Aggressive's attacked, clan-attacked and see-creature
// hooks. A hit from 300 away, fresh, wants its long range DD magic on the
// attacker, weight 1000000 from its attacked hook and as much again from
// its own clan call, and leaves its hate list empty; a hit on its clan
// member wants it once. Seeing
// the character 100 away it drops its walk, wants its short range DD
// magic on it, and hates it 200. Short of MP it gives up its spells and
// wants to attack, weight 1000.
func TestWizardHallDDMagic2AggressiveStrikesByDistance(t *testing.T) {
	t.Parallel()
	w := bootIndividual6g(t, raidBoot{}, inClan(i6gTemplate(i6gHallID, nil)), inClan(warriorTemplate(i6gMateID, nil)))
	me := w.player.ObjectID()
	h := w.i6gArmed(t, i6gHallID, 300, 0)
	w.hit(t, h, 10)
	i6fWantCasts(t, "hit", h, map[modelskill.ID]i6fCast{wLongDDID: {me, 2000000, false}})
	if got := w.hateListOf(h); got != 0 {
		t.Fatalf("hate list entry of its attacker = %v, want none", got)
	}

	called := w.i6gArmed(t, i6gHallID, 0, 300)
	mate := w.spawn(t, i6gMateID, 150, 150)
	w.hit(t, mate, 10)
	i6fWantCasts(t, "hit on its clan member", called, map[modelskill.ID]i6fCast{wLongDDID: {me, 1000000, false}})

	seer := w.i6gArmed(t, i6gHallID, 1500, 0)
	w.onQueue(t, seer, func() {
		seer.AI().Desires().Clear()
		script.NPCOf(seer).AddMoveToDesire(script.Loc{X: w.at.X + 2000, Y: w.at.Y, Z: w.at.Z}, 100)
	})
	w.i6fLookAt(t, seer)
	if d, ok := desire(seer, actorai.IntentionMoveTo); ok {
		t.Fatalf("walk desire after it saw the character = %+v, want none", d)
	}
	i6fWantCasts(t, "seen", seer, map[modelskill.ID]i6fCast{wShortDDID: {me, 1000000, false}})
	if got := w.hateListOf(seer); got != 200 {
		t.Fatalf("hate list entry of the character it saw = %v, want 200", got)
	}

	broke := w.spawn(t, i6gHallID, -300, 0)
	w.srv.InstallNPCCastSeam(t, broke, i6gSkills(wLongDDID))
	w.hit(t, broke, 10)
	w.i6eGaveUp(t, broke, 1000)
}

// TestWizardHallDDMagic2AggressiveAnswersGuardEvents runs
// WizardHallDDMagic2Aggressive's script-event hook. A first argument of
// 10033 naming the character drops every desire and wants the long range
// DD magic on the character 300 away; 10042 wants it to walk to the
// character, weight 10000000. Another first argument, or one naming no
// creature, does nothing.
func TestWizardHallDDMagic2AggressiveAnswersGuardEvents(t *testing.T) {
	t.Parallel()
	w := bootIndividual6g(t, raidBoot{}, i6gTemplate(i6gHallID, nil))
	me := w.player.ObjectID()
	h := w.i6gArmed(t, i6gHallID, 300, 0)
	w.onQueue(t, h, func() {
		h.AI().Desires().Clear()
		script.NPCOf(h).AddWanderDesire(5, 5)
	})
	w.sendEvent6d(t, h, 0, 10033, me)
	i6fWantCasts(t, "event 10033", h, map[modelskill.ID]i6fCast{wLongDDID: {me, 1000000, false}})
	if d, ok := desire(h, actorai.IntentionWander); ok {
		t.Fatalf("wander desire after event 10033 = %+v, want dropped", d)
	}

	w.onQueue(t, h, func() { h.AI().Desires().Clear() })
	w.sendEvent6d(t, h, 0, 10042, me)
	d, ok := desire(h, actorai.IntentionMoveTo)
	if !ok || d.Weight != 10000000 || d.Location.X != w.at.X || d.Location.Y != w.at.Y {
		t.Fatalf("walk desire after event 10042 = %+v (%v), want weight 10000000 to %d,%d", d, ok, w.at.X, w.at.Y)
	}

	w.onQueue(t, h, func() { h.AI().Desires().Clear() })
	w.sendEvent6d(t, h, 10033, me, me)
	w.sendEvent6d(t, h, 0, 10033, 0)
	if got := h.AI().Desires().Snapshot(); len(got) != 0 {
		t.Fatalf("desires after events it ignores = %+v, want none", got)
	}
}

// TestWizardHallDDMagic2AggressiveStrikesClerics runs
// WizardHallDDMagic2Aggressive's see-spell hook for the character's self
// buff seen 300 away. A cleric (15), on a draw under 50, makes it drop
// every desire and want its long range DD magic on the cleric, weight
// 1000000; a warrior (1), or a cleric on a draw of 50, does nothing.
func TestWizardHallDDMagic2AggressiveStrikesClerics(t *testing.T) {
	t.Parallel()
	db := sqltest.SharedDB(t)
	defs := []modelskill.Definition{{
		ID: i6gClericBuff, Level: 1, Activation: modelskill.ActivationActive, Target: modelskill.TargetSelf,
		SkillType: "BUFF", HitTime: 500, StaticHitTime: true, StaticReuse: true,
	}}
	table := modelskill.NewTable(append([]modelskill.Definition{{ID: 248, Level: 3}, {ID: 294, Level: 1}}, defs...))
	w := bootIndividual6g(t, raidBoot{
		opts: []gameservertest.Option{
			gameservertest.WithSkills(skillstate.NewPersistence(gamesql.NewSkillSaveStore(db), table, gamesql.NewCharacterSkillStore(db))),
			gameservertest.WithScriptSkills(defs...),
		},
		ready: func(srv *gameservertest.Server, objID int32) {
			if err := srv.KnownSkills.SetKnownSkill(context.Background(), objID, 0, int(i6gClericBuff), 1); err != nil {
				t.Fatalf("seed skill %d: %v", i6gClericBuff, err)
			}
		},
	}, i6gTemplate(i6gHallID, nil))
	me := w.player.ObjectID()
	h := w.i6gArmed(t, i6gHallID, 300, 0)
	cast := func() {
		w.onQueue(t, h, func() {
			h.AI().Desires().Clear()
			script.NPCOf(h).AddWanderDesire(5, 5)
		})
		castOwnSkill(w.warriorWorld, i6gClericBuff)
		w.srv.Advance(t, 2*time.Second)
		w.srv.ReadQueued(t, w.srv.Client)
	}
	for _, row := range []struct {
		class int
		draw  int32
	}{{1, 0}, {15, 50}} {
		(&ind1World{warriorWorld: w.warriorWorld}).setClass(t, w.player, row.class)
		w.draw.Store(row.draw)
		cast()
		if _, ok := desire(h, actorai.IntentionWander); !ok {
			t.Fatalf("class %d draw %d: wander desire dropped, want kept", row.class, row.draw)
		}
		i6fWantCasts(t, "a buff it ignores", h, nil)
	}
	w.draw.Store(0)
	cast()
	if d, ok := desire(h, actorai.IntentionWander); ok {
		t.Fatalf("wander desire after a cleric's buff = %+v, want dropped", d)
	}
	i6fWantCasts(t, "a cleric's buff", h, map[modelskill.ID]i6fCast{wLongDDID: {me, 1000000, false}})
}
