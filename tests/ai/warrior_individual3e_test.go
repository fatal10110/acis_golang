package ai

import (
	"fmt"
	"testing"

	actorai "github.com/fatal10110/acis_golang/internal/gameserver/model/actor/ai"
	"github.com/fatal10110/acis_golang/internal/gameserver/model/actor/npc"
	modelskill "github.com/fatal10110/acis_golang/internal/gameserver/model/skill"
	"github.com/fatal10110/acis_golang/internal/gameserver/network/serverpackets"
	"github.com/fatal10110/acis_golang/internal/gameserver/script"
	"github.com/fatal10110/acis_golang/internal/gameserver/script/ai/warrior"
	"github.com/fatal10110/acis_golang/internal/gameservertest"
)

// The NPC ids the scenarios of the third individual behavior batch's
// fourth PR spawn (#188 PR 4), each a fixture monster of level 1.
const (
	ind3eCoupleID     = int32(29971) // PartyLeaderCoupleCaptain, bound by the test
	ind3eSpecial2ID   = int32(29972) // PartyLeaderPhysicalSpecial2, bound by the test
	ind3eSpecial2Two  = int32(29974) // PartyLeaderPhysicalSpecial2, bound by the test
	ind3eSilhouetteID = int32(29973) // the couple captain's silhouette, unbound
	ind3eMaleID       = int32(22109) // PartyLeaderCoupleCaptainMaleAggressive
	ind3eNurseID      = int32(22118) // PartyLeaderCoupleCaptainNurseAggressive
	ind3eHeretic2ID   = int32(22155) // PartyLeaderHeretic2Aggressive
	ind3eHeretic2Two  = int32(22159) // PartyLeaderHeretic2Aggressive
	ind3eHereticID    = int32(22188) // PartyLeaderHereticAggressive
	ind3eRoamerID     = int32(22200) // PrimevalRoamer
	ind3eWalkerID     = int32(22205) // PrimevalRoamerWalker
	ind3eCouplePath   = "test.PartyLeaderCoupleCaptain"
	ind3eSpecial2Path = "test.PartyLeaderPhysicalSpecial2"
)

// The skills the PR's fixture NPCs list by type.
const (
	ind3eSummonSkill   = modelskill.ID(9901)
	ind3eSpecialSkill  = modelskill.ID(9902)
	ind3eRangeSpecial  = modelskill.ID(9903)
	ind3eRangeDDMagic  = modelskill.ID(9904)
	ind3eDDMagicSkill  = modelskill.ID(9905)
	ind3eSpecial2Skill = modelskill.ID(9906)
	ind3eSpecialAttack = modelskill.ID(9907)
	ind3eSelfHeal      = modelskill.ID(9908)
	ind3eRangeBuff1    = modelskill.ID(9909)
	ind3eSpecial1Skill = modelskill.ID(9910)
)

// ind3eBatch are the PR's classes, in scripts.xml order, with the fixture
// bindings of the two that have no NPC of their own.
var ind3eBatch = []struct {
	path string
	ctor func() script.Script
}{
	{"script.ai.individual.Monster.WarriorBase.Warrior.PartyLeaderWarrior.PartyLeaderCoupleCaptain.PartyLeaderCoupleCaptain", warrior.PartyLeaderCoupleCaptain},
	{"script.ai.individual.Monster.WarriorBase.Warrior.PartyLeaderWarrior.PartyLeaderCoupleCaptain.PartyLeaderCoupleCaptainMaleAggressive", warrior.PartyLeaderCoupleCaptainMaleAggressive},
	{"script.ai.individual.Monster.WarriorBase.Warrior.PartyLeaderWarrior.PartyLeaderCoupleCaptain.PartyLeaderCoupleCaptainNurseAggressive", warrior.PartyLeaderCoupleCaptainNurseAggressive},
	{"script.ai.individual.Monster.WarriorBase.Warrior.PartyLeaderWarrior.PartyLeaderPhysicalSpecial2.PartyLeaderHeretic2Aggressive", warrior.PartyLeaderHeretic2Aggressive},
	{"script.ai.individual.Monster.WarriorBase.Warrior.PartyLeaderWarrior.PartyLeaderPhysicalSpecial2.PartyLeaderHereticAggressive", warrior.PartyLeaderHereticAggressive},
	{"script.ai.individual.Monster.WarriorBase.Warrior.PartyLeaderWarrior.PartyLeaderPhysicalSpecial2.PartyLeaderPhysicalSpecial2", warrior.PartyLeaderPhysicalSpecial2},
	{"script.ai.individual.Monster.WarriorBase.Warrior.PartyLeaderWarrior.PrimevalRoamer.PrimevalRoamer", warrior.PrimevalRoamer},
	{"script.ai.individual.Monster.WarriorBase.Warrior.PartyLeaderWarrior.PrimevalRoamer.PrimevalRoamerWalker", warrior.PrimevalRoamerWalker},
	{ind3eCouplePath, func() script.Script {
		s := warrior.PartyLeaderCoupleCaptain()
		s.NPCs = []int32{ind3eCoupleID}
		return s
	}},
	{ind3eSpecial2Path, func() script.Script {
		s := warrior.PartyLeaderPhysicalSpecial2()
		s.NPCs = []int32{ind3eSpecial2ID, ind3eSpecial2Two}
		return s
	}},
}

// bootIndividual3e boots the PR's classes after the party Warriors beside
// templates, every draw 0 until a scenario queues others, with the
// director script and a listener on ind1ListenerID recording the script
// events it hears, and extra's options added last.
func bootIndividual3e(t *testing.T, extra raidBoot, templates ...*npc.Template) *ind1World {
	t.Helper()
	list, cat := partyScripts()
	for _, c := range ind3eBatch {
		list = append(list, script.Listing{Path: c.path})
		cat[c.path] = c.ctor
	}
	heard := &ind1Heard{}
	const listener, director = "test.Ind3eListener", "test.Ind1Director"
	cat[listener] = func() script.Script {
		return script.Script{Behavior: true, NPCs: []int32{ind1ListenerID}, Hooks: script.Hooks{
			OnScriptEvent: func(_ *script.Script, e script.ScriptEvent) {
				heard.mu.Lock()
				defer heard.mu.Unlock()
				heard.events = append(heard.events, fmt.Sprintf("event=%d arg1=%d arg2=%d", e.EventID, e.Arg1, e.Arg2))
			},
		}}
	}
	cat[director] = func() script.Script { return script.Script{} }
	list = append(list, script.Listing{Path: listener}, script.Listing{Path: director})
	kinds := map[int32]script.NPCKind{}
	for _, tmpl := range append(templates, warriorTemplate(ind1ListenerID, nil)) {
		kinds[int32(tmpl.ID)] = script.KindHostile
	}
	draws := &ind1Draws{}
	opts := append([]gameservertest.Option{
		gameservertest.WithScriptRand(draws.rand),
		gameservertest.WithNPCScripts(kinds, list, cat),
	}, extra.opts...)
	w := bootRaid(t, raidBoot{opts: opts, ready: extra.ready}, append(templates, warriorTemplate(ind1ListenerID, nil))...)
	return &ind1World{warriorWorld: w, draws: draws, heard: heard}
}

// ind3eTemplate is a fixture PR NPC of id with ai as its AI parameters,
// standing where it spawns and listing each of the PR's skills by type,
// declaring privates of the fixture private respawning after 240 s; with
// clan set, in the clan the scenario's NPCs share.
func ind3eTemplate(id int32, ai npc.AIParams, privates int, clan bool) *npc.Template {
	tmpl := leaderTemplate(id, ai, privates)
	tmpl.CanMove = false
	tmpl.SkillsByType = map[npc.SkillType]modelskill.Ref{
		npc.SkillTypeSummonMagic:              {ID: ind3eSummonSkill, Level: 1},
		npc.SkillTypePhysicalSpecial:          {ID: ind3eSpecialSkill, Level: 1},
		npc.SkillTypeSelfRangePhysicalSpecial: {ID: ind3eRangeSpecial, Level: 1},
		npc.SkillTypeSelfRangeDDMagic:         {ID: ind3eRangeDDMagic, Level: 1},
		npc.SkillTypeDDMagic:                  {ID: ind3eDDMagicSkill, Level: 1},
		npc.SkillTypePhysicalSpecial2:         {ID: ind3eSpecial2Skill, Level: 1},
		npc.SkillTypeSpecialAttack:            {ID: ind3eSpecialAttack, Level: 1},
		npc.SkillTypeSelfMagicHeal:            {ID: ind3eSelfHeal, Level: 1},
		npc.SkillTypeSelfRangeBuff1:           {ID: ind3eRangeBuff1, Level: 1},
		npc.SkillTypePhysicalSpecial1:         {ID: ind3eSpecial1Skill, Level: 1},
	}
	if clan {
		tmpl.Clans, tmpl.ClanRange = []string{"party"}, 500
	}
	return tmpl
}

// ind3eSkills are the PR NPCs' skills: the strikes and magic reach 2000,
// the heal and the buff are self buffs.
func ind3eSkills() *modelskill.Table {
	defs := []modelskill.Definition{ind1Skill(ind3eSelfHeal, 0, true), ind1Skill(ind3eRangeBuff1, 0, true)}
	for _, id := range []modelskill.ID{
		ind3eSummonSkill, ind3eSpecialSkill, ind3eRangeSpecial, ind3eRangeDDMagic, ind3eDDMagicSkill,
		ind3eSpecial2Skill, ind3eSpecialAttack, ind3eSpecial1Skill,
	} {
		defs = append(defs, ind1Skill(id, 2000, false))
	}
	return modelskill.NewTable(defs)
}

// ind3eSpawn spawns an NPC of id dx, dy from the character with the PR's
// skills to cast.
func (w *ind1World) ind3eSpawn(t *testing.T, id int32, dx, dy int) *npc.Hostile {
	t.Helper()
	n := w.spawn(t, id, dx, dy)
	w.srv.InstallNPCCastSeam(t, n, ind3eSkills())
	return n
}

// TestPartyLeaderCoupleCaptainPullsItsFarAttacker ports
// PartyLeaderCoupleCaptain's attacked and skill-finished hooks. Every hit
// by the character stores it in c_ai0. A hit from over 300 away on a first
// draw under 20, or from over 100 away on one under 50, makes it want its
// summon magic on the character, weight 1000000; the other rows want
// none. Once that magic is cast, the character jumps onto the captain,
// seeing one fast TeleportToLocation, and the captain wants to attack it
// with weight 100000 more.
func TestPartyLeaderCoupleCaptainPullsItsFarAttacker(t *testing.T) {
	t.Parallel()
	w := bootIndividual3e(t, raidBoot{}, ind3eTemplate(ind3eCoupleID, nil, 0, false))
	n := w.ind3eSpawn(t, ind3eCoupleID, 150, 0)
	me := w.player.ObjectID()
	for _, row := range []struct {
		dx, draw int
		summon   bool
	}{
		{150, 49, true}, {150, 50, false}, {350, 19, true}, {350, 49, true}, {350, 50, false}, {80, 0, false},
	} {
		when := fmt.Sprintf("a hit from %d, draw %d", row.dx, row.draw)
		w.moveCharacter(t, n, -row.dx, 0)
		w.clearDesires(t, n)
		w.draws.set(0, row.draw)
		w.hit(t, n, 1)
		var want map[modelskill.ID]ind1Cast
		if row.summon {
			want = map[modelskill.ID]ind1Cast{ind3eSummonSkill: {me, 1000000}}
		}
		w.wantCasts(t, when, n, want)
		if got := slotID(n, script.SlotCreature0); got != me {
			t.Fatalf("%s: c_ai0 = %d, want the character %d", when, got, me)
		}
	}

	w.moveCharacter(t, n, -350, 0)
	w.clearDesires(t, n)
	w.draws.set(0)
	w.hit(t, n, 1)
	var frames [][]byte
	w.srv.AdvanceUntil(t, "the character jumps onto the captain", func() bool {
		frames = append(frames, w.srv.ReadQueued(t, w.srv.Client)...)
		x, _, _ := w.srv.PlayerPosition(t, me)
		nx, _, _ := n.Position()
		return x == nx
	})
	frames = append(frames, w.srv.ReadQueued(t, w.srv.Client)...)
	if got := castsOf(frames, n, ind3eSummonSkill); len(got) != 1 || got[0] != me {
		t.Fatalf("summon magic MagicSkillUse targets = %v, want one on %d", got, me)
	}
	jumps := framesOf(frames, serverpackets.OpcodeTeleportToLocation, me)
	if len(jumps) != 1 {
		t.Fatalf("TeleportToLocation frames of the character = %d, want 1", len(jumps))
	}
	nx, ny, _ := n.Position()
	if at, fast := teleportFrame(jumps[0]); at.X != nx || at.Y != ny || !fast {
		t.Fatalf("TeleportToLocation = %v fast=%v, want onto the captain at %d,%d, fast", at, fast, nx, ny)
	}
	if d, ok := desire(n, actorai.IntentionAttack); !ok || d.Weight < 100000 || d.FinalTarget == nil || d.FinalTarget.ObjectID() != me {
		t.Fatalf("attack desire after the pull = %+v (%v), want the character with weight 100000 or more", d, ok)
	}
}

// TestPartyLeaderCoupleCaptainGivesWayToItsSilhouette ports
// PartyLeaderCoupleCaptain's party-died hook, which calls no parent: a
// private's death never arms its respawn. A captain with no top desire
// target, or at 70% of its HP, does nothing more. A captain above 70%
// with the character as its top desire target calls the silhouette its
// AI parameters name, as its private, where it stands, with spawn
// parameters 1000 and the character, which the client sees appear, and
// leaves the world.
func TestPartyLeaderCoupleCaptainGivesWayToItsSilhouette(t *testing.T) {
	t.Parallel()
	tmpl := ind3eTemplate(ind3eCoupleID, npc.AIParams{"silhouette": fmt.Sprint(ind3eSilhouetteID)}, 1, false)
	w := bootIndividual3e(t, raidBoot{}, tmpl, warriorTemplate(partyPrivateID, nil), warriorTemplate(ind3eSilhouetteID, nil))
	me := w.player.ObjectID()
	for _, row := range []struct {
		name  string
		top   bool
		share float64
	}{{"no top desire target", false, 1}, {"at 70% of its HP", true, 0.7}} {
		n := w.ind3eSpawn(t, ind3eCoupleID, 300, 0)
		if row.top {
			w.setTop(t, n, w.charOf(t))
		}
		w.setHPShare(t, n, row.share)
		w.kill(t, privatesOf(t, n, 1)[0])
		if !w.inWorld(n) {
			t.Fatalf("%s: the captain left the world", row.name)
		}
		if got := len(w.liveNPCs(ind3eSilhouetteID)); got != 0 {
			t.Fatalf("%s: silhouettes = %d, want none", row.name, got)
		}
		w.noPrivateRespawn(t, row.name)
		w.die(t, n)
	}

	n := w.ind3eSpawn(t, ind3eCoupleID, 300, 0)
	x, y, _ := n.Position()
	w.setTop(t, n, w.charOf(t))
	private := privatesOf(t, n, 1)[0]
	w.onQueue(t, private, func() { script.NPCOf(private).DoDie(nil) })
	frames := w.srv.ReadQueued(t, w.srv.Client)
	if w.inWorld(n) {
		t.Fatal("the captain is still in the world, want it gone")
	}
	if got := framesOf(frames, serverpackets.OpcodeDeleteObject, n.ObjectID()); len(got) != 1 {
		t.Fatalf("DeleteObject frames of the captain = %d, want 1", len(got))
	}
	silhouettes := w.liveNPCs(ind3eSilhouetteID)
	if len(silhouettes) != 1 {
		t.Fatalf("silhouettes = %d, want 1", len(silhouettes))
	}
	s := silhouettes[0]
	if sx, sy, _ := s.Position(); sx != x || sy != y {
		t.Fatalf("silhouette at %d,%d, want where the captain stood, %d,%d", sx, sy, x, y)
	}
	if p1, p2, p3 := ind1Slot(s, script.SlotParam1), ind1Slot(s, script.SlotParam2), ind1Slot(s, script.SlotParam3); p1 != 1000 || p2 != me || p3 != 0 {
		t.Fatalf("silhouette spawn parameters = %d, %d, %d, want 1000, %d, 0", p1, p2, p3, me)
	}
	if got := framesOf(frames, serverpackets.OpcodeNPCInfo, s.ObjectID()); len(got) != 1 {
		t.Fatalf("NpcInfo frames of the silhouette = %d, want 1", len(got))
	}
	w.noPrivateRespawn(t, "after the silhouette came")
}

// TestPartyLeaderCoupleCaptainMaleAggressiveStrikes ports
// PartyLeaderCoupleCaptainMaleAggressive's created, see-creature and
// attacked hooks, every draw 0 unless a row says otherwise. It spawns with
// i_ai0 clear and attacks the character it sees with hate 200. A hit by
// the character it most wants to attack wants its physical special on it
// and its self-centred one, each weight 1000000; a first draw of 33 skips
// the first.
func TestPartyLeaderCoupleCaptainMaleAggressiveStrikes(t *testing.T) {
	t.Parallel()
	tmpl := ind3eTemplate(ind3eMaleID, nil, 0, false)
	delete(tmpl.SkillsByType, npc.SkillTypeSummonMagic)
	w := bootIndividual3e(t, raidBoot{}, tmpl)
	n := w.ind3eSpawn(t, ind3eMaleID, 1500, 0)
	if got := ind1Slot(n, script.SlotIntAI0); got != 0 {
		t.Fatalf("created i_ai0 = %d, want 0", got)
	}
	if got := n.AggroHate(w.player); got != 0 {
		t.Fatalf("hate before it saw the character = %v, want 0", got)
	}
	w.moveCharacter(t, n, -150, 0)
	w.ticks(t, 1)
	if got := n.AggroHate(w.player); got != 200 {
		t.Fatalf("hate of the character it sees = %v, want 200", got)
	}
	me := w.player.ObjectID()
	w.clearDesires(t, n)
	w.setTop(t, n, w.charOf(t))
	w.hit(t, n, 1)
	w.wantCasts(t, "a hit by its top desire target", n, map[modelskill.ID]ind1Cast{ind3eSpecialSkill: {me, 1000000}, ind3eRangeSpecial: {me, 1000000}})
	w.setTop(t, n, w.charOf(t))
	w.draws.set(0, 33)
	w.hit(t, n, 1)
	w.wantCasts(t, "a hit by its top desire target, first draw 33", n, map[modelskill.ID]ind1Cast{ind3eRangeSpecial: {me, 1000000}})
}

// TestPartyLeaderCoupleCaptainMaleAggressiveAnswersItsPartyAndClan ports
// PartyLeaderCoupleCaptainMaleAggressive's party-attacked and
// clan-attacked hooks, every draw 0. A hit on its private before it has
// lived 7 AI cycles wants nothing, nor runs its parent, which would hate
// the private's attacker; once settled, the same hit wants its physical
// special and its self-centred one on the attacker, and the parent's hate
// follows. A settled clan member's attacker gets its physical
// special, while it does not attack already.
func TestPartyLeaderCoupleCaptainMaleAggressiveAnswersItsPartyAndClan(t *testing.T) {
	t.Parallel()
	tmpl := ind3eTemplate(ind3eMaleID, nil, 1, true)
	delete(tmpl.SkillsByType, npc.SkillTypeSummonMagic)
	victimTmpl := warriorTemplate(ind3eSpecial2ID, nil)
	victimTmpl.Clans, victimTmpl.ClanRange, victimTmpl.CanMove = []string{"party"}, 500, false
	w := bootIndividual3e(t, raidBoot{}, tmpl, warriorTemplate(partyPrivateID, nil), victimTmpl)
	me := w.player.ObjectID()

	n := w.ind3eSpawn(t, ind3eMaleID, 1500, 0)
	private := privatesOf(t, n, 1)[0]
	w.hit(t, private, 100)
	w.wantCasts(t, "a hit on its private before it settled", n, nil)
	if got := n.AggroHate(w.player); got != 0 {
		t.Fatalf("hate before it settled = %v, want none: no parent call", got)
	}
	w.ticks(t, 8)
	w.clearDesires(t, n)
	w.hit(t, private, 100)
	w.wantCasts(t, "a hit on its private once settled", n, map[modelskill.ID]ind1Cast{ind3eSpecialSkill: {me, 1000000}, ind3eRangeSpecial: {me, 1000000}})
	// The parent's party hate: 100 / (level + 7) x 100 x the private's
	// weight point 1 x 10.
	if got := n.AggroHate(w.player); got != 12500 {
		t.Fatalf("hate once settled = %v, want the parent's 12500", got)
	}

	member := w.ind3eSpawn(t, ind3eMaleID, 1500, 2000)
	victim := w.ind3eSpawn(t, ind3eSpecial2ID, 1600, 2000)
	w.ticks(t, 8)
	w.clearDesires(t, member)
	w.hit(t, victim, 1)
	w.wantCasts(t, "a settled member after a hit on its clan", member, map[modelskill.ID]ind1Cast{ind3eSpecialSkill: {me, 1000000}})
}

// TestPartyLeaderCoupleCaptainNurseAggressiveCastsByRange ports
// PartyLeaderCoupleCaptainNurseAggressive's see-creature, attacked and
// clan-attacked hooks, every draw 0. It attacks the character it sees with
// hate 200. Hit by its top desire target from 150 it wants its
// self-centred DD magic on itself and its DD magic on the character, each
// weight 1000000; from 250 only the DD magic, from 50 only the
// self-centred one. A settled clan member whose top desire target hits
// another member wants the same by its own distance.
func TestPartyLeaderCoupleCaptainNurseAggressiveCastsByRange(t *testing.T) {
	t.Parallel()
	tmpl := ind3eTemplate(ind3eNurseID, nil, 0, true)
	delete(tmpl.SkillsByType, npc.SkillTypeSummonMagic)
	w := bootIndividual3e(t, raidBoot{}, tmpl)
	me := w.player.ObjectID()
	n := w.ind3eSpawn(t, ind3eNurseID, 1500, 0)
	w.moveCharacter(t, n, -150, 0)
	w.ticks(t, 1)
	if got := n.AggroHate(w.player); got != 200 {
		t.Fatalf("hate of the character it sees = %v, want 200", got)
	}
	for _, row := range []struct {
		dx   int
		want map[modelskill.ID]ind1Cast
	}{
		{150, map[modelskill.ID]ind1Cast{ind3eRangeDDMagic: {n.ObjectID(), 1000000}, ind3eDDMagicSkill: {me, 1000000}}},
		{250, map[modelskill.ID]ind1Cast{ind3eDDMagicSkill: {me, 1000000}}},
		{50, map[modelskill.ID]ind1Cast{ind3eRangeDDMagic: {n.ObjectID(), 1000000}}},
	} {
		w.moveCharacter(t, n, -row.dx, 0)
		w.setTop(t, n, w.charOf(t))
		w.hit(t, n, 1)
		w.wantCasts(t, fmt.Sprintf("a hit from %d", row.dx), n, row.want)
	}

	victim := w.ind3eSpawn(t, ind3eNurseID, 3000, 3000)
	member := w.ind3eSpawn(t, ind3eNurseID, 3150, 3000)
	w.ticks(t, 8)
	w.moveCharacter(t, member, -150, 0)
	w.setTop(t, member, w.charOf(t))
	w.setTop(t, victim, nil)
	w.hit(t, victim, 1)
	w.wantCasts(t, "a settled member after a hit on its clan", member, map[modelskill.ID]ind1Cast{ind3eRangeDDMagic: {member.ObjectID(), 1000000}, ind3eDDMagicSkill: {me, 1000000}})
}

// TestPartyLeaderPhysicalSpecial2StrikesByRange ports
// PartyLeaderPhysicalSpecial2's created, attacked and clan-attacked hooks,
// every draw 0. It spawns with i_ai0 clear. Hit by its top desire target
// from 150 it wants its second physical special on the character, from
// 30 its first, each weight 1000000; a hit by anyone else wants neither.
// A settled clan member whose top desire target hits another member wants
// the second one on it from 150.
func TestPartyLeaderPhysicalSpecial2StrikesByRange(t *testing.T) {
	t.Parallel()
	w := bootIndividual3e(t, raidBoot{}, ind3eTemplate(ind3eSpecial2ID, nil, 0, true), ind3eTemplate(ind3eSpecial2Two, nil, 0, true))
	me := w.player.ObjectID()
	n := w.ind3eSpawn(t, ind3eSpecial2ID, 150, 0)
	if got := ind1Slot(n, script.SlotIntAI0); got != 0 {
		t.Fatalf("created i_ai0 = %d, want 0", got)
	}
	w.setTop(t, n, nil)
	w.hit(t, n, 1)
	w.wantCasts(t, "a first hit", n, nil)
	for _, row := range []struct {
		dx   int
		want modelskill.ID
	}{{150, ind3eSpecial2Skill}, {40, ind3eSpecial2Skill}, {30, ind3eSpecialSkill}} {
		w.moveCharacter(t, n, -row.dx, 0)
		w.setTop(t, n, w.charOf(t))
		w.hit(t, n, 1)
		w.wantCasts(t, fmt.Sprintf("a hit from %d", row.dx), n, map[modelskill.ID]ind1Cast{row.want: {me, 1000000}})
	}

	victim := w.ind3eSpawn(t, ind3eSpecial2ID, 3000, 3000)
	member := w.ind3eSpawn(t, ind3eSpecial2Two, 3150, 3000)
	w.ticks(t, 8)
	w.moveCharacter(t, member, -150, 0)
	w.setTop(t, member, w.charOf(t))
	w.setTop(t, victim, nil)
	w.hit(t, victim, 1)
	w.wantCasts(t, "a settled member after a hit on its clan", member, map[modelskill.ID]ind1Cast{ind3eSpecial2Skill: {me, 1000000}})
}
