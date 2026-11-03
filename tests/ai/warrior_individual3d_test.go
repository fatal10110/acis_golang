package ai

import (
	"testing"
	"time"

	actorai "github.com/fatal10110/acis_golang/internal/gameserver/model/actor/ai"
	"github.com/fatal10110/acis_golang/internal/gameserver/model/actor/npc"
	modelskill "github.com/fatal10110/acis_golang/internal/gameserver/model/skill"
	"github.com/fatal10110/acis_golang/internal/gameserver/script"
	"github.com/fatal10110/acis_golang/internal/gameserver/script/ai/warrior"
	"github.com/fatal10110/acis_golang/internal/gameservertest"
)

// The NPC ids the scenarios of the third individual behavior batch's third
// PR spawn (#188 PR 3), each a fixture monster of level 1.
const (
	ind3dSplashID     = int32(21058) // PartyLeaderCastSplashAggressive
	ind3dSplashTwoID  = int32(20966) // PartyLeaderCastSplashAggressive
	ind3dPabelID      = int32(22060) // PartyLeaderPabelgolemAggressive
	ind3dKetraID      = int32(21343) // PartyLeaderPatrolPhysicalSpecialStoneAggressive
	ind3dSlowID       = int32(22092) // PartyLeaderSlowTypeAggressive
	ind3dQuestID      = int32(27185) // QuestPartyLeader
	ind3dRoyalID      = int32(21434) // RoyalRushPartyLeader
	ind3dCasting3ID   = int32(29995) // PartyLeaderCasting3SkillsMagicalAggressive, bound by the test
	ind3dCasting3Two  = int32(29996) // PartyLeaderCasting3SkillsMagicalAggressive, bound by the test
	ind3dBehemothID   = int32(29069) // AntharasPrivateBehemothDragon
	ind3dBomberID     = int32(29070) // the Dragon Bomber it leads
	ind3dCasting3Path = "test.PartyLeaderCasting3SkillsMagicalAggressive"
)

// The skills the PR's fixture NPCs list by type.
const (
	ind3dSlowSkill        = modelskill.ID(9801)
	ind3dSplashSkill      = modelskill.ID(9802)
	ind3dSpecialSkill     = modelskill.ID(9803)
	ind3dRangeSpecial     = modelskill.ID(9804)
	ind3dDebuffSkill      = modelskill.ID(9805)
	ind3dBuffSkill        = modelskill.ID(9806)
	ind3dSpecial1Skill    = modelskill.ID(9807)
	ind3dDDMagic1Skill    = modelskill.ID(9808)
	ind3dDDMagic2Skill    = modelskill.ID(9809)
	ind3dDefaultDDMagic   = modelskill.ID(4001)
	ind3dDefaultDebuff    = modelskill.ID(4037)
	ind3dSummonSlaveParam = "27037" // partyPrivateID
)

// ind3dBatch are the PR's classes and the parent they share, in
// scripts.xml order.
var ind3dBatch = []struct {
	path string
	ctor func() script.Script
}{
	{"script.ai.individual.Monster.WarriorBase.Warrior.PartyLeaderWarrior.PartyLeaderCastSplashAggressive", warrior.PartyLeaderCastSplashAggressive},
	{"script.ai.individual.Monster.WarriorBase.Warrior.PartyLeaderWarrior.PartyLeaderCasting3SkillsMagicalAggressive.AntharasPrivateBehemothDragon", warrior.AntharasPrivateBehemothDragon},
	{"script.ai.individual.Monster.WarriorBase.Warrior.PartyLeaderWarrior.PartyLeaderCasting3SkillsMagicalAggressive.PartyLeaderCasting3SkillsMagicalAggressive", warrior.PartyLeaderCasting3SkillsMagicalAggressive},
	{"script.ai.individual.Monster.WarriorBase.Warrior.PartyLeaderWarrior.PartyLeaderPabelgolemAggressive", warrior.PartyLeaderPabelgolemAggressive},
	{"script.ai.individual.Monster.WarriorBase.Warrior.PartyLeaderWarrior.PartyLeaderPatrolPhysicalSpecialStoneAggressive", warrior.PartyLeaderPatrolPhysicalSpecialStoneAggressive},
	{"script.ai.individual.Monster.WarriorBase.Warrior.PartyLeaderWarrior.PartyLeaderSlowTypeAggressive", warrior.PartyLeaderSlowTypeAggressive},
	{"script.ai.individual.Monster.WarriorBase.Warrior.PartyLeaderWarrior.PartyLeaderWarriorAggressive", warrior.PartyLeaderWarriorAggressive},
	{"script.ai.individual.Monster.WarriorBase.Warrior.PartyLeaderWarrior.QuestPartyLeader", warrior.QuestPartyLeader},
	{"script.ai.individual.Monster.WarriorBase.Warrior.PartyLeaderWarrior.RoyalRushPartyLeader", warrior.RoyalRushPartyLeader},
	// PartyLeaderCasting3SkillsMagicalAggressive binds no NPC of its own;
	// the scenarios bind its hooks to fixture ids to drive them.
	{ind3dCasting3Path, func() script.Script {
		s := warrior.PartyLeaderCasting3SkillsMagicalAggressive()
		s.NPCs = []int32{ind3dCasting3ID, ind3dCasting3Two}
		return s
	}},
}

// bootIndividual3d boots the PR's classes after the party Warriors beside
// templates, every draw 0 until a scenario queues others, with the
// director script the scenarios call privates through, and extra's
// options added last.
func bootIndividual3d(t *testing.T, extra raidBoot, templates ...*npc.Template) *ind1World {
	t.Helper()
	list, cat := partyScripts()
	for _, c := range ind3dBatch {
		list = append(list, script.Listing{Path: c.path})
		cat[c.path] = c.ctor
	}
	const director = "test.Ind1Director"
	cat[director] = func() script.Script { return script.Script{} }
	list = append(list, script.Listing{Path: director})
	kinds := map[int32]script.NPCKind{}
	for _, tmpl := range templates {
		kinds[int32(tmpl.ID)] = script.KindHostile
	}
	draws := &ind1Draws{}
	opts := append([]gameservertest.Option{
		gameservertest.WithScriptRand(draws.rand),
		gameservertest.WithNPCScripts(kinds, list, cat),
	}, extra.opts...)
	w := bootRaid(t, raidBoot{opts: opts, ready: extra.ready}, templates...)
	return &ind1World{warriorWorld: w, draws: draws, heard: &ind1Heard{}}
}

// ind3dTemplate is a fixture PR NPC of id with ai as its AI parameters,
// standing where it spawns and listing each of the PR's skills by type,
// declaring privates of the fixture private respawning after 240 s; with
// clan set, in the clan the scenario's NPCs share.
func ind3dTemplate(id int32, ai npc.AIParams, privates int, clan bool) *npc.Template {
	tmpl := leaderTemplate(id, ai, privates)
	tmpl.CanMove = false
	tmpl.SkillsByType = map[npc.SkillType]modelskill.Ref{
		npc.SkillTypeDDMagicSlow:              {ID: ind3dSlowSkill, Level: 1},
		npc.SkillTypeSelfRangeDDMagic:         {ID: ind3dSplashSkill, Level: 1},
		npc.SkillTypePhysicalSpecial:          {ID: ind3dSpecialSkill, Level: 1},
		npc.SkillTypeSelfRangePhysicalSpecial: {ID: ind3dRangeSpecial, Level: 1},
		npc.SkillTypeDebuff:                   {ID: ind3dDebuffSkill, Level: 1},
		npc.SkillTypeBuff:                     {ID: ind3dBuffSkill, Level: 1},
		npc.SkillTypePhysicalSpecial1:         {ID: ind3dSpecial1Skill, Level: 1},
		npc.SkillTypeDDMagic1:                 {ID: ind3dDDMagic1Skill, Level: 1},
		npc.SkillTypeDDMagic2:                 {ID: ind3dDDMagic2Skill, Level: 1},
	}
	if clan {
		tmpl.Clans, tmpl.ClanRange = []string{"party"}, 500
	}
	return tmpl
}

// ind3dSkills are the PR NPCs' skills, the attacks reaching 2000, the buff
// a one-target spell on a friend.
func ind3dSkills() *modelskill.Table {
	buff := ind1Skill(ind3dBuffSkill, 2000, false)
	buff.Target, buff.Offensive, buff.SkillType, buff.Power = modelskill.TargetOne, false, "BUFF", 0
	defs := []modelskill.Definition{buff, npcSkill(petrifyID, 1, true)}
	for _, id := range []modelskill.ID{
		ind3dSlowSkill, ind3dSplashSkill, ind3dSpecialSkill, ind3dRangeSpecial, ind3dDebuffSkill,
		ind3dSpecial1Skill, ind3dDDMagic1Skill, ind3dDDMagic2Skill, ind3dDefaultDDMagic, ind3dDefaultDebuff,
	} {
		defs = append(defs, ind1Skill(id, 2000, false))
	}
	return modelskill.NewTable(defs)
}

// ind3dSpawn spawns an NPC of id dx, dy from the character with the PR's
// skills to cast.
func (w *ind1World) ind3dSpawn(t *testing.T, id int32, dx, dy int) *npc.Hostile {
	t.Helper()
	n := w.spawn(t, id, dx, dy)
	w.srv.InstallNPCCastSeam(t, n, ind3dSkills())
	return n
}

// TestPartyLeaderCastSplashAggressiveSplashesTheNearMostHated ports
// PartyLeaderCastSplashAggressive's attacked and clan-attacked hooks,
// every draw 0. A hit by the creature it hates most from 150 makes it want
// its self-centred DD magic on itself, weight 1000000; from 250 it wants
// none. A settled clan member that hates the attacker of another member,
// standing 150 from it, wants the same on itself; the member hit from 300
// wants none.
func TestPartyLeaderCastSplashAggressiveSplashesTheNearMostHated(t *testing.T) {
	t.Parallel()
	w := bootIndividual3d(t, raidBoot{}, ind3dTemplate(ind3dSplashID, nil, 0, true), ind3dTemplate(ind3dSplashTwoID, nil, 0, true))
	n := w.ind3dSpawn(t, ind3dSplashID, 150, 0)
	w.hit(t, n, 10)
	w.clearDesires(t, n)
	w.hit(t, n, 10)
	w.wantCasts(t, "hit from 150", n, map[modelskill.ID]ind1Cast{ind3dSplashSkill: {n.ObjectID(), 1000000}})
	w.clearDesires(t, n)
	w.moveCharacter(t, n, 250, 0)
	w.hit(t, n, 10)
	w.wantCasts(t, "hit from 250", n, nil)

	victim := w.ind3dSpawn(t, ind3dSplashID, 300, 1500)
	member := w.ind3dSpawn(t, ind3dSplashTwoID, 150, 1500)
	w.moveCharacter(t, member, -150, 0)
	w.ticks(t, 8)
	if member.AggroHate(w.player) == 0 {
		t.Fatal("setup: the member does not hate the character it sees")
	}
	w.clearDesires(t, member)
	w.clearDesires(t, victim)
	w.hit(t, victim, 10)
	w.wantCasts(t, "member after the hit", member, map[modelskill.ID]ind1Cast{ind3dSplashSkill: {member.ObjectID(), 1000000}})
	w.wantCasts(t, "victim hit from 300", victim, nil)
}

// TestPartyLeaderPabelgolemAggressiveStrikesByParty ports
// PartyLeaderPabelgolemAggressive's attacked and party-attacked hooks,
// every draw 0. A hit by a character in no party makes it want its
// physical special on the character; a hit on its private, its debuff on
// the private's attacker; and a hit by a character in a party, its
// self-centred physical special on the character, each weight 1000000. A
// hit on the leader is also its own party call, so each hit on it wants
// the debuff too.
func TestPartyLeaderPabelgolemAggressiveStrikesByParty(t *testing.T) {
	t.Parallel()
	w := bootIndividual3d(t, raidBoot{}, ind3dTemplate(ind3dPabelID, nil, 1, false), warriorTemplate(partyPrivateID, nil))
	n := w.ind3dSpawn(t, ind3dPabelID, 1500, 0)
	private := privatesOf(t, n, 1)[0]
	me := w.player.ObjectID()

	w.hit(t, n, 10)
	w.wantCasts(t, "hit by a character in no party", n, map[modelskill.ID]ind1Cast{ind3dSpecialSkill: {me, 1000000}, ind3dDebuffSkill: {me, 1000000}})
	w.clearDesires(t, n)
	w.hit(t, private, 10)
	w.wantCasts(t, "after a hit on its private", n, map[modelskill.ID]ind1Cast{ind3dDebuffSkill: {me, 1000000}})
	w.clearDesires(t, n)
	w.joinMate(t)
	w.hit(t, n, 10)
	w.wantCasts(t, "hit by a character in a party", n, map[modelskill.ID]ind1Cast{ind3dRangeSpecial: {me, 1000000}, ind3dDebuffSkill: {me, 1000000}})
}

// TestPartyLeaderPatrolStoneSparesAndPetrifiesAllies ports
// PartyLeaderPatrolPhysicalSpecialStoneAggressive's see-creature and
// attacked hooks for a Ketra leader, every draw 0. A Ketra ally it sees is
// not attacked, and its hit makes the leader want to petrify it (weight
// 1000000 from its attacked hook and as much again from its own clan
// call) and stop hating it, with no other skill. A character with no
// alliance is attacked on sight, and its hit makes the leader want its
// debuff and physical special on it and its buff on itself.
func TestPartyLeaderPatrolStoneSparesAndPetrifiesAllies(t *testing.T) {
	t.Parallel()
	tmpl := ind3dTemplate(ind3dKetraID, nil, 0, false)
	tmpl.Clans, tmpl.ClanRange = []string{"ketra_orc_clan"}, 500
	w := bootIndividual3d(t, raidBoot{}, tmpl)
	ch := ind3Character(t, w)
	me := w.player.ObjectID()
	ch.SetVarkaKetraAlliance(1)

	allied := w.ind3dSpawn(t, ind3dKetraID, 150, 0)
	w.ticks(t, 2)
	if got := allied.AggroHate(w.player); got != 0 {
		t.Fatalf("hate of an ally it sees = %v, want none", got)
	}
	w.hit(t, allied, 10)
	w.wantCasts(t, "after an ally's hit", allied, map[modelskill.ID]ind1Cast{petrifyID: {me, 2000000}})
	if got := allied.AggroHate(w.player); got != 0 {
		t.Fatalf("hate of the ally that hit it = %v, want none", got)
	}

	ch.SetVarkaKetraAlliance(0)
	open := w.ind3dSpawn(t, ind3dKetraID, 0, 150)
	w.ticks(t, 2)
	if got := open.AggroHate(w.player); got != 200 {
		t.Fatalf("hate of a character with no alliance it sees = %v, want 200", got)
	}
	w.clearDesires(t, open)
	w.hit(t, open, 10)
	w.wantCasts(t, "after the hit of the most hated", open, map[modelskill.ID]ind1Cast{
		ind3dDebuffSkill: {me, 1000000}, ind3dSpecialSkill: {me, 1000000}, ind3dBuffSkill: {open.ObjectID(), 1000000},
	})
}

// TestPartyLeaderPatrolStoneAnswersItsParty ports
// PartyLeaderPatrolPhysicalSpecialStoneAggressive's party-attacked hook,
// every draw 0: a settled leader that is not attacking wants its physical
// special and debuff on its private's attacker and its buff on the
// private, each weight 1000000.
func TestPartyLeaderPatrolStoneAnswersItsParty(t *testing.T) {
	t.Parallel()
	w := bootIndividual3d(t, raidBoot{}, ind3dTemplate(ind3dKetraID, nil, 1, false), warriorTemplate(partyPrivateID, nil))
	n := w.ind3dSpawn(t, ind3dKetraID, 1500, 0)
	private := privatesOf(t, n, 1)[0]
	w.ticks(t, 8)
	if fightingIntention(n) {
		t.Fatalf("setup: leader intention = %v, want neither attack nor cast", n.AI().CurrentIntention())
	}
	me := w.player.ObjectID()
	w.hit(t, private, 10)
	w.wantCasts(t, "leader after a hit on its private", n, map[modelskill.ID]ind1Cast{
		ind3dSpecialSkill: {me, 1000000}, ind3dDebuffSkill: {me, 1000000}, ind3dBuffSkill: {private.ObjectID(), 1000000},
	})
}

// TestPartyLeaderPatrolStoneAnswersItsClan ports
// PartyLeaderPatrolPhysicalSpecialStoneAggressive's clan-attacked hook,
// every draw 0: a settled Ketra member that is not attacking wants its
// physical special and debuff on the attacker of another member, a
// character with no alliance, each weight 1000000.
func TestPartyLeaderPatrolStoneAnswersItsClan(t *testing.T) {
	t.Parallel()
	ketra := func(id int32) *npc.Template {
		tmpl := ind3dTemplate(id, nil, 0, false)
		tmpl.Clans, tmpl.ClanRange = []string{"ketra_orc_clan"}, 500
		return tmpl
	}
	w := bootIndividual3d(t, raidBoot{}, ketra(ind3dKetraID))
	victim := w.ind3dSpawn(t, ind3dKetraID, 1500, 0)
	member := w.ind3dSpawn(t, ind3dKetraID, 1500, 200)
	w.ticks(t, 8)
	if fightingIntention(member) {
		t.Fatalf("setup: member intention = %v, want neither attack nor cast", member.AI().CurrentIntention())
	}
	me := w.player.ObjectID()
	w.hit(t, victim, 10)
	w.wantCasts(t, "member after the hit", member, map[modelskill.ID]ind1Cast{ind3dSpecialSkill: {me, 1000000}, ind3dDebuffSkill: {me, 1000000}})
}

// TestPartyLeaderSlowTypeAggressiveSlowsWhatItFights ports
// PartyLeaderSlowTypeAggressive's created, see-creature, attacked and
// timer hooks, every draw 0. It spawns with i_ai0 and i_ai1 clear. Settled,
// it wants its slow on a character it sees 250 away, weight 1000000, and
// attacks it. A hit from 250 slows nothing and arms the 5 s timer "2001"
// (i_ai0); a second hit by the most hated marks i_ai1, so the first tick
// of the fight slows nothing, and clears i_ai0. The next hit arms the
// timer again, and its tick, unmarked, slows the most hated. A hit from 50
// slows the attacker.
func TestPartyLeaderSlowTypeAggressiveSlowsWhatItFights(t *testing.T) {
	t.Parallel()
	w := bootIndividual3d(t, raidBoot{}, ind3dTemplate(ind3dSlowID, nil, 0, false))
	n := w.ind3dSpawn(t, ind3dSlowID, 1500, 0)
	me := w.player.ObjectID()
	if a0, a1 := ind1Slot(n, script.SlotIntAI0), ind1Slot(n, script.SlotIntAI1); a0 != 0 || a1 != 0 {
		t.Fatalf("created i_ai0, i_ai1 = %d, %d, want 0, 0", a0, a1)
	}
	w.ticks(t, 9)
	w.moveCharacter(t, n, 250, 0)
	w.ticks(t, 1)
	w.wantCasts(t, "on sight", n, map[modelskill.ID]ind1Cast{ind3dSlowSkill: {me, 1000000}})
	if got := n.AggroHate(w.player); got != 200 {
		t.Fatalf("hate of the character it sees = %v, want 200", got)
	}

	w.clearDesires(t, n)
	w.hit(t, n, 10)
	w.wantCasts(t, "hit from 250", n, nil)
	if got := ind1Slot(n, script.SlotIntAI0); got != 1 {
		t.Fatalf("i_ai0 after the first hit = %d, want 1", got)
	}
	w.hit(t, n, 10)
	if got := ind1Slot(n, script.SlotIntAI1); got != 1 {
		t.Fatalf("i_ai1 after a hit by the most hated = %d, want 1", got)
	}
	w.ticks(t, 1)
	if !fightingIntention(n) {
		t.Fatalf("intention after the hits = %v, want attack or cast", n.AI().CurrentIntention())
	}
	w.srv.Advance(t, 5*time.Second)
	w.wantCasts(t, "after the marked tick", n, nil)
	if got := ind1Slot(n, script.SlotIntAI0); got != 0 {
		t.Fatalf("i_ai0 after the tick = %d, want 0", got)
	}

	w.onQueue(t, n, func() { script.NPCOf(n).SetInt(script.SlotIntAI1, 0) })
	w.srv.Advance(t, 5*time.Second)
	w.wantCasts(t, "after the unmarked tick", n, map[modelskill.ID]ind1Cast{ind3dSlowSkill: {me, 1000000}})

	w.clearDesires(t, n)
	w.moveCharacter(t, n, 50, 0)
	w.hit(t, n, 10)
	w.wantCasts(t, "hit from 50", n, map[modelskill.ID]ind1Cast{ind3dSlowSkill: {me, 1000000}})
}

// TestPartyLeaderSlowTypeAggressiveTurnsOnTheNearAttacker ports the
// retarget branch of PartyLeaderSlowTypeAggressive's attacked hook, every
// draw 0: fighting the character 150 away, a hit by a second character 50
// away drops all its hate, the character's included, and it hates the
// second one instead.
func TestPartyLeaderSlowTypeAggressiveTurnsOnTheNearAttacker(t *testing.T) {
	t.Parallel()
	w := bootIndividual3d(t, raidBoot{}, ind3dTemplate(ind3dSlowID, nil, 0, false))
	n := w.ind3dSpawn(t, ind3dSlowID, 1500, 0)
	w.moveCharacter(t, n, 150, 0)
	w.draws.set(0, 99) // no slow on the first hit
	w.hit(t, n, 100)
	w.ticks(t, 1)
	if got := n.AI().CurrentIntention(); got != actorai.IntentionAttack {
		t.Fatalf("setup: intention after the hit = %v, want attack", got)
	}
	mate := w.joinCharacter(t, "mate", "Mate")
	w.moveNear(t, mate, n, 50)
	w.hitBy(t, n, mate, 10)
	if got := n.AggroHate(w.player); got != 0 {
		t.Fatalf("hate of the far character after the near hit = %v, want 0", got)
	}
	if got := n.AggroHate(mate); got <= 0 {
		t.Fatalf("hate of the near attacker = %v, want some", got)
	}
}

// TestPartyLeaderSlowTypeAggressiveSlowsItsClansAttacker ports
// PartyLeaderSlowTypeAggressive's clan-attacked hook, every draw 0: a
// settled member that is not attacking wants its slow on the character
// that hit another member, weight 1000000.
func TestPartyLeaderSlowTypeAggressiveSlowsItsClansAttacker(t *testing.T) {
	t.Parallel()
	w := bootIndividual3d(t, raidBoot{}, ind3dTemplate(ind3dSlowID, nil, 0, true))
	victim := w.ind3dSpawn(t, ind3dSlowID, 1500, 0)
	member := w.ind3dSpawn(t, ind3dSlowID, 1500, 200)
	w.ticks(t, 8)
	w.hit(t, victim, 10)
	w.wantCasts(t, "member after the hit", member, map[modelskill.ID]ind1Cast{ind3dSlowSkill: {w.player.ObjectID(), 1000000}})
}

// TestQuestPartyLeaderReactsToNothing ports QuestPartyLeader's hooks. It
// still creates its declared private as it spawns, but with no desire it
// does not wander; a hit on itself or its private adds no hate and no
// attack desire; and its 120 s check, finding it out of its territory,
// leaves it where it stands.
func TestQuestPartyLeaderReactsToNothing(t *testing.T) {
	t.Parallel()
	w := bootIndividual3d(t, raidBoot{}, ind3dTemplate(ind3dQuestID, nil, 1, false), warriorTemplate(partyPrivateID, nil))
	n := w.ind3dSpawn(t, ind3dQuestID, 1500, 0)
	private := privatesOf(t, n, 1)[0]
	w.ticks(t, 3)
	if d, ok := desire(n, actorai.IntentionWander); ok {
		t.Fatalf("the tree queued a wander desire %+v", d)
	}
	w.hit(t, n, 10)
	w.hit(t, private, 10)
	if got := n.AggroHate(w.player); got != 0 {
		t.Fatalf("hate after the hits = %v, want 0", got)
	}
	if d, ok := desire(n, actorai.IntentionAttack); ok {
		t.Fatalf("the tree queued an attack desire %+v", d)
	}
	w.wantCasts(t, "after the hits", n, nil)

	w.teleport(t, n, 1000, 0)
	away := positionOf(n)
	w.srv.Advance(t, 120*time.Second)
	if got := positionOf(n); got != away {
		t.Fatalf("tree at %v after its 120 s check, want still at %v", got, away)
	}
}

// TestQuestPartyLeaderIgnoresItsClan ports QuestPartyLeader's
// clan-attacked hook: a settled tree of the clan, beside another tree the
// character hits, gains no hate and no desire.
func TestQuestPartyLeaderIgnoresItsClan(t *testing.T) {
	t.Parallel()
	const otherTreeID = int32(27186) // QuestPartyLeader
	w := bootIndividual3d(t, raidBoot{}, ind3dTemplate(ind3dQuestID, nil, 0, true), ind3dTemplate(otherTreeID, nil, 0, true))
	victim := w.ind3dSpawn(t, ind3dQuestID, 1500, 0)
	member := w.ind3dSpawn(t, otherTreeID, 1500, 200)
	w.ticks(t, 8)
	w.hit(t, victim, 10)
	if got := member.AggroHate(w.player); got != 0 {
		t.Fatalf("member hate after the hit = %v, want 0", got)
	}
	if d, ok := desire(member, actorai.IntentionAttack); ok {
		t.Fatalf("member queued an attack desire %+v", d)
	}
	w.wantCasts(t, "member after the hit", member, nil)
}

// TestRoyalRushPartyLeaderCallsItsSlaveOnce ports RoyalRushPartyLeader's
// created and attacked hooks, every draw 0. It spawns with i_ai0 clear. A
// first hit, with no creature to attack yet, does nothing of its own. The
// second makes it want its first physical special on the character, weight
// 1000000, and call its SummonSlave private at the character's feet with
// spawn parameters 1000, the character and 0; a third calls no more.
func TestRoyalRushPartyLeaderCallsItsSlaveOnce(t *testing.T) {
	t.Parallel()
	w := bootIndividual3d(t, raidBoot{}, ind3dTemplate(ind3dRoyalID, npc.AIParams{"SummonSlave": ind3dSummonSlaveParam}, 0, false), warriorTemplate(partyPrivateID, nil))
	n := w.ind3dSpawn(t, ind3dRoyalID, 300, 0)
	me := w.player.ObjectID()
	if got := ind1Slot(n, script.SlotIntAI0); got != 0 {
		t.Fatalf("created i_ai0 = %d, want 0", got)
	}
	w.hit(t, n, 10)
	w.wantCasts(t, "after the first hit", n, nil)
	privatesOf(t, n, 0)
	w.ticks(t, 1)

	w.hit(t, n, 10)
	w.wantCasts(t, "after the second hit", n, map[modelskill.ID]ind1Cast{ind3dSpecial1Skill: {me, 1000000}})
	slave := privatesOf(t, n, 1)[0]
	if p1, p2, p3 := ind1Slot(slave, script.SlotParam1), ind1Slot(slave, script.SlotParam2), ind1Slot(slave, script.SlotParam3); p1 != 1000 || p2 != me || p3 != 0 {
		t.Fatalf("slave spawn parameters = %d, %d, %d, want 1000, %d, 0", p1, p2, p3, me)
	}
	if at := positionOf(slave); at.X != w.at.X || at.Y != w.at.Y {
		t.Fatalf("slave at %v, want at the character's feet %v", at, w.at)
	}
	if got := ind1Slot(n, script.SlotIntAI0); got != 1 {
		t.Fatalf("i_ai0 after the call = %d, want 1", got)
	}
	w.hit(t, n, 10)
	privatesOf(t, n, 1)
}

// TestPartyLeaderCasting3SkillsMagicalCastsFromAfar ports
// PartyLeaderCasting3SkillsMagicalAggressive's see-creature, attacked and
// clan-attacked hooks, every draw 0, for a template listing its DD magics
// and debuff and for one listing none, which casts the defaults: DD magic
// 4001, both times, and debuff 4037. Settled and wandering, it wants both
// DD magics and the debuff on a character it sees 300 away, and attacks
// it. A hit by the creature it most wants to attack from 300 wants the
// same, and its own clan call as much again, as does a settled clan member
// that is not attacking.
func TestPartyLeaderCasting3SkillsMagicalCastsFromAfar(t *testing.T) {
	t.Parallel()
	for _, row := range []struct {
		name   string
		listed bool
	}{{"listed", true}, {"defaults", false}} {
		t.Run(row.name, func(t *testing.T) {
			t.Parallel()
			tmpl := ind3dTemplate(ind3dCasting3ID, nil, 0, true)
			two := ind3dTemplate(ind3dCasting3Two, nil, 0, true)
			want := map[modelskill.ID]ind1Cast{}
			if row.listed {
				want[ind3dDDMagic1Skill] = ind1Cast{weight: 1000000}
				want[ind3dDDMagic2Skill] = ind1Cast{weight: 1000000}
				want[ind3dDebuffSkill] = ind1Cast{weight: 1000000}
			} else {
				for _, tm := range []*npc.Template{tmpl, two} {
					tm.SkillsByType = nil
				}
				want[ind3dDefaultDDMagic] = ind1Cast{weight: 2000000}
				want[ind3dDefaultDebuff] = ind1Cast{weight: 1000000}
			}
			w := bootIndividual3d(t, raidBoot{}, tmpl, two)
			for id, c := range want {
				want[id] = ind1Cast{w.player.ObjectID(), c.weight}
			}

			n := w.ind3dSpawn(t, ind3dCasting3ID, 1500, 0)
			w.ticks(t, 9)
			if got := n.AI().CurrentIntention(); got != actorai.IntentionWander {
				t.Fatalf("setup: intention = %v, want wander", got)
			}
			w.moveCharacter(t, n, 300, 0)
			w.ticks(t, 1)
			w.wantCasts(t, "on sight", n, want)
			if got := n.AggroHate(w.player); got != 200 {
				t.Fatalf("hate of the character it sees = %v, want 200", got)
			}
			// The hit is also the NPC's own clan call, which, settled and
			// not attacking, wants as much again.
			w.clearDesires(t, n)
			w.hit(t, n, 10)
			doubled := map[modelskill.ID]ind1Cast{}
			for id, c := range want {
				doubled[id] = ind1Cast{c.target, 2 * c.weight}
			}
			w.wantCasts(t, "hit from 300", n, doubled)

			victim := w.ind3dSpawn(t, ind3dCasting3ID, -1500, 0)
			member := w.ind3dSpawn(t, ind3dCasting3Two, -1500, 200)
			w.ticks(t, 8)
			w.clearDesires(t, member)
			w.hit(t, victim, 10)
			w.wantCasts(t, "member after the hit", member, want)
		})
	}
}

// TestAntharasPrivateBehemothDragonLeadsBombers ports
// AntharasPrivateBehemothDragon's created and timer hooks. Called as a
// private with its first spawn parameter 4, 5 or 6, it calls 3, 5 or 7
// Dragon Bombers, and with any other none; a private's privates join its
// master's, so the bombers are its master's. Its created hook calls no
// parent, so its declared private never comes. A dead bomber arms no
// respawn. 240 minutes after it appears it leaves the world.
func TestAntharasPrivateBehemothDragonLeadsBombers(t *testing.T) {
	t.Parallel()
	for _, row := range []struct {
		param1  int32
		bombers int
	}{{0, 0}, {4, 3}, {5, 5}, {6, 7}} {
		t.Run(string(rune('0'+row.param1)), func(t *testing.T) {
			t.Parallel()
			w := bootIndividual3d(t, raidBoot{}, ind3dTemplate(ind3dBehemothID, nil, 1, false), warriorTemplate(ind3dBomberID, nil),
				warriorTemplate(partyPrivateID, nil), leaderTemplate(leaderID, nil, 0))
			master := w.spawn(t, leaderID, 1500, 0)
			behemoth := w.ind4cCallPrivate(t, master, ind3dBehemothID, 1500, 200, script.Params{P1: row.param1})
			var bombers []*npc.Hostile
			for _, p := range privatesOf(t, master, 1+row.bombers) {
				if p.NpcID() == int(ind3dBomberID) {
					bombers = append(bombers, p)
				}
			}
			if len(bombers) != row.bombers {
				t.Fatalf("bombers = %d, want %d", len(bombers), row.bombers)
			}
			if got := ind3Live(w, partyPrivateID); got != 0 {
				t.Fatalf("declared privates in the world = %d, want none", got)
			}
			if row.bombers > 0 {
				w.kill(t, bombers[0])
				w.noPrivateRespawn(t, "after a bomber died")
			}

			w.srv.Advance(t, 240*time.Minute-time.Second)
			if got := ind3Live(w, ind3dBehemothID); got != 1 {
				t.Fatalf("behemoths before 240 minutes = %d, want 1", got)
			}
			w.srv.Advance(t, 2*time.Second)
			if got := ind3Live(w, ind3dBehemothID); got != 0 || !behemoth.Decayed() {
				t.Fatalf("behemoths after 240 minutes = %d (decayed %v), want it gone", got, behemoth.Decayed())
			}
		})
	}
}
