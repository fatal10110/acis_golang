package ai

import (
	"testing"

	actorai "github.com/fatal10110/acis_golang/internal/gameserver/model/actor/ai"
	"github.com/fatal10110/acis_golang/internal/gameserver/model/actor/npc"
	modelskill "github.com/fatal10110/acis_golang/internal/gameserver/model/skill"
	"github.com/fatal10110/acis_golang/internal/gameserver/script"
	"github.com/fatal10110/acis_golang/internal/gameserver/script/ai/defaultnpc"
	"github.com/fatal10110/acis_golang/internal/gameserver/script/ai/monster"
	"github.com/fatal10110/acis_golang/internal/gameserver/script/ai/monsterbehavior"
	"github.com/fatal10110/acis_golang/internal/gameservertest"
)

// bootWarriorUseSkill boots the WarriorUseSkill chain beside templates,
// with extra and the director script landOnCharacter runs through, every
// NPC casting the WizardUseSkill fixture skills, every draw 0 until a test
// queues others.
func bootWarriorUseSkill(t *testing.T, extra raidBoot, templates ...*npc.Template) *ind7World {
	t.Helper()
	paths := []string{
		"script.ai.individual.DefaultNpc",
		"script.ai.individual.Monster.MonsterAI",
		"script.ai.individual.Monster.MonsterBehavior.MonsterBehavior",
		"script.ai.individual.Monster.MonsterBehavior.WarriorBehavior.WarriorBehavior",
		"script.ai.individual.Monster.MonsterBehavior.WarriorBehavior.WarriorUseSkill.WarriorUseSkill",
	}
	catalog := script.Catalog{
		paths[0]: defaultnpc.New, paths[1]: monster.New, paths[2]: monsterbehavior.MonsterBehavior,
		paths[3]: monsterbehavior.WarriorBehavior, paths[4]: monsterbehavior.WarriorUseSkill,
	}
	const director = "test.Ind1Director"
	catalog[director] = func() script.Script { return script.Script{} }
	var list []script.Listing
	for _, p := range append(paths, director) {
		list = append(list, script.Listing{Path: p})
	}
	kinds := map[int32]script.NPCKind{}
	for _, tmpl := range templates {
		kinds[int32(tmpl.ID)] = script.KindHostile
	}
	draws := &ind1Draws{}
	w := bootRaid(t, raidBoot{ready: extra.ready, opts: append([]gameservertest.Option{
		gameservertest.WithScriptRand(draws.rand),
		gameservertest.WithNPCScripts(kinds, list, catalog),
		gameservertest.WithNpcSpawnSkills(modelskill.NewTable(wusSkills())),
		gameservertest.WithScriptSkills(wusSkills()...),
	}, extra.opts...)}, templates...)
	return &ind7World{ind1World: &ind1World{warriorWorld: w, draws: draws, heard: &ind1Heard{}}}
}

// TestWarriorUseSkillAnswersAHitOnEachTarget ports WarriorUseSkill's
// attacked hook, every draw 0, under the 3333 every skill is wanted on by
// default, at 1000000000 at any distance:
//   - Skill0<n>_Target 1 aims at the attacker and 2 at the warrior itself;
//   - a healer (IsHealer 1) skips its first skill;
//   - 0 aims at its top desire target as the hit began: none on the first
//     hit, the character on the second, once WarriorBehavior wants to
//     attack it.
func TestWarriorUseSkillAnswersAHitOnEachTarget(t *testing.T) {
	t.Parallel()
	w := bootWarriorUseSkill(t, raidBoot{},
		wusTemplate(20136, npc.AIParams{"Skill01_Target": "1", "Skill02_Target": "2"},
			map[npc.SkillType]modelskill.ID{npc.SkillTypeSkill01ID: wusMagic1, npc.SkillTypeSkill02ID: wusSelf}),
		wusTemplate(20214, npc.AIParams{"IsHealer": "1", "Skill01_Target": "1", "Skill02_Target": "1"},
			map[npc.SkillType]modelskill.ID{npc.SkillTypeSkill01ID: wusMagic1, npc.SkillTypeSkill02ID: wusMagic2}),
		wusTemplate(20215, nil, map[npc.SkillType]modelskill.ID{npc.SkillTypeSkill01ID: wusMagic1}))
	me := w.player.ObjectID()
	both := w.spawn(t, 20136, 400, 0)
	healer := w.spawn(t, 20214, -400, 0)
	top := w.spawn(t, 20215, 0, 400)

	for _, h := range []*npc.Hostile{both, healer, top} {
		w.hit(t, h, 10)
	}
	w.wantCasts(t, "attacker and self", both, map[modelskill.ID]ind1Cast{wusMagic1: {me, 1000000000}, wusSelf: {both.ObjectID(), 1000000000}})
	w.wantCasts(t, "healer", healer, map[modelskill.ID]ind1Cast{wusMagic2: {me, 1000000000}})
	w.wantCasts(t, "top desire target, first hit", top, nil)
	if got := w.attackWeight(top); got != 100 {
		t.Fatalf("attack desire after the first hit = %v, want 100 (WarriorBehavior)", got)
	}
	w.hit(t, top, 10)
	w.wantCasts(t, "top desire target, second hit", top, map[modelskill.ID]ind1Cast{wusMagic1: {me, 1000000000}})
}

// TestWarriorUseSkillDrawsOnlyForAPositiveDesire ports the order of the
// cast helper's checks: a skill whose desire is not positive
// (Skill01_Desire 0) is dropped before its draw, so the one draw of 0
// queued for the hit goes to the second skill, which the warrior then
// wants; the draws after it are 9999, above every probability.
func TestWarriorUseSkillDrawsOnlyForAPositiveDesire(t *testing.T) {
	t.Parallel()
	w := bootWarriorUseSkill(t, raidBoot{},
		wusTemplate(20217, npc.AIParams{"Skill01_Target": "1", "Skill01_Desire": "0", "Skill02_Target": "1"},
			map[npc.SkillType]modelskill.ID{npc.SkillTypeSkill01ID: wusMagic1, npc.SkillTypeSkill02ID: wusMagic2}))
	h := w.spawn(t, 20217, 400, 0)

	w.draws.set(9999, 0)
	w.hit(t, h, 10)
	w.wantCasts(t, "after the hit", h, map[modelskill.ID]ind1Cast{wusMagic2: {w.player.ObjectID(), 1000000000}})
}

// TestWarriorUseSkillHoldsItsGround ports the MovingAttack 0 branch of the
// cast helper: a warrior that holds its ground wants its skill on the
// attacker from where it stands, a cast desire that does not move.
func TestWarriorUseSkillHoldsItsGround(t *testing.T) {
	t.Parallel()
	tmpl := wusTemplate(20218, npc.AIParams{"MovingAttack": "0", "Skill01_Target": "1"},
		map[npc.SkillType]modelskill.ID{npc.SkillTypeSkill01ID: wusMagic1})
	tmpl.CanMove = false
	w := bootWarriorUseSkill(t, raidBoot{}, tmpl)
	h := w.spawn(t, 20218, 400, 0)

	w.hit(t, h, 10)
	d, ok := desire(h, actorai.IntentionCast)
	if !ok || d.Skill.ID != wusMagic1 || d.Weight != 1000000000 || d.MoveToTarget || d.FinalTarget == nil || d.FinalTarget.ObjectID() != w.player.ObjectID() {
		t.Fatalf("holding warrior's cast desire = %+v (%v), want skill %d on the character at 1e9, not moving", d, ok, wusMagic1)
	}
}

// TestWarriorUseSkillDebuffsOnlyWhatLacksTheEffect ports the type 1 rule
// of the cast helper: a debuff (Skill03_Type 1) is wanted on the attacker
// (Skill03_Target 1) only while it holds no effect of its kind.
func TestWarriorUseSkillDebuffsOnlyWhatLacksTheEffect(t *testing.T) {
	t.Parallel()
	w := bootWarriorUseSkill(t, raidBoot{},
		wusTemplate(20219, npc.AIParams{"Skill03_Target": "1", "Skill03_Type": "1"},
			map[npc.SkillType]modelskill.ID{npc.SkillTypeSkill03ID: wusDebuff}))
	h := w.spawn(t, 20219, 400, 0)

	w.hit(t, h, 10)
	w.wantCasts(t, "first hit", h, map[modelskill.ID]ind1Cast{wusDebuff: {w.player.ObjectID(), 1000000000}})
	w.clearDesires(t, h)
	w.landOnCharacter(t, wusDebuff, h)
	w.hit(t, h, 10)
	w.wantCasts(t, "hit on the affected character", h, nil)
}

// TestWarriorUseSkillAnswersItsClan ports the clan attacked hook: a hit of
// 10 on a clan member makes a warrior of the default Party_Type 0 want its
// third skill (a buff, Skill03_Type 3) on that member when Skill03_Target
// is 2, and one whose Skill03_Target is 3 want it on itself. A warrior
// younger than its Aggressive_Time wants nothing.
func TestWarriorUseSkillAnswersItsClan(t *testing.T) {
	t.Parallel()
	clan := func(tmpl *npc.Template) *npc.Template {
		tmpl.Clans, tmpl.ClanRange = []string{"warriors"}, 600
		return tmpl
	}
	buff := map[npc.SkillType]modelskill.ID{npc.SkillTypeSkill03ID: wusBuff}
	w := bootWarriorUseSkill(t, raidBoot{},
		clan(wusTemplate(20136, nil, nil)),
		clan(wusTemplate(20214, npc.AIParams{"Skill03_Target": "2", "Skill03_Type": "3"}, buff)),
		clan(wusTemplate(20215, npc.AIParams{"Skill03_Target": "3", "Skill03_Type": "3"}, buff)),
		clan(wusTemplate(20217, npc.AIParams{"Skill03_Target": "2", "Skill03_Type": "3", "Aggressive_Time": "100000"}, buff)))
	victim := w.spawn(t, 20136, 400, 0)
	helper := w.spawn(t, 20214, 400, 100)
	self := w.spawn(t, 20215, 400, -100)
	young := w.spawn(t, 20217, 500, 0)

	w.hit(t, victim, 10)
	w.wantCasts(t, "helper", helper, map[modelskill.ID]ind1Cast{wusBuff: {victim.ObjectID(), 1000000000}})
	w.wantCasts(t, "self", self, map[modelskill.ID]ind1Cast{wusBuff: {self.ObjectID(), 1000000000}})
	w.wantCasts(t, "young", young, nil)
}

// TestWarriorUseSkillPartyCallCastsFromTheCaller ports the party attacked
// hook as the reference writes it. The leader (Party_Type 2) is hit; its
// private (Party_Type 1, Party_Loyalty 0) is told. For the private's first
// skill, aimed at the calling member (Skill01_Target 2), it is the caller,
// the leader, that wants the skill, on the attacker. For its second, aimed
// at itself (Skill02_Target 3), the private wants it on the attacker. A
// private of Party_Type 0 wants nothing.
func TestWarriorUseSkillPartyCallCastsFromTheCaller(t *testing.T) {
	t.Parallel()
	leaderTmpl := wusTemplate(20751, npc.AIParams{"Party_Type": "2"}, nil)
	leaderTmpl.Privates = []npc.PrivateEntry{{NpcID: 20752, Weight: 1}, {NpcID: 20753, Weight: 1}}
	skills := map[npc.SkillType]modelskill.ID{npc.SkillTypeSkill01ID: wusMagic1, npc.SkillTypeSkill02ID: wusMagic2}
	w := bootWarriorUseSkill(t, raidBoot{}, leaderTmpl,
		wusTemplate(20752, npc.AIParams{"Party_Type": "1", "Party_Loyalty": "0", "Skill01_Target": "2", "Skill02_Target": "3"}, skills),
		wusTemplate(20753, npc.AIParams{"Skill01_Target": "1", "Skill02_Target": "1"}, skills))
	me := w.player.ObjectID()
	leader := w.spawn(t, 20751, 400, 0)
	var follower, plain *npc.Hostile
	for _, p := range privatesOf(t, leader, 2) {
		if p.NpcID() == 20752 {
			follower = p
		} else {
			plain = p
		}
	}

	w.hit(t, leader, 10)
	w.wantCasts(t, "leader", leader, map[modelskill.ID]ind1Cast{wusMagic1: {me, 1000000000}})
	w.wantCasts(t, "follower", follower, map[modelskill.ID]ind1Cast{wusMagic2: {me, 1000000000}})
	w.wantCasts(t, "Party_Type 0", plain, nil)
}

// TestWarriorUseSkillCastsAtWhatItSees ports the see-creature hook: an
// aggressive warrior (IsAggressive 1) that sees the character wants its
// first skill, aimed at what it sees (Skill01_Target 1), on it at
// 1000000000; one that is not aggressive, or younger than its
// Aggressive_Time, wants nothing.
func TestWarriorUseSkillCastsAtWhatItSees(t *testing.T) {
	t.Parallel()
	first := map[npc.SkillType]modelskill.ID{npc.SkillTypeSkill01ID: wusMagic1}
	w := bootWarriorUseSkill(t, raidBoot{},
		wusTemplate(20222, npc.AIParams{"IsAggressive": "1", "Skill01_Target": "1"}, first),
		wusTemplate(20754, npc.AIParams{"Skill01_Target": "1"}, first),
		wusTemplate(21035, npc.AIParams{"IsAggressive": "1", "Aggressive_Time": "100000", "Skill01_Target": "1"}, first))
	seer := w.spawn(t, 20222, 150, 0)
	calm := w.spawn(t, 20754, -150, 0)
	young := w.spawn(t, 21035, 0, 150)

	w.ticks(t, 2)
	w.wantCasts(t, "aggressive", seer, map[modelskill.ID]ind1Cast{wusMagic1: {w.player.ObjectID(), 1000000000}})
	w.wantCasts(t, "calm", calm, nil)
	w.wantCasts(t, "young", young, nil)
}

// TestWarriorUseSkillAnswersASpell ports the see-spell hook: a warrior
// that sees the character cast its self buff wants its first skill, aimed
// at the caster (Skill01_Target 1), on it at 1000000000.
func TestWarriorUseSkillAnswersASpell(t *testing.T) {
	t.Parallel()
	w := bootWarriorUseSkill(t, ind1AggroSpellBoot(t),
		wusTemplate(21037, npc.AIParams{"Skill01_Target": "1"}, map[npc.SkillType]modelskill.ID{npc.SkillTypeSkill01ID: wusMagic1}))
	h := w.spawn(t, 21037, 150, 0)

	w.castAggroBuff()
	w.srv.AdvanceUntil(t, "the cast desire", func() bool { return len(ind1Casts(h)) == 1 })
	w.wantCasts(t, "after the spell", h, map[modelskill.ID]ind1Cast{wusMagic1: {w.player.ObjectID(), 1000000000}})
}
