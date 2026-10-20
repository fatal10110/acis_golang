package ai

import (
	"testing"
	"time"

	actorai "github.com/fatal10110/acis_golang/internal/gameserver/model/actor/ai"
	"github.com/fatal10110/acis_golang/internal/gameserver/model/actor/npc"
	modelskill "github.com/fatal10110/acis_golang/internal/gameserver/model/skill"
	"github.com/fatal10110/acis_golang/internal/gameserver/network/serverpackets"
	"github.com/fatal10110/acis_golang/internal/gameserver/script"
	"github.com/fatal10110/acis_golang/internal/gameserver/script/ai/defaultnpc"
	"github.com/fatal10110/acis_golang/internal/gameserver/script/ai/monster"
	"github.com/fatal10110/acis_golang/internal/gameserver/script/ai/monsterbehavior"
	"github.com/fatal10110/acis_golang/internal/gameservertest"
)

// The skills the WizardUseSkill fixture NPCs list by type: two magic
// attacks reaching 2000, one costing 50 MP (a fixture NPC has none), a buff
// on one target, a self buff, magic attacks reaching only 100 and 50, and a
// debuff reaching 2000 whose effect lasts 60 s.
const (
	wusMagic1 = modelskill.ID(9931)
	wusMagic2 = modelskill.ID(9932)
	wusCostly = modelskill.ID(9933)
	wusBuff   = modelskill.ID(9934)
	wusSelf   = modelskill.ID(9935)
	wusShort  = modelskill.ID(9936)
	wusDebuff = modelskill.ID(9937)
	wusTiny   = modelskill.ID(9938)
)

// wusSkills are the fixture skills of the WizardUseSkill scenarios.
func wusSkills() []modelskill.Definition {
	costly := ind1Skill(wusCostly, 2000, false)
	costly.MPConsume = 50
	buff := ind1Skill(wusBuff, 2000, false)
	buff.Offensive, buff.SkillType, buff.Power = false, "BUFF", 0
	return []modelskill.Definition{
		ind1Skill(wusMagic1, 2000, false), ind1Skill(wusMagic2, 2000, false), costly, buff, ind1Skill(wusSelf, 0, true),
		ind1Skill(wusShort, 100, false), ind1Skill(wusTiny, 50, false), ind3bHeld(wusDebuff),
	}
}

// wusTemplate is a level 1 fixture monster of id with ai, listing skills by
// type.
func wusTemplate(id int32, ai npc.AIParams, skills map[npc.SkillType]modelskill.ID) *npc.Template {
	tmpl := warriorTemplate(id, ai)
	tmpl.SkillsByType = map[npc.SkillType]modelskill.Ref{}
	for t, sk := range skills {
		tmpl.SkillsByType[t] = modelskill.Ref{ID: sk, Level: 1}
	}
	return tmpl
}

// bootWizardUseSkill boots the WizardUseSkill chain beside templates, with
// the director script landOnCharacter runs through, every NPC casting the
// fixture skills, every draw 0 until a test queues others.
func bootWizardUseSkill(t *testing.T, templates ...*npc.Template) *ind7World {
	t.Helper()
	paths := []string{
		"script.ai.individual.DefaultNpc",
		"script.ai.individual.Monster.MonsterAI",
		"script.ai.individual.Monster.MonsterBehavior.MonsterBehavior",
		"script.ai.individual.Monster.MonsterBehavior.WizardBehavior.WizardBehavior",
		"script.ai.individual.Monster.MonsterBehavior.WizardBehavior.WizardUseSkill.WizardUseSkill",
	}
	catalog := script.Catalog{
		paths[0]: defaultnpc.New, paths[1]: monster.New, paths[2]: monsterbehavior.MonsterBehavior,
		paths[3]: monsterbehavior.WizardBehavior, paths[4]: monsterbehavior.WizardUseSkill,
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
	w := bootRaid(t, raidBoot{opts: []gameservertest.Option{
		gameservertest.WithScriptRand(draws.rand),
		gameservertest.WithNPCScripts(kinds, list, catalog),
		gameservertest.WithNpcSpawnSkills(modelskill.NewTable(wusSkills())),
		gameservertest.WithScriptSkills(wusSkills()...),
	}}, templates...)
	return &ind7World{ind1World: &ind1World{warriorWorld: w, draws: draws, heard: &ind1Heard{}}}
}

// TestWizardUseSkillWantsItsMainAttacksByDistance ports WizardUseSkill's
// attacked hook, every draw 0. A hit of 10 puts the character at the top of
// the wizard's hate list (WizardBehavior); the wizard then wants, at
// 1000000000 on its most hated, the main attack whose default distance
// band holds the character: its first skill from 300 up to 2000 away, its
// second under 300. It wants no attack: a wizard fills its hate list. Its
// next AI ticks cast the skill. A healer (IsHealer 1) skips its first
// skill, so 400 away it wants neither.
func TestWizardUseSkillWantsItsMainAttacksByDistance(t *testing.T) {
	t.Parallel()
	mains := map[npc.SkillType]modelskill.ID{npc.SkillTypeSkill01ID: wusMagic1, npc.SkillTypeSkill02ID: wusMagic2}
	w := bootWizardUseSkill(t,
		wusTemplate(20216, nil, mains), wusTemplate(20220, nil, mains),
		wusTemplate(21036, npc.AIParams{"IsHealer": "1"}, mains))
	me := w.player.ObjectID()
	far := w.spawn(t, 20216, 400, 0)
	near := w.spawn(t, 20220, 0, 100)
	healer := w.spawn(t, 21036, -400, 0)

	for _, h := range []*npc.Hostile{far, near, healer} {
		w.hit(t, h, 10)
	}
	w.wantCasts(t, "far", far, map[modelskill.ID]ind1Cast{wusMagic1: {me, 1000000000}})
	w.wantCasts(t, "near", near, map[modelskill.ID]ind1Cast{wusMagic2: {me, 1000000000}})
	w.wantCasts(t, "healer", healer, nil)
	for _, h := range []*npc.Hostile{far, near, healer} {
		if d, ok := attackOn(h, w.player); ok {
			t.Errorf("npc %d wants to attack the character: %+v", h.NpcID(), d)
		}
	}

	frames := w.ticks(t, 2)
	if got := framesOf(frames, serverpackets.OpcodeMagicSkillUse, far.ObjectID()); len(got) != 1 {
		t.Fatalf("far wizard MagicSkillUse frames = %d, want 1", len(got))
	}
}

// TestWizardUseSkillAttacksWhileItCannotPay ports the fallback of a main
// attack the wizard cannot pay for: its first skill costs 50 MP and the
// fixture has none, so a hit of 10 makes it attack the character at 1000
// instead and leave its skills (int slot 4 becomes 1). Its next hit, away
// from its skills, adds 10 to that attack. After 10 s WizardBehavior's
// check drops its attack desires and answers its most hated as if hit by
// it for 100: the skill still cannot be paid, so it attacks at 1000 again.
// The wizard cannot move, so the character stays inside the skill's band.
func TestWizardUseSkillAttacksWhileItCannotPay(t *testing.T) {
	t.Parallel()
	tmpl := wusTemplate(21038, nil, map[npc.SkillType]modelskill.ID{npc.SkillTypeSkill01ID: wusCostly})
	tmpl.CanMove = false
	w := bootWizardUseSkill(t, tmpl)
	h := w.spawn(t, 21038, 400, 0)

	w.hit(t, h, 10)
	w.wantCasts(t, "first hit", h, nil)
	if got := w.attackWeight(h); got != 1000 {
		t.Fatalf("attack desire after the first hit = %v, want 1000", got)
	}
	if got := ind1Slot(h, script.SlotIntAI4); got != 1 {
		t.Fatalf("int slot 4 = %d, want 1", got)
	}

	w.hit(t, h, 10)
	if got := w.attackWeight(h); got != 1010 {
		t.Fatalf("attack desire after the second hit = %v, want 1010", got)
	}

	w.srv.Advance(t, 10*time.Second)
	if got := w.attackWeight(h); got != 1000 {
		t.Fatalf("attack desire after the 10 s check = %v, want 1000", got)
	}
	if got := ind1Slot(h, script.SlotIntAI4); got != 1 {
		t.Fatalf("int slot 4 after the check = %d, want 1", got)
	}
}

// TestWizardUseSkillAnswersItsClan ports the clan attacked hook: a hit of
// 10 on a clan member makes the wizard, of the default Party_Type 0, want
// its third skill (a buff, Skill03_Type 3, always wanted) on that member
// when Skill03_Target is 2, at 1000000000 on a draw under 3333. A wizard
// of Party_Type 1 and Party_Loyalty 1 helps no clan member, so with
// Skill03_Target 4 it wants nothing.
func TestWizardUseSkillAnswersItsClan(t *testing.T) {
	t.Parallel()
	clan := func(tmpl *npc.Template) *npc.Template {
		tmpl.Clans, tmpl.ClanRange = []string{"wizards"}, 600
		return tmpl
	}
	buff := map[npc.SkillType]modelskill.ID{npc.SkillTypeSkill03ID: wusBuff}
	w := bootWizardUseSkill(t,
		clan(wusTemplate(20216, nil, nil)),
		clan(wusTemplate(20220, npc.AIParams{"Skill03_Target": "2", "Skill03_Type": "3"}, buff)),
		clan(wusTemplate(21036, npc.AIParams{"Skill03_Target": "4", "Skill03_Type": "3", "Party_Type": "1", "Party_Loyalty": "1"}, buff)))
	victim := w.spawn(t, 20216, 400, 0)
	helper := w.spawn(t, 20220, 400, 100)
	loyal := w.spawn(t, 21036, 400, -100)

	w.hit(t, victim, 10)
	w.wantCasts(t, "helper", helper, map[modelskill.ID]ind1Cast{wusBuff: {victim.ObjectID(), 1000000000}})
	w.wantCasts(t, "loyal", loyal, nil)
}

// TestWizardUseSkillPartyCallReachesTheCaller ports the party attacked
// hook's target rule as the reference groups it: for a leader (Party_Type
// 2), a call from anyone but its master picks the calling member whatever
// the skill's target says, unless that target is the most hated or the
// hit one. So the leader whose third skill aims at itself (Skill03_Target
// 3) wants it on the private that was hit.
func TestWizardUseSkillPartyCallReachesTheCaller(t *testing.T) {
	t.Parallel()
	leaderTmpl := wusTemplate(20216, npc.AIParams{"Party_Type": "2", "Skill03_Target": "3", "Skill03_Type": "3"},
		map[npc.SkillType]modelskill.ID{npc.SkillTypeSkill03ID: wusBuff})
	leaderTmpl.Privates = []npc.PrivateEntry{{NpcID: 21039, Weight: 1}}
	w := bootWizardUseSkill(t, leaderTmpl, wusTemplate(21039, npc.AIParams{"Party_Type": "1", "Party_Loyalty": "2"}, nil))
	leader := w.spawn(t, 20216, 400, 0)
	private := privatesOf(t, leader, 1)[0]

	w.hit(t, private, 10)
	w.wantCasts(t, "leader", leader, map[modelskill.ID]ind1Cast{wusBuff: {private.ObjectID(), 1000000000}})
}

// TestWizardUseSkillCastsAtWhatItSees ports the see-creature hook: an
// aggressive wizard (IsAggressive 1) that sees the character wants its
// first skill, aimed at what it sees (Skill01_Target 1) at any distance
// (Skill01_Check_Dist 0), on it at 1000000000; a wizard that is not
// aggressive wants nothing.
func TestWizardUseSkillCastsAtWhatItSees(t *testing.T) {
	t.Parallel()
	first := map[npc.SkillType]modelskill.ID{npc.SkillTypeSkill01ID: wusMagic1}
	params := npc.AIParams{"Skill01_Target": "1", "Skill01_Check_Dist": "0"}
	aggressive := npc.AIParams{"IsAggressive": "1", "Skill01_Target": "1", "Skill01_Check_Dist": "0"}
	w := bootWizardUseSkill(t, wusTemplate(21040, aggressive, first), wusTemplate(21394, params, first))
	seer := w.spawn(t, 21040, 150, 0)
	calm := w.spawn(t, 21394, -150, 0)

	w.ticks(t, 2)
	w.wantCasts(t, "aggressive", seer, map[modelskill.ID]ind1Cast{wusMagic1: {w.player.ObjectID(), 1000000000}})
	w.wantCasts(t, "calm", calm, nil)
}

// TestWizardUseSkillAttacksWhatItSeesAwayFromItsSkills ports the
// see-creature hook away from its skills: an aggressive wizard whose first
// skill, aimed at what it sees, it cannot pay for attacks the character it
// sees at 1000 and leaves its skills. The character steps out of its sight,
// which drops that desire, and back: seen again from inside its territory,
// it wants to attack it at 300, and no skill.
func TestWizardUseSkillAttacksWhatItSeesAwayFromItsSkills(t *testing.T) {
	t.Parallel()
	tmpl := wusTemplate(21040, npc.AIParams{"IsAggressive": "1", "Skill01_Target": "1", "Skill01_Check_Dist": "0"},
		map[npc.SkillType]modelskill.ID{npc.SkillTypeSkill01ID: wusCostly})
	tmpl.CanMove = false
	w := bootWizardUseSkill(t, tmpl)
	h := w.spawn(t, 21040, 150, 0)

	w.ticks(t, 2)
	if got := w.attackWeight(h); got != 1000 {
		t.Fatalf("attack desire on first sight = %v, want 1000", got)
	}
	if got := ind1Slot(h, script.SlotIntAI4); got != 1 {
		t.Fatalf("int slot 4 = %d, want 1", got)
	}

	w.moveCharacter(t, h, 3000, 0)
	w.ticks(t, 2)
	if got := w.attackWeight(h); got != 0 {
		t.Fatalf("attack desire with the character out of sight = %v, want none", got)
	}
	w.moveCharacter(t, h, 150, 0)
	w.ticks(t, 1)
	if got := w.attackWeight(h); got != 300 {
		t.Fatalf("attack desire on second sight = %v, want 300", got)
	}
	w.wantCasts(t, "second sight", h, nil)
}

// TestWizardUseSkillRecastsAfterEachCast ports the use-skill finished hook,
// which calls no parent: once any cast of the wizard ends, it wants its
// second skill on its most hated when the second skill's distance check
// holds, else its first. The wizards cannot move and their first skill
// reaches only 100, so a wish for it 400 away stays queued. Hit by the
// character, a wizard wants that skill; with its desires dropped, a cast of
// its self buff wants it again on the character, at 1000000000. A wizard
// hating no one wants nothing after the same cast.
func TestWizardUseSkillRecastsAfterEachCast(t *testing.T) {
	t.Parallel()
	parked := func(id int32) *npc.Template {
		tmpl := wusTemplate(id, nil, map[npc.SkillType]modelskill.ID{npc.SkillTypeSkill01ID: wusShort, npc.SkillTypeSkill02ID: wusMagic2})
		tmpl.CanMove = false
		return tmpl
	}
	w := bootWizardUseSkill(t, parked(21395), parked(20220))
	me := w.player.ObjectID()
	hated := w.spawn(t, 21395, 400, 0)
	lonely := w.spawn(t, 20220, -400, 0)

	w.hit(t, hated, 10)
	w.wantCasts(t, "after the hit", hated, map[modelskill.ID]ind1Cast{wusShort: {me, 1000000000}})
	w.castOnce(t, hated, wusSelf)
	w.wantCasts(t, "after the cast", hated, map[modelskill.ID]ind1Cast{wusShort: {me, 1000000000}})

	w.castOnce(t, lonely, wusSelf)
	if d, ok := desire(lonely, actorai.IntentionCast); ok {
		t.Fatalf("a wizard hating no one wants %+v after its cast", d)
	}
}

// TestWizardUseSkillDebuffsOnlyWhatLacksTheEffect ports the type 1 and 2
// rule of the cast helper: a debuff (Skill03_Type 1) or a buff (2) is
// wanted only on a target that holds no effect of its kind. Hit by the
// character, both wizards want their third skill on it (Skill03_Target 1)
// at 1000000000; once its effect holds on the character, the next hit
// wants it on neither.
func TestWizardUseSkillDebuffsOnlyWhatLacksTheEffect(t *testing.T) {
	t.Parallel()
	debuff := map[npc.SkillType]modelskill.ID{npc.SkillTypeSkill03ID: wusDebuff}
	w := bootWizardUseSkill(t,
		wusTemplate(21036, npc.AIParams{"Skill03_Target": "1", "Skill03_Type": "1"}, debuff),
		wusTemplate(20216, npc.AIParams{"Skill03_Target": "1", "Skill03_Type": "2"}, debuff))
	me := w.player.ObjectID()
	type1 := w.spawn(t, 21036, 400, 0)
	type2 := w.spawn(t, 20216, -400, 0)

	for _, h := range []*npc.Hostile{type1, type2} {
		w.hit(t, h, 10)
		w.wantCasts(t, "first hit", h, map[modelskill.ID]ind1Cast{wusDebuff: {me, 1000000000}})
		w.clearDesires(t, h)
	}
	w.landOnCharacter(t, wusDebuff, type1)
	for _, h := range []*npc.Hostile{type1, type2} {
		w.hit(t, h, 10)
		w.wantCasts(t, "hit on the affected character", h, nil)
	}
}

// TestWizardUseSkillHoldsItsGround ports the MovingAttack 0 branches. A hit
// makes a wizard that holds its ground want its first skill on the
// character from where it stands (a cast desire that does not move). One
// that cannot pay for that skill wants to attack the character from where
// it stands at 1000 and leaves its skills. Once its AI takes that attack
// up, the character is its top desire target; the next hit, off its skills,
// drops that attack, which it cannot land from 400 away, and wants a held
// attack on the attacker at 10.
func TestWizardUseSkillHoldsItsGround(t *testing.T) {
	t.Parallel()
	hold := npc.AIParams{"MovingAttack": "0"}
	caster := wusTemplate(21394, hold, map[npc.SkillType]modelskill.ID{npc.SkillTypeSkill01ID: wusMagic1})
	poor := wusTemplate(21395, hold, map[npc.SkillType]modelskill.ID{npc.SkillTypeSkill01ID: wusCostly})
	caster.CanMove, poor.CanMove = false, false
	w := bootWizardUseSkill(t, caster, poor)
	c := w.spawn(t, 21394, 400, 0)
	p := w.spawn(t, 21395, -400, 0)

	w.hit(t, c, 10)
	d, ok := desire(c, actorai.IntentionCast)
	if !ok || d.Skill.ID != wusMagic1 || d.Weight != 1000000000 || d.MoveToTarget || d.FinalTarget == nil || d.FinalTarget.ObjectID() != w.player.ObjectID() {
		t.Fatalf("holding caster's cast desire = %+v (%v), want skill %d on the character at 1e9, not moving", d, ok, wusMagic1)
	}

	w.hit(t, p, 10)
	a, ok := attackOn(p, w.player)
	if !ok || a.Weight != 1000 || a.MoveToTarget {
		t.Fatalf("attack desire of the wizard that cannot pay = %+v (%v), want 1000, not moving", a, ok)
	}
	if got := ind1Slot(p, script.SlotIntAI4); got != 1 {
		t.Fatalf("int slot 4 = %d, want 1", got)
	}
	w.ticks(t, 1)
	if top := script.NPCOf(p).TopDesireTarget(); !script.Same(top, script.PlayerOf(w.player)) {
		t.Fatalf("top desire target = %v, want the character", top)
	}
	w.hit(t, p, 10)
	a, ok = attackOn(p, w.player)
	if !ok || a.Weight != 10 || a.MoveToTarget {
		t.Fatalf("attack desire after the hit off its skills = %+v (%v), want 10, not moving", a, ok)
	}
}

// TestWizardUseSkillRecastsItsSecondSkillInItsBand ports the use-skill
// finished hook's first arm: with its most hated 150 away, inside its
// second skill's band (Skill02_Dist_Max 200, strictly between 0 and 200),
// a finished cast of its self buff makes the wizard want its second skill
// on the character, not its first. It cannot move, and the second skill
// reaches only 50, so the wish stays queued.
func TestWizardUseSkillRecastsItsSecondSkillInItsBand(t *testing.T) {
	t.Parallel()
	tmpl := wusTemplate(21036, npc.AIParams{"Skill02_Dist_Max": "200"},
		map[npc.SkillType]modelskill.ID{npc.SkillTypeSkill01ID: wusShort, npc.SkillTypeSkill02ID: wusTiny})
	tmpl.CanMove = false
	w := bootWizardUseSkill(t, tmpl)
	h := w.spawn(t, 21036, 150, 0)

	w.hit(t, h, 10)
	w.castOnce(t, h, wusSelf)
	w.wantCasts(t, "after the cast", h, map[modelskill.ID]ind1Cast{wusTiny: {w.player.ObjectID(), 1000000000}})
}
