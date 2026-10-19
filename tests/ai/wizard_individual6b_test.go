package ai

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"slices"
	"sync/atomic"
	"testing"
	"time"

	gamesql "github.com/fatal10110/acis_golang/internal/gameserver/data/sql"
	"github.com/fatal10110/acis_golang/internal/gameserver/data/sql/sqltest"
	gamexml "github.com/fatal10110/acis_golang/internal/gameserver/data/xml"
	actorai "github.com/fatal10110/acis_golang/internal/gameserver/model/actor/ai"
	"github.com/fatal10110/acis_golang/internal/gameserver/model/actor/move"
	"github.com/fatal10110/acis_golang/internal/gameserver/model/actor/npc"
	"github.com/fatal10110/acis_golang/internal/gameserver/model/actor/player"
	"github.com/fatal10110/acis_golang/internal/gameserver/model/location"
	modelskill "github.com/fatal10110/acis_golang/internal/gameserver/model/skill"
	"github.com/fatal10110/acis_golang/internal/gameserver/network"
	"github.com/fatal10110/acis_golang/internal/gameserver/script"
	"github.com/fatal10110/acis_golang/internal/gameserver/script/ai/defaultnpc"
	"github.com/fatal10110/acis_golang/internal/gameserver/script/ai/monster"
	"github.com/fatal10110/acis_golang/internal/gameserver/script/ai/wizard"
	skillstate "github.com/fatal10110/acis_golang/internal/gameserver/skill"
	"github.com/fatal10110/acis_golang/internal/gameservertest"
	"github.com/rs/zerolog"
)

// The NPC ids of the PartyLeaderWizardDD2 children: the datapack ids each
// class binds, beside the creatures they call or fight.
const (
	i6bKetraStoneID = int32(21347) // PartyLeaderWizardCastRangeCurseStoneAggressive
	i6bVarkaStoneID = int32(21373) // PartyLeaderWizardCastRangeCurseStoneAggressive
	i6bNecroID      = int32(21596) // PartyLeaderWizardCorpseNecroAggressive
	i6bNecroCallID  = int32(21561) // its SummonPrivate, no behavior here
	i6bCurseID      = int32(21090) // PartyLeaderWizardDD2CurseAggressive
	i6bHereticID    = int32(22163) // PartyLeaderWizardHereticAggressive
	i6bSummonerID   = int32(22033) // PartyLeaderWizardDD2Summon
	i6bSilhouette1  = int32(22045) // no behavior here
	i6bSilhouette2  = int32(22047) // no behavior here
	i6bSilhouette3  = int32(22048) // no behavior here
	i6bEyeID        = int32(21312) // PartyLeaderWizardRangeCurseAggressive
	i6bMateID       = int32(29520) // no behavior
	i6bFoeID        = int32(29521) // no behavior
)

// The skills the children list by type.
const (
	i6bBuff1ID        = modelskill.ID(9360)
	i6bBuff2ID        = modelskill.ID(9361)
	i6bRangeDebuffID  = modelskill.ID(9362)
	i6bDebuffID       = modelskill.ID(9363)
	i6bDDMagic1ID     = modelskill.ID(9364)
	i6bClearCorpseID  = modelskill.ID(9365)
	i6bSpecialID      = modelskill.ID(9366)
	i6bSelfHealID     = modelskill.ID(9367)
	i6bRangeBuffID    = modelskill.ID(9368)
	i6bRangeDDID      = modelskill.ID(9369)
	i6bDebuffCancelID = modelskill.ID(9370)
)

// individual6bScripts are the behavior chain down to WizardBase, the
// PartyLeaderWizardDD2 children in scripts.xml order, and the event
// listener.
func individual6bScripts() ([]script.Listing, script.Catalog) {
	entries := []struct {
		path string
		ctor func() script.Script
	}{
		{"script.ai.individual.DefaultNpc", defaultnpc.New},
		{"script.ai.individual.Monster.MonsterAI", monster.New},
		{"script.ai.individual.Monster.WizardBase.WizardBase", wizard.Base},
		{"script.ai.individual.Monster.WizardBase.PartyLeaderWizard.PartyLeaderWizardCastRangeCurseStoneAggressive", wizard.PartyLeaderCastRangeCurseStoneAggressive},
		{"script.ai.individual.Monster.WizardBase.PartyLeaderWizard.PartyLeaderWizardCorpseNecroAggressive", wizard.PartyLeaderCorpseNecroAggressive},
		{"script.ai.individual.Monster.WizardBase.PartyLeaderWizard.PartyLeaderWizardDD2CurseAggressive", wizard.PartyLeaderDD2CurseAggressive},
		{"script.ai.individual.Monster.WizardBase.PartyLeaderWizard.PartyLeaderWizardDD2Summon", wizard.PartyLeaderDD2Summon},
		{"script.ai.individual.Monster.WizardBase.PartyLeaderWizard.PartyLeaderWizardHereticAggressive", wizard.PartyLeaderHereticAggressive},
		{"script.ai.individual.Monster.WizardBase.PartyLeaderWizard.PartyLeaderWizardRangeCurseAggressive", wizard.PartyLeaderRangeCurseAggressive},
		{"ai.EventListener", eventListener},
		{"test.I6bDirector", func() script.Script { return script.Script{} }},
	}
	var list []script.Listing
	catalog := script.Catalog{}
	for _, e := range entries {
		list = append(list, script.Listing{Path: e.path})
		catalog[e.path] = e.ctor
	}
	return list, catalog
}

func bootIndividual6b(t *testing.T, extra raidBoot, templates ...*npc.Template) *individual6World {
	t.Helper()
	list, catalog := individual6bScripts()
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

// i6bTemplate is a level 1 fixture of id with ai as its AI parameters,
// listing skills by type.
func i6bTemplate(id int32, ai npc.AIParams, skills map[npc.SkillType]modelskill.ID) *npc.Template {
	tmpl := warriorTemplate(id, ai)
	tmpl.SkillsByType = map[npc.SkillType]modelskill.Ref{}
	for t, skill := range skills {
		tmpl.SkillsByType[t] = modelskill.Ref{ID: skill, Level: 1}
	}
	return tmpl
}

// i6bCharacter returns the scenario's online character.
func i6bCharacter(t *testing.T, w *individual6World) *player.Character {
	t.Helper()
	obj, ok := w.srv.State.Player(w.player.ObjectID())
	if !ok {
		t.Fatal("the character is not in the world")
	}
	ch, ok := network.OnlineCharacter(obj)
	if !ok {
		t.Fatalf("the character is %T, not an online character", obj)
	}
	return ch
}

// i6bCastWeighs reports whether h wants to cast skill id at target with
// weight.
func i6bCastWeighs(h *npc.Hostile, id modelskill.ID, target int32, weight float64) bool {
	d, ok := castDesireOf(h, id)
	return ok && d.Weight == weight && d.FinalTarget != nil && d.FinalTarget.ObjectID() == target
}

// moveNextTo puts the character dx, dy from h.
func (w *individual6World) moveNextTo(t *testing.T, h *npc.Hostile, dx, dy int) {
	t.Helper()
	obj, ok := w.srv.State.Player(w.player.ObjectID())
	if !ok {
		t.Fatal("the character is not in the world")
	}
	x, y, z := h.Position()
	if err := w.srv.State.Move(obj, x+dx, y+dy, z); err != nil {
		t.Fatalf("move the character: %v", err)
	}
	w.srv.Settle(t)
}

// i6bStoneTemplate is a fixture PartyLeaderWizardCastRangeCurseStoneAggressive
// of id in the faction clan, listing its two buffs and range debuff.
func i6bStoneTemplate(id int32, faction string) *npc.Template {
	tmpl := i6bTemplate(id, nil, map[npc.SkillType]modelskill.ID{
		npc.SkillTypeBuff1: i6bBuff1ID, npc.SkillTypeBuff2: i6bBuff2ID, npc.SkillTypeRangeDebuff: i6bRangeDebuffID,
	})
	tmpl.Clans, tmpl.ClanRange = []string{faction}, 500
	return tmpl
}

// i6bStoneSkills are a fixture stone's skills, its range debuff costing mp
// MP.
func i6bStoneSkills(mp int) *modelskill.Table {
	rangeDebuff := npcSkill(i6bRangeDebuffID, 1, false)
	rangeDebuff.MPConsume = mp
	return modelskill.NewTable([]modelskill.Definition{
		npcSkill(petrifyID, 1, true), npcSkill(i6bBuff1ID, 1, false), npcSkill(i6bBuff2ID, 1, false), rangeDebuff,
	})
}

// TestPartyLeaderWizardStonePetrifiesAllies runs
// PartyLeaderWizardCastRangeCurseStoneAggressive's see-creature, attacked
// and clan-attacked hooks for a Ketra and a Varka prophet. A character
// allied with the prophet's faction is not attacked on sight; its hit makes
// the prophet want to petrify it (4515, weight 1000000 from its attacked
// hook and as much again from its own clan call), stop hating it and cast
// no buff. A character with no alliance is attacked on sight with weight
// 200.
func TestPartyLeaderWizardStonePetrifiesAllies(t *testing.T) {
	t.Parallel()
	for _, row := range []struct {
		name     string
		id       int32
		faction  string
		standing int
	}{{"ketra", i6bKetraStoneID, "ketra_orc_clan", 1}, {"varka", i6bVarkaStoneID, "varka_silenos_clan", -1}} {
		t.Run(row.name, func(t *testing.T) {
			t.Parallel()
			w := bootIndividual6b(t, raidBoot{}, i6bStoneTemplate(row.id, row.faction))
			ch := i6bCharacter(t, w)
			ch.SetVarkaKetraAlliance(row.standing)

			allied := w.spawn(t, row.id, 150, 0)
			w.srv.InstallNPCCastSeam(t, allied, i6bStoneSkills(0))
			w.ticks(t, 2)
			if got := allied.AggroHate(w.player); got != 0 {
				t.Fatalf("hate of an ally it sees = %v, want none", got)
			}
			w.hit(t, allied, 10)
			if !i6bCastWeighs(allied, petrifyID, w.player.ObjectID(), 2000000) {
				d, ok := castDesireOf(allied, petrifyID)
				t.Fatalf("petrify desire = %+v (%v), want on the character, weight 2000000", d, ok)
			}
			for _, id := range []modelskill.ID{i6bBuff1ID, i6bBuff2ID} {
				if d, ok := castDesireOf(allied, id); ok {
					t.Fatalf("buff %d desire after an ally's hit = %+v, want none", id, d)
				}
			}
			if got := allied.AggroHate(w.player); got != 0 {
				t.Fatalf("hate of the ally that hit it = %v, want none", got)
			}

			ch.SetVarkaKetraAlliance(0)
			open := w.spawn(t, row.id, 0, 150)
			w.ticks(t, 2)
			if got := open.AggroHate(w.player); got != 200 {
				t.Fatalf("hate of a character with no alliance it sees = %v, want 200", got)
			}
		})
	}
}

// TestPartyLeaderWizardStoneBuffsAndCursesAround runs the stone's attacked
// hook for a character with no alliance, every draw 0.
// The first hit, above half of its HP, wants both buffs on itself, weight
// 9999999, and sets int slot 3; a later hit wants none. With another top
// desire target, a hit wants its range debuff on itself, weight 9999999;
// a stone that cannot pay for it sets int slot 0 and wants the character
// attacked instead.
func TestPartyLeaderWizardStoneBuffsAndCursesAround(t *testing.T) {
	t.Parallel()
	w := bootIndividual6b(t, raidBoot{}, i6bStoneTemplate(i6bKetraStoneID, "ketra_orc_clan"), warriorTemplate(i6bFoeID, nil))
	me := w.player.ObjectID()
	h := w.spawn(t, i6bKetraStoneID, 150, 0)
	foe := w.spawn(t, i6bFoeID, 200, 0)
	w.srv.InstallNPCCastSeam(t, h, i6bStoneSkills(0))

	w.hit(t, h, 10)
	for _, id := range []modelskill.ID{i6bBuff1ID, i6bBuff2ID} {
		if !i6bCastWeighs(h, id, h.ObjectID(), 9999999) {
			t.Fatalf("stone wants no buff %d on itself after its first hit, weight 9999999", id)
		}
	}
	if got := h.Scratch().Int(npc.IntAI3); got != 1 {
		t.Fatalf("int slot 3 after the buffs = %d, want 1", got)
	}
	if d, ok := castDesireOf(h, i6bRangeDebuffID); ok {
		t.Fatalf("range debuff desire with the character its top desire target = %+v, want none", d)
	}

	w.onQueue(t, h, func() {
		h.AI().Desires().Clear()
		script.NPCOf(h).SetTopDesireTarget(script.NPCOf(foe))
	})
	w.hit(t, h, 10)
	if !i6bCastWeighs(h, i6bRangeDebuffID, h.ObjectID(), 9999999) {
		t.Fatal("stone wants no range debuff around itself on a hit with another top desire target")
	}
	for _, id := range []modelskill.ID{i6bBuff1ID, i6bBuff2ID} {
		if d, ok := castDesireOf(h, id); ok {
			t.Fatalf("buff %d desire on a later hit = %+v, want none", id, d)
		}
	}

	broke := w.spawn(t, i6bKetraStoneID, 0, 150)
	w.srv.InstallNPCCastSeam(t, broke, i6bStoneSkills(100000))
	w.onQueue(t, broke, func() { script.NPCOf(broke).SetTopDesireTarget(script.NPCOf(foe)) })
	w.hit(t, broke, 10)
	if d, ok := desire(broke, actorai.IntentionAttack); broke.Scratch().Int(npc.IntAI0) != 1 || !ok || d.FinalTarget == nil || d.FinalTarget.ObjectID() != me {
		t.Fatalf("stone short of MP: int slot 0 %d, attack desire %+v (%v); want 1 and the character attacked", broke.Scratch().Int(npc.IntAI0), d, ok)
	}
	if d, ok := castDesireOf(broke, i6bRangeDebuffID); ok {
		t.Fatalf("stone short of MP wants its range debuff: %+v", d)
	}
}

// TestPartyLeaderWizardStoneAnswersItsClan runs the stone's clan-attacked
// hook, every draw 0: young, a clan member's hit wants no buff; settled
// (life time past 7), it wants both buffs and its range debuff on itself,
// weight 9999999, whatever its top desire target.
func TestPartyLeaderWizardStoneAnswersItsClan(t *testing.T) {
	t.Parallel()
	mate := warriorTemplate(i6bMateID, nil)
	mate.Clans, mate.ClanRange = []string{"ketra_orc_clan"}, 500
	w := bootIndividual6b(t, raidBoot{}, i6bStoneTemplate(i6bKetraStoneID, "ketra_orc_clan"), mate)
	h := w.spawn(t, i6bKetraStoneID, 1500, 0)
	m := w.spawn(t, i6bMateID, 1500, 100)
	w.srv.InstallNPCCastSeam(t, h, i6bStoneSkills(0))

	w.hit(t, m, 10)
	if d, ok := castDesireOf(h, i6bBuff1ID); ok {
		t.Fatalf("young stone wants its buff on a clan hit: %+v", d)
	}
	w.ticks(t, 9)
	if h.LifeTime() <= 7 {
		t.Fatalf("setup: life time %d, want past 7", h.LifeTime())
	}
	w.hit(t, m, 10)
	for _, id := range []modelskill.ID{i6bBuff1ID, i6bBuff2ID, i6bRangeDebuffID} {
		if !i6bCastWeighs(h, id, h.ObjectID(), 9999999) {
			d, ok := castDesireOf(h, id)
			t.Fatalf("settled stone skill %d desire on a clan hit = %+v (%v), want on itself, weight 9999999", id, d, ok)
		}
	}
}

// TestPartyLeaderWizardStonePetrifiesAlliedCaster runs the stone's
// see-spell hook for a Ketra prophet that sees the character cast a self
// buff. A buff with aggro points cast by a character with no alliance, or
// one without aggro points cast by a Ketra ally, wants no petrify; a buff
// with aggro points cast by a Ketra ally wants the caster petrified (4515,
// weight 1000000).
func TestPartyLeaderWizardStonePetrifiesAlliedCaster(t *testing.T) {
	t.Parallel()
	db := sqltest.SharedDB(t)
	buff := func(id modelskill.ID, aggro int) modelskill.Definition {
		return modelskill.Definition{
			ID: id, Level: 1, Activation: modelskill.ActivationActive, Target: modelskill.TargetSelf,
			SkillType: "BUFF", HitTime: 500, StaticHitTime: true, StaticReuse: true, AggroPoints: aggro,
		}
	}
	defs := []modelskill.Definition{buff(plwAggroBuff, 100), buff(plwPlainBuff, 0)}
	table := modelskill.NewTable(append([]modelskill.Definition{{ID: 248, Level: 3}, {ID: 294, Level: 1}}, defs...))
	w := bootIndividual6b(t, raidBoot{
		opts: []gameservertest.Option{
			gameservertest.WithSkills(skillstate.NewPersistence(gamesql.NewSkillSaveStore(db), table, gamesql.NewCharacterSkillStore(db))),
			gameservertest.WithScriptSkills(defs...),
		},
		ready: func(srv *gameservertest.Server, objID int32) {
			for _, d := range defs {
				if err := srv.KnownSkills.SetKnownSkill(context.Background(), objID, 0, int(d.ID), 1); err != nil {
					t.Fatalf("seed skill %d: %v", d.ID, err)
				}
			}
		},
	}, i6bStoneTemplate(i6bKetraStoneID, "ketra_orc_clan"))
	ch := i6bCharacter(t, w)
	h := w.spawn(t, i6bKetraStoneID, 150, 0)
	w.srv.InstallNPCCastSeam(t, h, i6bStoneSkills(0))

	for _, row := range []struct {
		standing int
		skill    modelskill.ID
	}{{0, plwAggroBuff}, {1, plwPlainBuff}} {
		ch.SetVarkaKetraAlliance(row.standing)
		castOwnSkill(w.warriorWorld, row.skill)
		w.srv.Advance(t, 2*time.Second)
		w.srv.ReadQueued(t, w.srv.Client)
		if d, ok := castDesireOf(h, petrifyID); ok {
			t.Fatalf("standing %d, buff %d: the stone wants %+v, want no petrify", row.standing, row.skill, d)
		}
	}
	castOwnSkill(w.warriorWorld, plwAggroBuff)
	w.srv.AdvanceUntil(t, "the petrify desire", func() bool {
		_, ok := castDesireOf(h, petrifyID)
		return ok
	})
	if !i6bCastWeighs(h, petrifyID, w.player.ObjectID(), 1000000) {
		d, _ := castDesireOf(h, petrifyID)
		t.Fatalf("petrify desire = %+v, want on the caster, weight 1000000", d)
	}
}

// i6bNecroTemplate is a fixture PartyLeaderWizardCorpseNecroAggressive,
// calling i6bNecroCallID, listing its DD magic and corpse clearing.
func i6bNecroTemplate() *npc.Template {
	return i6bTemplate(i6bNecroID, npc.AIParams{"SummonPrivate": fmt.Sprint(i6bNecroCallID)}, map[npc.SkillType]modelskill.ID{
		npc.SkillTypeDDMagic1: i6bDDMagic1ID, npc.SkillTypeClearCorpse: i6bClearCorpseID,
		npc.SkillTypeWLongRangeDDMagic: wLongDDID, npc.SkillTypeWShortRangeDDMagic: wShortDDID,
	})
}

// i6bNecroSkills are a fixture necromancer's skills.
func i6bNecroSkills() *modelskill.Table {
	return modelskill.NewTable([]modelskill.Definition{harmlessSkill(i6bDDMagic1ID), npcSkill(i6bClearCorpseID, 1, false), harmlessSkill(wLongDDID), harmlessSkill(wShortDDID)})
}

// TestPartyLeaderWizardCorpseNecroLooksAround runs
// PartyLeaderWizardCorpseNecroAggressive's created and timer hooks: the
// 5 s timer "3456" looks around with no AI tick, so the character beside
// it is attacked on sight, weight 200.
func TestPartyLeaderWizardCorpseNecroLooksAround(t *testing.T) {
	t.Parallel()
	w := bootIndividual6b(t, raidBoot{}, i6bNecroTemplate())
	h := w.spawn(t, i6bNecroID, 150, 0)
	if got := h.AggroHate(w.player); got != 0 {
		t.Fatalf("setup: hate before the first look = %v, want none", got)
	}
	w.srv.Advance(t, 5*time.Second)
	if got := h.AggroHate(w.player); got != 200 {
		t.Fatalf("hate after the 3456 look = %v, want 200", got)
	}
}

// TestPartyLeaderWizardCorpseNecroRaisesTheDead runs the necromancer's
// see-creature hook, every draw 0: attacking another creature, the AI
// tick's look sees the dead character within 100. It places
// its SummonPrivate where the corpse lies, told (1000, the creature it
// hates most), and wants the corpse cleared, weight 1000000 (its corpse
// clearing, like the datapack's, targets the caster).
func TestPartyLeaderWizardCorpseNecroRaisesTheDead(t *testing.T) {
	t.Parallel()
	w := bootIndividual6b(t, raidBoot{}, i6bNecroTemplate(), warriorTemplate(i6bNecroCallID, nil), warriorTemplate(i6bFoeID, nil))
	ch := i6bCharacter(t, w)
	done := make(chan struct{})
	if !w.srv.PlayerQueue(t, w.player.ObjectID()).Post(func() {
		defer close(done)
		ch.Die(nil)
	}) {
		t.Fatal("the character's queue is closed")
	}
	<-done
	w.srv.Settle(t)
	if !ch.Dead() {
		t.Fatal("setup: the character is alive")
	}

	h := w.spawn(t, i6bNecroID, 50, 0)
	foe := w.spawn(t, i6bFoeID, 50, 50)
	w.srv.InstallNPCCastSeam(t, h, i6bNecroSkills())
	w.onQueue(t, h, func() { script.NPCOf(h).AddAttackDesire(script.NPCOf(foe), 1000000) })
	w.ticks(t, 1)
	if got := h.AI().CurrentIntention(); got != actorai.IntentionAttack {
		t.Fatalf("setup: intention = %v, want attack", got)
	}
	called := privatesOf(t, h, 1)[0]
	if got := int32(called.Instance.Template.ID); got != i6bNecroCallID {
		t.Fatalf("raised private = %d, want %d", got, i6bNecroCallID)
	}
	if cx, cy, _ := called.Position(); cx != w.at.X || cy != w.at.Y {
		t.Fatalf("raised private at %d,%d, want on the corpse at %d,%d", cx, cy, w.at.X, w.at.Y)
	}
	if got := [2]int32{called.Scratch().Int(npc.IntParam1), called.Scratch().Int(npc.IntParam2)}; got != [2]int32{1000, foe.ObjectID()} {
		t.Fatalf("raised private spawn parameters = %v, want 1000 and the foe %d", got, foe.ObjectID())
	}
	if !i6bCastWeighs(h, i6bClearCorpseID, h.ObjectID(), 1000000) {
		d, ok := castDesireOf(h, i6bClearCorpseID)
		t.Fatalf("clear corpse desire = %+v (%v), want weight 1000000, the self skill aimed at the necromancer", d, ok)
	}
}

// TestPartyLeaderWizardCorpseNecroStrikesBelow40 runs the necromancer's
// attacked hook, every draw 0: with the character its top desire target
// and in its hate list, a hit at full HP wants no DD magic; under 40% of
// its HP a hit wants its DD magic on the character, weight 1000000.
func TestPartyLeaderWizardCorpseNecroStrikesBelow40(t *testing.T) {
	t.Parallel()
	w := bootIndividual6b(t, raidBoot{}, i6bNecroTemplate())
	h := w.spawn(t, i6bNecroID, 1500, 0)
	w.srv.InstallNPCCastSeam(t, h, i6bNecroSkills())
	w.onQueue(t, h, func() { script.NPCOf(h).SetTopDesireTarget(script.PlayerOf(w.player)) })

	w.hit(t, h, 10)
	if d, ok := castDesireOf(h, i6bDDMagic1ID); ok {
		t.Fatalf("DD magic desire at full HP = %+v, want none", d)
	}
	w.onQueue(t, h, func() {
		h.SetHP(h.MaxHPValue() * 0.3)
		script.NPCOf(h).SetTopDesireTarget(script.PlayerOf(w.player))
	})
	w.hit(t, h, 10)
	if !i6bCastWeighs(h, i6bDDMagic1ID, w.player.ObjectID(), 1000000) {
		d, ok := castDesireOf(h, i6bDDMagic1ID)
		t.Fatalf("DD magic desire under 40%% = %+v (%v), want on the character, weight 1000000", d, ok)
	}
}

// i6bCurseTemplate is a fixture DD2 curse leader of id listing its debuff
// and its two DD magics.
func i6bCurseTemplate(id int32) *npc.Template {
	return i6bTemplate(id, nil, map[npc.SkillType]modelskill.ID{
		npc.SkillTypeDebuff: i6bDebuffID, npc.SkillTypeWLongRangeDDMagic: wLongDDID, npc.SkillTypeWShortRangeDDMagic: wShortDDID,
		npc.SkillTypeSpecialAttack: i6bSpecialID, npc.SkillTypeSelfMagicHeal: i6bSelfHealID,
	})
}

// i6bCurseSkills are a fixture curse leader's skills.
func i6bCurseSkills() *modelskill.Table {
	return modelskill.NewTable([]modelskill.Definition{
		harmlessSkill(i6bDebuffID), harmlessSkill(wLongDDID), harmlessSkill(wShortDDID), harmlessSkill(i6bSpecialID), npcSkill(i6bSelfHealID, 1, false),
	})
}

// TestPartyLeaderWizardDD2CurseSeesAndCurses runs
// PartyLeaderWizardDD2CurseAggressive's see-creature hook, every draw 0.
// Young, it does nothing on sight, not even its parents' hooks. Settled in
// its territory with an empty hate list, the character arriving 300 away
// is wanted cursed (weight 1000000) and struck with its long range DD
// magic, and enters its hate list; arriving 50 away it is struck with the
// short range one.
func TestPartyLeaderWizardDD2CurseSeesAndCurses(t *testing.T) {
	t.Parallel()
	for _, row := range []struct {
		name  string
		dist  int
		young bool
		dd    modelskill.ID
	}{{"far", 300, false, wLongDDID}, {"near", 50, false, wShortDDID}, {"young", 300, true, 0}} {
		t.Run(row.name, func(t *testing.T) {
			t.Parallel()
			w := bootIndividual6b(t, raidBoot{}, i6bCurseTemplate(i6bCurseID))
			h := w.spawn(t, i6bCurseID, 1500, 0)
			w.srv.InstallNPCCastSeam(t, h, i6bCurseSkills())
			if !row.young {
				w.ticks(t, 9)
				if h.LifeTime() <= 7 {
					t.Fatalf("setup: life time %d, want past 7", h.LifeTime())
				}
			}
			w.moveNextTo(t, h, row.dist, 0)
			w.ticks(t, 1)
			me := w.player.ObjectID()
			if row.young {
				if d, ok := castDesireOf(h, i6bDebuffID); ok {
					t.Fatalf("young curse leader debuff desire = %+v, want none", d)
				}
				if got := h.AggroHate(w.player); got != 0 {
					t.Fatalf("young curse leader hate = %v, want none", got)
				}
				return
			}
			if !i6bCastWeighs(h, i6bDebuffID, me, 1000000) {
				t.Fatal("settled curse leader wants no debuff on the character it sees")
			}
			if !i6bCastWeighs(h, row.dd, me, 1000000) {
				t.Fatalf("settled curse leader wants no DD magic %d on the character %d away", row.dd, row.dist)
			}
			if w.hateListOf(h) == 0 {
				t.Fatal("settled curse leader did not enter the character in its hate list")
			}
		})
	}
}

// TestPartyLeaderWizardDD2CurseCursesAttackers runs the curse leader's
// attacked and clan-attacked hooks, every draw 0: a hit, once the parents
// have entered the character in its hate list, wants it cursed, weight
// 1000000; a settled leader whose clan member is hit, with an empty hate
// list, wants the attacker cursed too.
func TestPartyLeaderWizardDD2CurseCursesAttackers(t *testing.T) {
	t.Parallel()
	w := bootIndividual6b(t, raidBoot{}, inClan(i6bCurseTemplate(i6bCurseID)), inClan(warriorTemplate(i6bMateID, nil)))
	me := w.player.ObjectID()
	h := w.spawn(t, i6bCurseID, 1500, 0)
	other := w.spawn(t, i6bCurseID, 1500, 200)
	mate := w.spawn(t, i6bMateID, 1500, 100)
	w.srv.InstallNPCCastSeam(t, h, i6bCurseSkills())
	w.srv.InstallNPCCastSeam(t, other, i6bCurseSkills())

	w.hit(t, h, 10)
	if !i6bCastWeighs(h, i6bDebuffID, me, 1000000) {
		t.Fatal("curse leader wants no debuff on its attacker")
	}

	w.ticks(t, 9)
	w.onQueue(t, other, func() { other.AI().Desires().Clear(); other.AI().Hates().Clear() })
	w.hit(t, mate, 10)
	if !i6bCastWeighs(other, i6bDebuffID, me, 1000000) {
		t.Fatal("settled curse leader wants no debuff on its clan member's attacker")
	}
}

// TestPartyLeaderWizardHereticKeepsHomeAndCallsForHelp runs
// PartyLeaderWizardHereticAggressive's created, attacked, no-desire and
// skill-finished hooks, every draw 0. As it spawns it wants to walk to its
// spawn point, weight 30. A hit while it neither attacks nor casts
// broadcasts event 10033 with the attacker's id. Its self heal landing sets
// int slot 3.
func TestPartyLeaderWizardHereticKeepsHomeAndCallsForHelp(t *testing.T) {
	t.Parallel()
	w := bootIndividual6b(t, raidBoot{}, i6bCurseTemplate(i6bHereticID), warriorTemplate(plwListenerID, nil))
	h := w.spawn(t, i6bHereticID, 1500, 0)
	listener := w.spawn(t, plwListenerID, 1500, 100)
	w.srv.InstallNPCCastSeam(t, h, i6bCurseSkills())
	home := script.NPCOf(h).SpawnLocation()
	if d, ok := desire(h, actorai.IntentionMoveTo); !ok || d.Weight != 30 || d.Location.X != home.X || d.Location.Y != home.Y {
		t.Fatalf("heretic move desire after spawning = %+v (%v), want weight 30 to %d,%d", d, ok, home.X, home.Y)
	}

	w.hit(t, h, 10)
	if got := says(w.srv.ReadQueued(t, w.srv.Client), listener.ObjectID()); !slices.Contains(got, fmt.Sprintf("event 10033 %d", w.player.ObjectID())) {
		t.Fatalf("events heard after an idle hit = %q, want event 10033 with the character", got)
	}

	w.onQueue(t, h, func() {
		script.NPCOf(h).AddCastDesire(script.NPCOf(h), modelskill.Ref{ID: i6bSelfHealID, Level: 1}, 1e9)
	})
	for i := 0; h.Scratch().Int(npc.IntAI3) != 1; i++ {
		if i == 20 {
			t.Fatal("the heretic's self heal never landed")
		}
		w.ticks(t, 1)
		w.srv.Advance(t, time.Second)
	}
}

// TestPartyLeaderWizardHereticLooksAroundForItsParty runs the heretic's
// party-attacked and see-creature hooks, every draw 0. Settled and idle
// under 40% of its HP, a hit on its private makes it look around: it sees
// the character in its territory with an empty hate list and broadcasts
// event 10033 with the character's id. With int slot 3 set (its self heal
// landed) it does not look.
func TestPartyLeaderWizardHereticLooksAroundForItsParty(t *testing.T) {
	t.Parallel()
	for _, row := range []struct {
		name   string
		healed bool
	}{{"hurt", false}, {"healed", true}} {
		t.Run(row.name, func(t *testing.T) {
			t.Parallel()
			leader := i6bCurseTemplate(i6bHereticID)
			leader.Privates = []npc.PrivateEntry{{NpcID: int(plwPrivateID), Weight: 1, RespawnDelay: 240 * time.Second}}
			w := bootIndividual6b(t, raidBoot{}, leader, warriorTemplate(plwPrivateID, nil), warriorTemplate(plwListenerID, nil))
			h := w.spawn(t, i6bHereticID, 1500, 0)
			private := privatesOf(t, h, 1)[0]
			listener := w.spawn(t, plwListenerID, 1500, 100)
			w.srv.InstallNPCCastSeam(t, h, i6bCurseSkills())
			w.ticks(t, 9)
			w.onQueue(t, h, func() {
				h.SetHP(h.MaxHPValue() * 0.3)
				h.AI().Desires().Clear()
				if row.healed {
					h.Scratch().SetInt(npc.IntAI3, 1)
				}
			})
			w.ticks(t, 1)
			if fightingIntention(h) {
				t.Fatalf("setup: intention %v, want idle", h.AI().CurrentIntention())
			}
			w.moveNextTo(t, h, 300, 0)
			w.srv.ReadQueued(t, w.srv.Client)

			w.hit(t, private, 10)
			got := says(w.srv.ReadQueued(t, w.srv.Client), listener.ObjectID())
			heard := slices.Contains(got, fmt.Sprintf("event 10033 %d", w.player.ObjectID()))
			if heard == row.healed {
				t.Fatalf("events heard after a hit on its private = %q; want event 10033 with the character: %v", got, !row.healed)
			}
		})
	}
}

// i6bHereticDen is an npcmaker spawning one heretic in a 400 by 400
// territory.
const i6bHereticDen = `<?xml version="1.0" encoding="utf-8"?>
<list>
	<territory name="heretic_den" minZ="-1000" maxZ="1000"><node x="0" y="0"/><node x="400" y="0"/><node x="400" y="400"/><node x="0" y="400"/></territory>
	<npcmaker name="heretic_den" territory="heretic_den" maximumNpcs="1">
		<ai type="default_maker"/>
		<npc id="22163" total="1" pos="40;20;30;0" respawn="60sec"/>
	</npcmaker>
</list>`

// TestPartyLeaderWizardHereticWalksHomeFromOutside runs the heretic's
// out-of-territory hook on a heretic its npcmaker spawned: arriving
// outside its territory it drops its hate list, wants to walk to its spawn
// point with weight 100 more (on top of the 30 its created hook queued)
// and broadcasts event 10035 with its own id.
func TestPartyLeaderWizardHereticWalksHomeFromOutside(t *testing.T) {
	t.Parallel()
	dir := t.TempDir()
	if err := os.WriteFile(filepath.Join(dir, "den.xml"), []byte(i6bHereticDen), 0o644); err != nil {
		t.Fatalf("write spawnlist: %v", err)
	}
	table, err := gamexml.LoadSpawnlist(dir, zerolog.Nop(), 1)
	if err != nil {
		t.Fatalf("load spawnlist: %v", err)
	}
	w := bootIndividual6b(t, raidBoot{opts: []gameservertest.Option{gameservertest.WithNpcSpawns(table)}}, i6bCurseTemplate(i6bHereticID), warriorTemplate(plwListenerID, nil))
	var h *npc.Hostile
	for _, obj := range w.srv.State.Objects() {
		if o, ok := obj.(*npc.Hostile); ok && o.NpcID() == int(i6bHereticID) {
			h = o
		}
	}
	if h == nil {
		t.Fatal("the den spawned no heretic")
	}
	n, err := w.srv.NpcSpawns.AddSpawn(plwListenerID, 40, 800, 30, 0, false, 0)
	if err != nil {
		t.Fatalf("spawn the listener: %v", err)
	}
	w.srv.Settle(t)
	w.srv.ReadQueued(t, w.srv.Client)
	home := script.NPCOf(h).SpawnLocation()
	w.onQueue(t, h, func() { h.AI().Hates().Add(w.player, 100) })

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
	var frames [][]byte
	for i := 0; h.IsMoving(); i++ {
		if i >= int(20*time.Second/move.PositionUpdateInterval) {
			t.Fatal("walk never arrived")
		}
		w.srv.TickPositions()
	}
	w.srv.Settle(t)
	frames = append(frames, w.srv.ReadQueued(t, w.srv.Client)...)
	if h.InTerritory() {
		t.Fatal("setup: the walk ended inside the territory")
	}
	if d, ok := desire(h, actorai.IntentionMoveTo); !ok || d.Weight != 30+100 || d.Location.X != home.X || d.Location.Y != home.Y {
		t.Fatalf("move desire after arriving outside = %+v (%v), want its standing walk home (30) raised by 100 to its spawn point %d,%d", d, ok, home.X, home.Y)
	}
	if got := w.hateListOf(h); got != 0 {
		t.Fatalf("hate list entry after leaving its territory = %v, want none", got)
	}
	if got := says(frames, n.ObjectID()); !slices.Contains(got, fmt.Sprintf("event 10035 %d", h.ObjectID())) {
		t.Fatalf("events heard = %q, want event 10035 from the heretic", got)
	}
}

// TestPartyLeaderWizardDD2SummonCallsSilhouettesOnce runs
// PartyLeaderWizardDD2Summon's created and attacked hooks, every draw 0:
// its first hit places its three silhouettes around it (silhouette at -25,
// -25, the other two at -50, -50), each told (1000, the attacker); a later
// hit calls none.
func TestPartyLeaderWizardDD2SummonCallsSilhouettesOnce(t *testing.T) {
	t.Parallel()
	summoner := i6bTemplate(i6bSummonerID, npc.AIParams{
		"silhouette": fmt.Sprint(i6bSilhouette1), "silhouette2": fmt.Sprint(i6bSilhouette2), "silhouette3": fmt.Sprint(i6bSilhouette3), "SummonPrivateRate": "-1",
	}, map[npc.SkillType]modelskill.ID{npc.SkillTypeWLongRangeDDMagic: wLongDDID, npc.SkillTypeWShortRangeDDMagic: wShortDDID})
	w := bootIndividual6b(t, raidBoot{}, summoner, warriorTemplate(i6bSilhouette1, nil), warriorTemplate(i6bSilhouette2, nil), warriorTemplate(i6bSilhouette3, nil))
	h := w.spawn(t, i6bSummonerID, 300, 0)
	w.srv.InstallNPCCastSeam(t, h, dd2Skills(0))
	privatesOf(t, h, 0)
	x, y, _ := h.Position()

	w.hit(t, h, 10)
	silhouettes := privatesOf(t, h, 3)
	want := []struct {
		id     int32
		dx, dy int
	}{{i6bSilhouette1, -25, -25}, {i6bSilhouette2, -50, -50}, {i6bSilhouette3, -50, -50}}
	for i, s := range silhouettes {
		sx, sy, _ := s.Position()
		if got := int32(s.Instance.Template.ID); got != want[i].id || sx != x+want[i].dx || sy != y+want[i].dy {
			t.Errorf("silhouette %d: %d at %d,%d, want %d at %d,%d", i, got, sx, sy, want[i].id, x+want[i].dx, y+want[i].dy)
		}
		if got := [2]int32{s.Scratch().Int(npc.IntParam1), s.Scratch().Int(npc.IntParam2)}; got != [2]int32{1000, w.player.ObjectID()} {
			t.Errorf("silhouette %d spawn parameters = %v, want 1000 and the character", i, got)
		}
	}
	if got := h.Scratch().Int(npc.IntAI1); got != 1 {
		t.Fatalf("int slot 1 after the call = %d, want 1", got)
	}
	w.hit(t, h, 10)
	privatesOf(t, h, 3)
}

// i6bEyeTemplate is a fixture PartyLeaderWizardRangeCurseAggressive listing
// its range buff, range DD magic, debuff and debuff cancel.
func i6bEyeTemplate() *npc.Template {
	return i6bTemplate(i6bEyeID, nil, map[npc.SkillType]modelskill.ID{
		npc.SkillTypeRangeBuff: i6bRangeBuffID, npc.SkillTypeRangeDD: i6bRangeDDID, npc.SkillTypeDebuff: i6bDebuffID,
		npc.SkillTypeDebuffCancel: i6bDebuffCancelID, npc.SkillTypeWLongRangeDDMagic: wLongDDID, npc.SkillTypeWShortRangeDDMagic: wShortDDID,
	})
}

// i6bEyeSkills are a fixture eye's skills.
func i6bEyeSkills() *modelskill.Table {
	return modelskill.NewTable([]modelskill.Definition{
		npcSkill(i6bRangeBuffID, 1, false), harmlessSkill(i6bRangeDDID), harmlessSkill(i6bDebuffID), harmlessSkill(wLongDDID), harmlessSkill(wShortDDID),
	})
}

// i6bEyeCancel is the eye's debuff cancel: a buff holding an effect of its
// own stack for a minute.
func i6bEyeCancel() modelskill.Definition {
	d := ind1Skill(i6bDebuffCancelID, 0, true)
	d.Effects = []modelskill.EffectTemplate{{Name: "Buff", Time: 60, Icon: true, StackType: "i6b_cancel", StackOrder: 1}}
	return d
}

// TestPartyLeaderWizardRangeCurseCursesItsTarget runs
// PartyLeaderWizardRangeCurseAggressive's created, attacked and timer
// hooks, every draw 0. It spawns remembering itself as the creature it
// cursed. Its first hit wants its range buff on itself, weight 1000000,
// and arms the 5 s timer "5001", which wants its debuff on its top desire
// target, the character, weight 9999999, and remembers it. A second hit before the timer
// defers the curse by 5 s more.
func TestPartyLeaderWizardRangeCurseCursesItsTarget(t *testing.T) {
	t.Parallel()
	w := bootIndividual6b(t, raidBoot{}, i6bEyeTemplate())
	me := w.player.ObjectID()
	h := w.spawn(t, i6bEyeID, 300, 0)
	deferred := w.spawn(t, i6bEyeID, 0, 300)
	for _, n := range []*npc.Hostile{h, deferred} {
		w.srv.InstallNPCCastSeam(t, n, i6bEyeSkills())
		if c := script.NPCOf(n).Slot(script.SlotCreature2); !script.Same(c, script.NPCOf(n)) {
			t.Fatalf("creature slot 2 after spawning = %v, want the eye itself", c)
		}
	}

	w.hit(t, h, 10)
	if !i6bCastWeighs(h, i6bRangeBuffID, h.ObjectID(), 1000000) {
		t.Fatal("eye wants no range buff on itself on its first hit")
	}
	w.hit(t, deferred, 10)
	w.hit(t, deferred, 10)
	for _, n := range []*npc.Hostile{h, deferred} {
		w.onQueue(t, n, func() { script.NPCOf(n).SetTopDesireTarget(script.PlayerOf(w.player)) })
	}
	w.srv.Advance(t, 5*time.Second)
	if !i6bCastWeighs(h, i6bDebuffID, me, 9999999) {
		d, ok := castDesireOf(h, i6bDebuffID)
		t.Fatalf("eye debuff desire 5 s after its first hit = %+v (%v), want on the character, weight 9999999", d, ok)
	}
	if c := script.NPCOf(h).Slot(script.SlotCreature2); c == nil || c.ObjectID() != me {
		t.Fatalf("creature slot 2 after the curse = %v, want the character", c)
	}
	if d, ok := castDesireOf(deferred, i6bDebuffID); ok {
		t.Fatalf("eye hit twice wants its debuff after 5 s: %+v", d)
	}
	w.srv.Advance(t, 5*time.Second)
	if !i6bCastWeighs(deferred, i6bDebuffID, me, 9999999) {
		t.Fatal("eye hit twice wants no debuff after 10 s")
	}
}

// TestPartyLeaderWizardRangeCurseStrikesAndCancelsOnDeath runs the eye's
// see-creature, attacked and dying hooks, every draw 0. It attacks the
// character it sees, weight 200. With another top desire target, a hit
// wants its range DD magic on the attacker, weight 1000000. Dying with the
// character remembered as cursed, its debuff cancel lands on the
// character.
func TestPartyLeaderWizardRangeCurseStrikesAndCancelsOnDeath(t *testing.T) {
	t.Parallel()
	w := bootIndividual6b(t, raidBoot{opts: []gameservertest.Option{gameservertest.WithScriptSkills(i6bEyeCancel())}}, i6bEyeTemplate(), warriorTemplate(i6bFoeID, nil))
	me := w.player.ObjectID()
	h := w.spawn(t, i6bEyeID, 150, 0)
	foe := w.spawn(t, i6bFoeID, 200, 0)
	w.srv.InstallNPCCastSeam(t, h, i6bEyeSkills())
	w.ticks(t, 2)
	if got := h.AggroHate(w.player); got != 200 {
		t.Fatalf("hate of the character it sees = %v, want 200", got)
	}

	w.onQueue(t, h, func() {
		h.AI().Desires().Clear()
		script.NPCOf(h).SetTopDesireTarget(script.NPCOf(foe))
	})
	w.hit(t, h, 10)
	if !i6bCastWeighs(h, i6bRangeDDID, me, 1000000) {
		d, ok := castDesireOf(h, i6bRangeDDID)
		t.Fatalf("range DD desire with another top desire target = %+v (%v), want on the character, weight 1000000", d, ok)
	}

	w.onQueue(t, h, func() { script.NPCOf(h).SetSlot(script.SlotCreature2, script.PlayerOf(w.player)) })
	w.onQueue(t, h, func() { script.NPCOf(h).DoDie(nil) })
	ref := modelskill.Ref{ID: i6bDebuffCancelID, Level: 1}
	w.srv.AdvanceUntil(t, "the debuff cancel landed", func() bool {
		var level int32
		w.srv.RunScript(t, me, "I6bDirector", func(s *script.Script, p *script.Player) {
			level = s.AbnormalLevel(p, ref)
		})
		return level > 0
	})
}
