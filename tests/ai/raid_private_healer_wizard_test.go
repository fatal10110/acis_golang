package ai

import (
	"testing"
	"time"

	actorai "github.com/fatal10110/acis_golang/internal/gameserver/model/actor/ai"
	"github.com/fatal10110/acis_golang/internal/gameserver/model/actor/npc"
	modelskill "github.com/fatal10110/acis_golang/internal/gameserver/model/skill"
	"github.com/fatal10110/acis_golang/internal/gameserver/script"
	"github.com/fatal10110/acis_golang/internal/gameserver/script/ai/defaultnpc"
	"github.com/fatal10110/acis_golang/internal/gameserver/script/ai/monster"
	"github.com/fatal10110/acis_golang/internal/gameserver/script/ai/raid"
	"github.com/fatal10110/acis_golang/internal/gameservertest"
	"github.com/rs/zerolog"
)

// The batch's NPCs and the fixture skills they list by type.
const (
	rhwMasterID   = int32(29603) // the raising NPC, no behavior
	rhwHealerID   = int32(25008) // RaidPrivateHealer
	rhwWizardID   = int32(25002) // RaidPrivateWizard, level 1
	rhwWizard20ID = int32(25033) // RaidPrivateWizard, level 20
	rhwLavaID     = int32(29029) // ValakasPrivateLavasaurus
	rhwHeal       = modelskill.ID(9960)
	rhwRangeBuff  = modelskill.ID(9961)
	rhwDDMagicA   = modelskill.ID(9962)
	rhwDebuffA    = modelskill.ID(9963)
	rhwDDMagic    = modelskill.ID(9964)
	rhwOther      = modelskill.ID(9965)
)

// rhwBatch is the chain of the raid privates and the magic hold monsters
// in scripts.xml order.
var rhwBatch = []struct {
	path string
	ctor func() script.Script
}{
	{"script.ai.individual.DefaultNpc", defaultnpc.New},
	{"script.ai.individual.Monster.MonsterAI", monster.New},
	{"script.ai.individual.Monster.RaidPrivate.RaidPrivateHealer", raid.PrivateHealer},
	{"script.ai.individual.Monster.RaidPrivate.RaidPrivateStandard", raid.PrivateStandard},
	{"script.ai.individual.Monster.RaidPrivate.RaidPrivateWizard", raid.PrivateWizard},
	{"script.ai.individual.Monster.WarriorDDMagicHold.WarriorDDMagicHold", monster.WarriorDDMagicHold},
	{"script.ai.individual.Monster.WarriorDDMagicHold.WarriorDDMagicHoldAggressive.ValakasPrivateLavasaurus", monster.ValakasPrivateLavasaurus},
	{"script.ai.individual.Monster.WarriorDDMagicHold.WarriorDDMagicHoldAggressive.WarriorDDMagicHoldAggressive", monster.WarriorDDMagicHoldAggressive},
}

// rhwSkills are the fixture skills: the heal and the magic strikes and
// debuff reach one target within 600, the buff and the other skill are
// cast on the caster itself.
func rhwSkills() []modelskill.Definition {
	heal := modelskill.Definition{
		ID: rhwHeal, Level: 1, Activation: modelskill.ActivationActive, Target: modelskill.TargetOne,
		SkillType: "HEAL", CastRange: 600, HitTime: 1000, StaticHitTime: true, StaticReuse: true, Power: 1,
	}
	return []modelskill.Definition{
		heal, ind1Skill(rhwRangeBuff, 0, true), ind1Skill(rhwOther, 0, true),
		ind1Skill(rhwDDMagicA, 600, false), ind1Skill(rhwDebuffA, 600, false), ind1Skill(rhwDDMagic, 600, false),
	}
}

// bootRhw boots the batch beside its fixture NPCs, with extra, every NPC
// casting the fixture skills and the raid curse, every draw 0 until a test
// queues others.
func bootRhw(t *testing.T, extra raidBoot) *ind7World {
	t.Helper()
	list := []script.Listing{}
	catalog := script.Catalog{}
	for _, c := range rhwBatch {
		list = append(list, script.Listing{Path: c.path})
		catalog[c.path] = c.ctor
	}
	const director = "test.Ind1Director"
	catalog[director] = func() script.Script { return script.Script{} }
	list = append(list, script.Listing{Path: director})
	wizard20 := wusTemplate(rhwWizard20ID, nil, map[npc.SkillType]modelskill.ID{npc.SkillTypeDDMagicA: rhwDDMagicA, npc.SkillTypeDebuffA: rhwDebuffA})
	wizard20.Level = 20
	lava := wusTemplate(rhwLavaID, nil, map[npc.SkillType]modelskill.ID{npc.SkillTypeDDMagic: rhwDDMagic})
	lava.AggroRange, lava.Clans, lava.ClanRange = 300, []string{"lava"}, 600
	templates := []*npc.Template{
		warriorTemplate(rhwMasterID, nil),
		wusTemplate(rhwHealerID, nil, map[npc.SkillType]modelskill.ID{npc.SkillTypeHealMagicA: rhwHeal, npc.SkillTypeSelfRangeBuffA: rhwRangeBuff}),
		wusTemplate(rhwWizardID, nil, map[npc.SkillType]modelskill.ID{npc.SkillTypeDDMagicA: rhwDDMagicA, npc.SkillTypeDebuffA: rhwDebuffA}),
		wizard20, lava,
	}
	kinds := map[int32]script.NPCKind{}
	for _, tmpl := range templates {
		kinds[int32(tmpl.ID)] = script.KindHostile
	}
	draws := &ind1Draws{}
	skills := rhwSkills()
	opts := append([]gameservertest.Option{
		gameservertest.WithScriptRand(draws.rand),
		gameservertest.WithNPCScripts(kinds, list, catalog),
		gameservertest.WithNpcSpawnSkills(modelskill.NewTable(append(skills, raidCurseDefinition()))),
		gameservertest.WithScriptSkills(skills...),
	}, extra.opts...)
	w := bootRaid(t, raidBoot{opts: opts, ready: extra.ready}, templates...)
	return &ind7World{ind1World: &ind1World{warriorWorld: w, draws: draws, heard: &ind1Heard{}}, spawner: script.NewSpawner(w.srv.NpcSpawns, zerolog.Nop())}
}

// rhwFlee fails unless h wants to run from the character at 100000000
// when want is set, and unless h wants to run from nothing when it is not.
func (w *ind7World) rhwFlee(t *testing.T, when string, h *npc.Hostile, want bool) {
	t.Helper()
	d, ok := desire(h, actorai.IntentionFlee)
	if !want {
		if ok {
			t.Errorf("%s: flee desire %+v, want none", when, d)
		}
		return
	}
	if !ok || d.Weight != 100000000 || d.FinalTarget == nil || d.FinalTarget.ObjectID() != w.player.ObjectID() {
		t.Errorf("%s: flee desire %+v (%v), want from the character at 100000000", when, d, ok)
	}
}

// TestRaidPrivateHealerHealsItsParty ports RaidPrivateHealer's created,
// attacked and party-attacked hooks. Two level 1 healers of one master:
// the character's hit on the first has the second want its heal on the
// first at 100 and the raid curse on the character, 12 levels over it, at
// 1000000; with the first under half its HP, on a draw of 0 the heal
// gains 1000000 more, on a draw of 1 it stays at 100. Seeing the character
// cast, it wants nothing, not even the raid curse its parent would.
func TestRaidPrivateHealerHealsItsParty(t *testing.T) {
	t.Parallel()
	w := bootRhw(t, raidSpellBoot(t))
	w.draws.set(1)
	master := w.spawn(t, rhwMasterID, 400, 0)
	first := w.raise(t, master, rhwHealerID, 100, 0, script.Params{})
	second := w.raise(t, master, rhwHealerID, -100, 0, script.Params{})
	if c0 := script.NPCOf(second).Slot(script.SlotCreature0); !script.Same(c0, script.NPCOf(second)) {
		t.Fatalf("c_ai0 as the healer appeared = %v, want itself", c0)
	}
	me := w.player.ObjectID()
	curse := ind1Cast{me, 1000000}

	w.draws.set(0)
	w.clearDesires(t, second)
	w.hit(t, first, 10)
	w.wantCasts(t, "after a hit on its fellow at full HP", second, map[modelskill.ID]ind1Cast{rhwHeal: {first.ObjectID(), 100}, raidCurseID: curse})
	if c0 := script.NPCOf(first).Slot(script.SlotCreature0); c0 == nil || c0.ObjectID() != me {
		t.Fatalf("c_ai0 of the healer hit = %v, want the character", c0)
	}

	// The hooks see the HP before the hit: a hit leaves the first under half
	// its HP, the next one finds it there.
	w.hit(t, first, first.MaxHP()*6/10)
	w.clearDesires(t, second)
	w.hit(t, first, 10)
	w.wantCasts(t, "after a hit on its fellow under half its HP, draw 0", second, map[modelskill.ID]ind1Cast{rhwHeal: {first.ObjectID(), 1000100}, raidCurseID: curse})

	w.draws.set(1)
	w.clearDesires(t, second)
	w.hit(t, first, 10)
	w.wantCasts(t, "after a hit on its fellow under half its HP, draw 1", second, map[modelskill.ID]ind1Cast{rhwHeal: {first.ObjectID(), 100}, raidCurseID: curse})

	w.srv.Advance(t, 10*time.Second)
	w.clearDesires(t, second)
	w.castSelfBuff()
	w.srv.Advance(t, time.Second)
	w.wantCasts(t, "after a spell it saw", second, nil)
}

// TestRaidPrivateHealerBuffsAndRunsAfterHealing ports RaidPrivateHealer's
// timer and use-skill-finished hooks. Its 2 s check on a draw of 1 wants
// nothing, on a draw of 0 its self range buff at 1000000. Once its heal
// ends it runs from the character, the last playable that hit it, from
// 100 away, at 100000000; it does not after another skill, nor before any
// playable hit it.
func TestRaidPrivateHealerBuffsAndRunsAfterHealing(t *testing.T) {
	t.Parallel()
	w := bootRhw(t, raidBoot{})
	w.draws.set(1)
	master := w.spawn(t, rhwMasterID, 400, 0)
	quiet := w.raise(t, master, rhwHealerID, 3000, 0, script.Params{})
	w.srv.Advance(t, 2*time.Second)
	w.ticks(t, 1)
	if casts := ind1Casts(quiet); len(casts) > 0 || quiet.CastingNow() {
		t.Fatalf("2 s after it appeared, draw 1: casts %+v, casting %v; want none", casts, quiet.CastingNow())
	}

	w.draws.set(0)
	buffer := w.raise(t, master, rhwHealerID, 3000, 100, script.Params{})
	w.srv.AdvanceUntil(t, "the self range buff", func() bool {
		d, ok := ind1Casts(buffer)[rhwRangeBuff]
		return (ok && d.Weight == 1000000 && d.FinalTarget.ObjectID() == buffer.ObjectID()) || buffer.CastingNow()
	})

	w.draws.set(1)
	healer := w.raise(t, master, rhwHealerID, 100, 0, script.Params{})
	w.castOnce(t, healer, rhwHeal)
	w.rhwFlee(t, "after a heal with nobody hitting it", healer, false)
	w.hit(t, healer, 10)
	w.castOnce(t, healer, rhwOther)
	w.rhwFlee(t, "after another skill", healer, false)
	w.castOnce(t, healer, rhwHeal)
	w.rhwFlee(t, "after a heal, the character 100 away", healer, true)
}

// TestRaidPrivateWizardStrikesAndCurses ports RaidPrivateWizard's
// party-attacked, see-spell and attacked hooks, every draw 0 unless named.
// The character, 19 levels over a level 1 wizard, hits its fellow: it
// wants its magic strike at 1000000 and its debuff twice over, 2000000;
// at its own level, 20, the strike and the debuff once. Seeing the
// character cast, the level 20 wizard wants the strike and the debuff at
// 1000000, the level 1 wizard the raid curse beside them. On draws of 1
// none of it. A hit on itself with its master alive wants nothing.
func TestRaidPrivateWizardStrikesAndCurses(t *testing.T) {
	t.Parallel()
	w := bootRhw(t, raidSpellBoot(t))
	w.draws.set(1)
	master := w.spawn(t, rhwMasterID, 400, 0)
	low := []*npc.Hostile{w.raise(t, master, rhwWizardID, 100, 0, script.Params{}), w.raise(t, master, rhwWizardID, -100, 0, script.Params{})}
	high := []*npc.Hostile{w.raise(t, master, rhwWizard20ID, 0, 100, script.Params{}), w.raise(t, master, rhwWizard20ID, 0, -100, script.Params{})}
	me := w.player.ObjectID()
	clear := func() {
		for _, h := range append(append([]*npc.Hostile{}, low...), high...) {
			w.clearDesires(t, h)
		}
	}

	w.draws.set(0)
	clear()
	w.hit(t, low[0], 10)
	w.wantCasts(t, "a level 1 wizard after a hit on its fellow", low[1], map[modelskill.ID]ind1Cast{rhwDDMagicA: {me, 1000000}, rhwDebuffA: {me, 2000000}})
	w.wantCasts(t, "a level 20 wizard after a hit on its fellow", high[1], map[modelskill.ID]ind1Cast{rhwDDMagicA: {me, 1000000}, rhwDebuffA: {me, 1000000}})
	w.draws.set(1)
	clear()
	w.hit(t, low[0], 10)
	w.wantCasts(t, "a level 1 wizard after a hit on its fellow, draw 1", low[1], nil)
	w.wantCasts(t, "a level 20 wizard after a hit on its fellow, draw 1", high[1], nil)
	if c0 := script.NPCOf(low[0]).Slot(script.SlotCreature0); c0 == nil || c0.ObjectID() != me {
		t.Fatalf("c_ai0 of the wizard hit = %v, want the character", c0)
	}

	spell := func(when string, draw int, wantLow, wantHigh map[modelskill.ID]ind1Cast) {
		t.Helper()
		w.srv.Advance(t, 10*time.Second)
		clear()
		w.draws.set(draw)
		w.castSelfBuff()
		w.srv.Advance(t, time.Second)
		w.wantCasts(t, when+", level 1", low[1], wantLow)
		w.wantCasts(t, when+", level 20", high[1], wantHigh)
	}
	strike := map[modelskill.ID]ind1Cast{rhwDDMagicA: {me, 1000000}, rhwDebuffA: {me, 1000000}}
	spell("after a spell it saw, draw 0", 0, map[modelskill.ID]ind1Cast{rhwDDMagicA: {me, 1000000}, rhwDebuffA: {me, 1000000}, raidCurseID: {me, 1000000}}, strike)
	spell("after a spell it saw, draw 1", 1, map[modelskill.ID]ind1Cast{raidCurseID: {me, 1000000}}, nil)
}

// TestRaidPrivateWizardRunsFromItsAttacker ports RaidPrivateWizard's
// created and timer hooks: 2 s after it appeared, a wizard the character
// hit from 100 away runs from it at 100000000; its fellow, which nobody
// hit, does not.
func TestRaidPrivateWizardRunsFromItsAttacker(t *testing.T) {
	t.Parallel()
	w := bootRhw(t, raidBoot{})
	w.draws.set(1)
	master := w.spawn(t, rhwMasterID, 400, 0)
	hit := w.raise(t, master, rhwWizard20ID, 100, 0, script.Params{})
	untouched := w.raise(t, master, rhwWizard20ID, -100, 0, script.Params{})
	if c0 := script.NPCOf(untouched).Slot(script.SlotCreature0); !script.Same(c0, script.NPCOf(untouched)) {
		t.Fatalf("c_ai0 as the wizard appeared = %v, want itself", c0)
	}
	w.hit(t, hit, 10)
	w.rhwFlee(t, "right after the hit", hit, false)
	w.srv.Advance(t, 2*time.Second)
	w.rhwFlee(t, "2 s after it appeared, hit", hit, true)
	w.rhwFlee(t, "2 s after it appeared, not hit", untouched, false)
}
