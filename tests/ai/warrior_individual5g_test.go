package ai

import (
	"testing"
	"time"

	actorai "github.com/fatal10110/acis_golang/internal/gameserver/model/actor/ai"
	"github.com/fatal10110/acis_golang/internal/gameserver/model/actor/npc"
	modelskill "github.com/fatal10110/acis_golang/internal/gameserver/model/skill"
	"github.com/fatal10110/acis_golang/internal/gameserver/network/serverpackets"
	"github.com/fatal10110/acis_golang/internal/gameserver/script"
	"github.com/fatal10110/acis_golang/internal/gameserver/script/ai/warrior"
	"github.com/fatal10110/acis_golang/internal/gameservertest"
)

// The NPC ids of the fifth individual behavior batch's seventh PR the
// tests spawn.
const (
	ind5gSleepID     = int32(21664) // WarriorCastingHealSleep
	ind5gSleepClanID = int32(21687) // WarriorCastingHealSleep, in a clan
	ind5gGhostID     = int32(21556) // WarriorCorpseGhostDDMagic
	ind5gGhostClanID = int32(21579) // WarriorCorpseGhostDDMagic, in a clan
	ind5gGhostAggrID = int32(21562) // WarriorCorpseGhostDDMagicAggressive
	ind5gZombieID    = int32(21554) // WarriorCorpseZombie
	ind5gZombieTwoID = int32(21548) // WarriorCorpseZombie
	ind5gDDID        = int32(21553) // WarriorCorpseZombieDDMagic
	ind5gDDClanID    = int32(21597) // WarriorCorpseZombieDDMagic, in a clan
	ind5gDDAggrID    = int32(21409) // WarriorCorpseZombieDDMagicAggressive
	ind5gDDAggrTwoID = int32(21570) // WarriorCorpseZombieDDMagicAggressive
	ind5gPSAggrID    = int32(21550) // WarriorCorpseZombiePhysicalSpecialAggressive
	ind5gPSID        = int32(21578) // WarriorCorpseZombiePhysicalSpecial
	ind5gPSClanID    = int32(21549) // WarriorCorpseZombiePhysicalSpecial, in a clan
	ind5gLeaderID    = int32(29973) // PartyLeaderWarrior (fixture binding), leading a corpse zombie
)

// The skills the batch's fixture NPCs list by type, and the zombies' jump.
const (
	ind5gSleep    = modelskill.ID(9560) // MAGIC_SLEEP
	ind5gSelfBuff = modelskill.ID(9561) // SELF_BUFF
	ind5gDDMagic  = modelskill.ID(9562) // DD_MAGIC
	ind5gDebuff   = modelskill.ID(9563) // SELF_RANGE_DEBUFF
	ind5gStrike   = modelskill.ID(9564) // PHYSICAL_SPECIAL
	ind5gJump     = modelskill.ID(4671)
)

// ind5gBatch are the classes of the PR, in scripts.xml order.
var ind5gBatch = []struct {
	path string
	ctor func() script.Script
}{
	{"script.ai.individual.Monster.WarriorBase.Warrior.WarriorCastingHeal.WarriorCastingHealSleep", warrior.CastingHealSleep},
	{"script.ai.individual.Monster.WarriorBase.Warrior.WarriorCorpseGhostDDMagic.WarriorCorpseGhostDDMagic", warrior.CorpseGhostDDMagic},
	{"script.ai.individual.Monster.WarriorBase.Warrior.WarriorCorpseGhostDDMagic.WarriorCorpseGhostDDMagicAggressive", warrior.CorpseGhostDDMagicAggressive},
	{"script.ai.individual.Monster.WarriorBase.Warrior.WarriorCorpseZombie.WarriorCorpseZombie", warrior.CorpseZombie},
	{"script.ai.individual.Monster.WarriorBase.Warrior.WarriorCorpseZombie.WarriorCorpseZombieDDMagic", warrior.CorpseZombieDDMagic},
	{"script.ai.individual.Monster.WarriorBase.Warrior.WarriorCorpseZombie.WarriorCorpseZombieDDMagicAggressive", warrior.CorpseZombieDDMagicAggressive},
	{"script.ai.individual.Monster.WarriorBase.Warrior.WarriorCorpseZombie.WarriorCorpseZombiePhysicalSpecialAggressive", warrior.CorpseZombiePhysicalSpecialAggressive},
	{"script.ai.individual.Monster.WarriorBase.Warrior.WarriorCorpseZombie.WarriorCorpseZombiePhysicalSpecial", warrior.CorpseZombiePhysicalSpecial},
}

// ind5gDefs are the batch's skills: the sleep, damage magic, strike and
// jump one-target attacks of reach 2000, and the self buff and the
// self-centred debuff, the buff with an effect of its own stack.
func ind5gDefs() []modelskill.Definition {
	return []modelskill.Definition{
		ind1Skill(ind5gSleep, 2000, false), ind1Skill(ind5gDDMagic, 2000, false),
		ind1Skill(ind5gStrike, 2000, false), ind1Skill(ind5gJump, 2000, false),
		ind5dStacked(ind1Skill(ind5gSelfBuff, 0, true), "ind5g_self", 1),
		ind1Skill(ind5gDebuff, 0, true),
	}
}

// ind5gTemplate is a fixture monster of id with ai listing the batch's
// skills by type.
func ind5gTemplate(id int32, ai npc.AIParams) *npc.Template {
	return ind5dTemplate(id, ai, map[npc.SkillType]modelskill.ID{
		npc.SkillTypeMagicSleep: ind5gSleep, npc.SkillTypeSelfBuff: ind5gSelfBuff, npc.SkillTypeDDMagic: ind5gDDMagic,
		npc.SkillTypeSelfRangeDebuff: ind5gDebuff, npc.SkillTypePhysicalSpecial: ind5gStrike,
	})
}

// ind5gStill is ind5gTemplate for an NPC that does not move.
func ind5gStill(id int32, ai npc.AIParams) *npc.Template {
	tmpl := ind5gTemplate(id, ai)
	tmpl.CanMove = false
	return tmpl
}

// ind5gTeleport are the AI parameters of a zombie set to jump.
var ind5gTeleport = npc.AIParams{"IsTeleport": "1"}

// bootIndividual5g boots the PR's classes after the Warrior chain beside
// templates and an unbound decoy monster, with a director script to land
// effects through, every NPC casting the batch's skills, every draw 0
// until a scenario queues others.
func bootIndividual5g(t *testing.T, templates ...*npc.Template) *ind1World {
	t.Helper()
	list, catalog := warriorScripts()
	for _, c := range ind5gBatch {
		list = append(list, script.Listing{Path: c.path})
		catalog[c.path] = c.ctor
	}
	const leader = "test.Ind5gLeader"
	catalog[leader] = boundTo(warrior.PartyLeader, ind5gLeaderID)
	const director = "test.Ind1Director"
	catalog[director] = func() script.Script { return script.Script{} }
	list = append(list, script.Listing{Path: leader}, script.Listing{Path: director})
	templates = append(templates, warriorTemplate(unboundID, nil))
	kinds := map[int32]script.NPCKind{}
	for _, tmpl := range templates {
		kinds[int32(tmpl.ID)] = script.KindHostile
	}
	draws := &ind1Draws{}
	opts := []gameservertest.Option{
		gameservertest.WithScriptRand(draws.rand),
		gameservertest.WithNPCScripts(kinds, list, catalog),
		gameservertest.WithNpcSpawnSkills(modelskill.NewTable(ind5gDefs())),
		gameservertest.WithScriptSkills(ind5gDefs()...),
	}
	w := bootRaid(t, raidBoot{opts: opts}, templates...)
	return &ind1World{warriorWorld: w, draws: draws, heard: &ind1Heard{}}
}

// ind5gAttackWeight returns h's attack desire weight on the creature of
// object id target, 0 for none.
func ind5gAttackWeight(h *npc.Hostile, target int32) float64 {
	for _, d := range h.AI().Desires().Snapshot() {
		if d.Kind == actorai.IntentionAttack && d.FinalTarget != nil && d.FinalTarget.ObjectID() == target {
			return d.Weight
		}
	}
	return 0
}

// ind5gWantJumped fails unless h stands at the character's x, y, the
// character saw one fast TeleportToLocation of h there among frames, and
// h wants its jump skill on the character.
func (w *ind1World) ind5gWantJumped(t *testing.T, when string, h *npc.Hostile, frames [][]byte) {
	t.Helper()
	me := w.player.ObjectID()
	x, y, _ := w.srv.PlayerPosition(t, me)
	if nx, ny, _ := h.Position(); nx != x || ny != y {
		t.Fatalf("%s: NPC at %d,%d, want on the character at %d,%d", when, nx, ny, x, y)
	}
	jumps := framesOf(frames, serverpackets.OpcodeTeleportToLocation, h.ObjectID())
	if len(jumps) != 1 {
		t.Fatalf("%s: TeleportToLocation frames of the NPC = %d, want 1", when, len(jumps))
	}
	if at, fast := teleportFrame(jumps[0]); at.X != x || at.Y != y || !fast {
		t.Fatalf("%s: TeleportToLocation = %v fast=%v, want fast onto %d,%d", when, at, fast, x, y)
	}
	ind5bWantCast(t, when, h, ind5gJump, me)
}

// ind5gSeeFromAfar spawns id 800 from the character, out of its sight,
// lets it settle past a life time of 7 and brings the character 200 from
// it, returning it and the frames of the AI cycle that saw the character.
func (w *ind1World) ind5gSeeFromAfar(t *testing.T, id int32) (*npc.Hostile, [][]byte) {
	t.Helper()
	n := w.spawn(t, id, 800, 0)
	w.ticks(t, 8)
	if n.LifeTime() <= 7 || n.AI().CurrentIntention() == actorai.IntentionAttack {
		t.Fatalf("setup: life time %d, intention %v, want settled and not attacking", n.LifeTime(), n.AI().CurrentIntention())
	}
	w.srv.ReadQueued(t, w.srv.Client)
	w.moveCharacter(t, n, -200, 0)
	frames := w.ticks(t, 1)
	return n, append(frames, w.srv.ReadQueued(t, w.srv.Client)...)
}

// TestCastingHealSleepSleepsWhoItDoesNotHateMost ports
// WarriorCastingHealSleep's attacked and clan-attacked hooks. A hit by the
// character, which it does not hate most yet, wants its sleep magic on
// the character on a draw under 33, weight 1000000; once the character is
// its most hated, a hit wants none. Hating a decoy most, a hit wants it on
// a draw of 32, not on 33. Told of the character's hit on a clan member,
// a healer hating the character most wants its sleep on it; one hating
// nobody does not.
func TestCastingHealSleepSleepsWhoItDoesNotHateMost(t *testing.T) {
	t.Parallel()
	w := bootIndividual5g(t, ind5gStill(ind5gSleepID, nil), ind5dClan(ind5gStill(ind5gSleepClanID, nil)), ind5dClan(warriorTemplate(warriorID, nil)))
	me := w.player.ObjectID()
	n := w.spawn(t, ind5gSleepID, 150, 0)
	w.hit(t, n, 1)
	ind5bWantCast(t, "the first hit", n, ind5gSleep, me)
	w.ind5dClear(t, n)
	w.hit(t, n, 1)
	ind5bNoCast(t, "a hit by its most hated", n, ind5gSleep)

	w.ind5dDecoy(t, n)
	w.draws.set(0, 33)
	w.hit(t, n, 1)
	ind5bNoCast(t, "a hit hating a decoy most, draw 33", n, ind5gSleep)
	w.draws.set(0, 32)
	w.hit(t, n, 1)
	ind5bWantCast(t, "a hit hating a decoy most, draw 32", n, ind5gSleep, me)

	victim := w.spawn(t, warriorID, 100, 300)
	hating := w.spawn(t, ind5gSleepClanID, 0, 300)
	calm := w.spawn(t, ind5gSleepClanID, 0, 400)
	w.onQueue(t, hating, func() { hating.AddDamageHate(w.player, 0, 1000) })
	w.draws.set(0)
	w.hit(t, victim, 1)
	ind5bWantCast(t, "a clan member's attacker it hates most", hating, ind5gSleep, me)
	ind5bNoCast(t, "a clan member's attacker it does not hate", calm, ind5gSleep)
}

// TestCorpseGhostDDMagicBuffsAndCastsFromAfar ports
// WarriorCorpseGhostDDMagic's created, timer, attacked and clan-attacked
// hooks, every draw 0. It wants its self buff on itself as it spawns, and
// at its 10 s timer while it lacks the buff. The hit that makes the
// character its most hated wants no damage magic; the next, from 300,
// wants it on the character, weight 1000000; one from 80 does not. A
// settled clan member wants it on the character 300 away at a hit on its
// clan, not when every draw is 33.
func TestCorpseGhostDDMagicBuffsAndCastsFromAfar(t *testing.T) {
	t.Parallel()
	w := bootIndividual5g(t, ind5gStill(ind5gGhostID, nil), ind5dClan(ind5gStill(ind5gGhostClanID, nil)))
	me := w.player.ObjectID()
	n := w.spawn(t, ind5gGhostID, 300, 0)
	ind5bWantCast(t, "after it spawned", n, ind5gSelfBuff, n.ObjectID())
	w.ind5dClear(t, n)
	w.srv.Advance(t, 10*time.Second)
	ind5bWantCast(t, "at its first timer", n, ind5gSelfBuff, n.ObjectID())

	w.ind5dClear(t, n)
	w.hit(t, n, 1)
	ind5bNoCast(t, "the hit that made the character its most hated", n, ind5gDDMagic)
	w.hit(t, n, 1)
	ind5bWantCast(t, "a hit from 300 by its most hated", n, ind5gDDMagic, me)
	w.ind5dClear(t, n)
	w.moveCharacter(t, n, -80, 0)
	w.hit(t, n, 1)
	ind5bNoCast(t, "a hit from 80", n, ind5gDDMagic)

	victim := w.spawn(t, ind5gGhostClanID, 0, 1000)
	member := w.spawn(t, ind5gGhostClanID, 300, 1000)
	w.moveCharacter(t, victim, -300, 0)
	w.ticks(t, 8)
	if member.LifeTime() <= 7 {
		t.Fatalf("clan member life time = %d, want past 7", member.LifeTime())
	}
	w.ind5dClear(t, member)
	w.draws.set(33)
	w.hit(t, victim, 1)
	ind5bNoCast(t, "a settled clan member, every draw 33", member, ind5gDDMagic)
	w.draws.set(0)
	w.hit(t, victim, 1)
	ind5bWantCast(t, "a settled clan member", member, ind5gDDMagic, me)
}

// TestCorpseGhostDDMagicAggressiveCastsAtWhatItSeesFromAfar ports
// WarriorCorpseGhostDDMagicAggressive's see-creature hook, every draw 0:
// settled and attacking nobody, it sees the character 200 away and both
// wants its damage magic on it, weight 1000000, and attacks it.
func TestCorpseGhostDDMagicAggressiveCastsAtWhatItSeesFromAfar(t *testing.T) {
	t.Parallel()
	w := bootIndividual5g(t, ind5gTemplate(ind5gGhostAggrID, nil))
	n, _ := w.ind5gSeeFromAfar(t, ind5gGhostAggrID)
	ind5bWantCast(t, "seeing the character 200 away", n, ind5gDDMagic, w.player.ObjectID())
	if got := n.AggroHate(w.player); got <= 0 {
		t.Fatalf("hate of the character seen = %v, want it attacked", got)
	}
}

// TestCorpseZombiePrivateFollowsItsMasterAndOutlivesIt ports
// WarriorCorpseZombie's created, no-desire and 1004 timer hooks on a
// private (IsPrivate 1). Created, it clears int slot 0; with nothing to do
// it follows its live leader with weight 50, and the 20 s check keeps it.
// The leader's death lets it go (no master), so it wanders (5, weight 5)
// and the 20 s check, which only takes out a private whose master is
// dead, keeps it in the world. A zombie that is no private wants neither
// to follow nor to wander.
func TestCorpseZombiePrivateFollowsItsMasterAndOutlivesIt(t *testing.T) {
	t.Parallel()
	leaderTmpl := warriorTemplate(ind5gLeaderID, nil)
	leaderTmpl.Privates = []npc.PrivateEntry{{NpcID: int(ind5gZombieID), Weight: 1, RespawnDelay: 240 * time.Second}}
	w := bootIndividual5g(t, leaderTmpl, ind5gTemplate(ind5gZombieID, npc.AIParams{"IsPrivate": "1"}), ind5gTemplate(ind5gZombieTwoID, nil))
	leader := w.spawn(t, ind5gLeaderID, 400, 0)
	private := privatesOf(t, leader, 1)[0]
	if got := ind1Slot(private, script.SlotIntAI0); got != 0 {
		t.Fatalf("private int slot 0 = %d, want 0", got)
	}
	w.ticks(t, 2)
	if d, ok := desire(private, actorai.IntentionFollow); !ok || d.Weight != 50 || d.FinalTarget == nil || d.FinalTarget.ObjectID() != leader.ObjectID() {
		t.Fatalf("private follow desire = %+v (%v), want its leader with weight 50", d, ok)
	}
	w.srv.Advance(t, 20*time.Second)
	w.srv.ReadQueued(t, w.srv.Client)
	if !w.inWorld(private) {
		t.Fatal("the 20 s check took out the private of a live leader")
	}

	w.kill(t, leader)
	if private.Master() != nil {
		t.Fatal("the private still has its dead leader as master")
	}
	w.ind5dClear(t, private)
	w.ticks(t, 2)
	if d, ok := desire(private, actorai.IntentionWander); !ok || d.Weight != 5 || d.Timer != 5 {
		t.Fatalf("masterless private wander desire = %+v (%v), want timer 5, weight 5", d, ok)
	}
	for range 2 {
		w.srv.Advance(t, 20*time.Second)
	}
	frames := w.srv.ReadQueued(t, w.srv.Client)
	if !w.inWorld(private) {
		t.Fatal("the 20 s check took out a masterless private")
	}
	if got := framesOf(frames, serverpackets.OpcodeDeleteObject, private.ObjectID()); len(got) != 0 {
		t.Fatalf("DeleteObject frames for the masterless private = %d, want none", len(got))
	}

	loner := w.spawn(t, ind5gZombieTwoID, 0, 1500)
	w.ind5dClear(t, loner)
	w.ticks(t, 2)
	if d, ok := desire(loner, actorai.IntentionWander); ok {
		t.Fatalf("a zombie that is no private wants to wander: %+v", d)
	}
	if d, ok := desire(loner, actorai.IntentionFollow); ok {
		t.Fatalf("a zombie that is no private wants to follow: %+v", d)
	}
}

// TestCorpseZombieJumpsOntoItsFarMostHated ports WarriorCorpseZombie's
// attacked hook on a zombie set to jump (IsTeleport 1), every draw 0. The
// hit that makes the character its most hated, from 300, makes no jump.
// The next stops it and puts it on the character in one step: the
// character sees one fast TeleportToLocation of it, it wants its jump
// skill (4671-1) on the character, weight 1000000, and its attack desire
// on the character grows by the hit's 10 / 8 * 30 beside the Warrior's
// 10 / 8 * 100. On a draw of 10 it stays. The shipped zombies set
// IsTeleport to "4671-1", which is no int and so reads as 0: such a zombie
// never jumps.
func TestCorpseZombieJumpsOntoItsFarMostHated(t *testing.T) {
	t.Parallel()
	w := bootIndividual5g(t, ind5gStill(ind5gZombieID, ind5gTeleport), ind5gStill(ind5gZombieTwoID, npc.AIParams{"IsTeleport": "4671-1"}))
	me := w.player.ObjectID()
	n := w.spawn(t, ind5gZombieID, 300, 0)
	w.hit(t, n, 10)
	if x, _, _ := n.Position(); x != w.at.X+300 {
		t.Fatalf("the first hit moved the zombie to x %d", x)
	}
	before := ind5gAttackWeight(n, me)
	w.hit(t, n, 10)
	w.ind5gWantJumped(t, "a hit from 300 by its most hated", n, w.srv.ReadQueued(t, w.srv.Client))
	if got, want := ind5gAttackWeight(n, me)-before, 10.0/8*30+10.0/8*100; got != want {
		t.Fatalf("attack desire growth = %v, want %v", got, want)
	}

	still := w.spawn(t, ind5gZombieID, 0, 1000)
	w.moveCharacter(t, still, -300, 0)
	w.hit(t, still, 10)
	w.draws.set(0, 10)
	w.hit(t, still, 10)
	if x, _, _ := still.Position(); x != w.at.X {
		t.Fatalf("a draw of 10 moved the zombie to x %d", x)
	}
	ind5bNoCast(t, "a draw of 10", still, ind5gJump)

	unset := w.spawn(t, ind5gZombieTwoID, 0, 2000)
	w.moveCharacter(t, unset, -300, 0)
	w.draws.set(0)
	w.hit(t, unset, 10)
	w.hit(t, unset, 10)
	ind5bNoCast(t, "a zombie whose IsTeleport is no int", unset, ind5gJump)
}

// TestCorpseZombieJumpsOntoItsClansFarAttacker ports WarriorCorpseZombie's
// clan-attacked hook on a zombie set to jump, every draw 0. Told of the
// character's hit of 10 on a clan member from 300 away while it attacks
// nobody, it jumps onto the character, wants its jump skill on it and an
// attack on it of weight int(10 / 8 * 30) = 37: it has not lived past 7,
// so the Warrior adds nothing.
func TestCorpseZombieJumpsOntoItsClansFarAttacker(t *testing.T) {
	t.Parallel()
	w := bootIndividual5g(t, ind5dClan(ind5gStill(ind5gZombieID, ind5gTeleport)), ind5dClan(warriorTemplate(warriorID, nil)))
	victim := w.spawn(t, warriorID, 100, 0)
	member := w.spawn(t, ind5gZombieID, 300, 0)
	if member.LifeTime() > 7 {
		t.Fatalf("setup: member life time %d, want at most 7", member.LifeTime())
	}
	w.hit(t, victim, 10)
	w.ind5gWantJumped(t, "a hit on a clan member from 300", member, w.srv.ReadQueued(t, w.srv.Client))
	if got := ind5gAttackWeight(member, w.player.ObjectID()); got != 37 {
		t.Fatalf("attack desire = %v, want 37", got)
	}
}

// TestCorpseZombieDebuffsBelowATenthOfItsHP ports WarriorCorpseZombie's
// attacked and dying hooks, every draw 0. The hook reads the HP before
// the hit lands, so the hit leaving it at 5% wants nothing and the next
// wants its self-centred debuff on itself, weight 1000000, and sets int
// slot 0, so a later hit wants none. On a draw of 10 it wants none and
// keeps slot 0 clear. Dying, it draws for the debuff first.
func TestCorpseZombieDebuffsBelowATenthOfItsHP(t *testing.T) {
	t.Parallel()
	w := bootIndividual5g(t, ind5gStill(ind5gZombieID, nil), ind5gStill(ind5gZombieTwoID, npc.AIParams{"ShoutMsg4": "1000403"}))
	n := w.spawn(t, ind5gZombieID, 150, 0)
	w.hit(t, n, n.MaxHP()*95/100)
	ind5bNoCast(t, "the hit leaving 5%, read before it lands", n, ind5gDebuff)
	w.hit(t, n, 1)
	ind5bWantCast(t, "a hit at 5%", n, ind5gDebuff, n.ObjectID())
	if got := ind1Slot(n, script.SlotIntAI0); got != 1 {
		t.Fatalf("int slot 0 = %d, want 1", got)
	}
	w.ind5dClear(t, n)
	w.hit(t, n, 1)
	ind5bNoCast(t, "a hit once slot 0 is set", n, ind5gDebuff)

	other := w.spawn(t, ind5gZombieID, 0, 150)
	w.hit(t, other, other.MaxHP()*95/100)
	w.draws.set(0, 10)
	w.hit(t, other, 1)
	ind5bNoCast(t, "a hit at 5%, draw 10", other, ind5gDebuff)
	if got := ind1Slot(other, script.SlotIntAI0); got != 0 {
		t.Fatalf("int slot 0 after a draw of 10 = %d, want 0", got)
	}

	// The dying hook draws for its debuff before the Warrior's draws for
	// its death shout: a first draw of 0 leaves the shout a draw of 30,
	// which says nothing, and a first draw of 10 leaves it 29, which
	// shouts. A dead NPC keeps no desire, so the debuff is never cast.
	for _, row := range []struct {
		draw  int
		shout bool
	}{{0, false}, {10, true}} {
		dying := w.spawn(t, ind5gZombieTwoID, 0, -150)
		w.draws.set(0, row.draw, 30-row.draw/10)
		w.kill(t, dying)
		w.srv.Advance(t, 3*time.Second)
		shouts := framesOf(w.srv.ReadQueued(t, w.srv.Client), serverpackets.OpcodeNpcSay, dying.ObjectID())
		if got := len(shouts) == 1; got != row.shout {
			t.Fatalf("death on a first draw of %d: shouts %d, want shout %v", row.draw, len(shouts), row.shout)
		}
	}
}

// TestCorpseZombieDDMagicCastsAtItsTopTarget ports
// WarriorCorpseZombieDDMagic's attacked and clan-attacked hooks, every
// draw 0. The first hit, before it wants anything of the character, wants
// no damage magic; the next, from 300, wants it on the character, weight
// 1000000, and not on a draw of 33; a hit from 80 does not. A settled clan
// member wants it on the character 300 away at a hit on its clan.
func TestCorpseZombieDDMagicCastsAtItsTopTarget(t *testing.T) {
	t.Parallel()
	w := bootIndividual5g(t, ind5gStill(ind5gDDID, nil), ind5dClan(ind5gStill(ind5gDDClanID, nil)))
	me := w.player.ObjectID()
	n := w.spawn(t, ind5gDDID, 300, 0)
	w.hit(t, n, 1)
	ind5bNoCast(t, "the first hit", n, ind5gDDMagic)
	w.draws.set(0, 33)
	w.hit(t, n, 1)
	ind5bNoCast(t, "a hit from 300, draw 33", n, ind5gDDMagic)
	w.draws.set(0)
	w.hit(t, n, 1)
	ind5bWantCast(t, "a hit from 300 by its top target", n, ind5gDDMagic, me)
	w.ind5dClear(t, n)
	w.moveCharacter(t, n, -80, 0)
	w.hit(t, n, 1)
	ind5bNoCast(t, "a hit from 80", n, ind5gDDMagic)

	victim := w.spawn(t, ind5gDDClanID, 0, 1000)
	member := w.spawn(t, ind5gDDClanID, 300, 1000)
	w.moveCharacter(t, victim, -300, 0)
	w.ticks(t, 8)
	w.ind5dClear(t, member)
	w.hit(t, victim, 1)
	ind5bWantCast(t, "a settled clan member", member, ind5gDDMagic, me)
}

// TestCorpseZombieDDMagicAggressiveJumpsOrCastsOnSight ports
// WarriorCorpseZombieDDMagicAggressive's see-creature hook, every draw 0.
// Settled and attacking nobody, a zombie not set to jump sees the
// character 200 away, wants its damage magic on it, weight 1000000, and
// attacks it. One set to jump jumps onto it instead, wants its jump skill
// on it, and, now beside it, no damage magic.
func TestCorpseZombieDDMagicAggressiveJumpsOrCastsOnSight(t *testing.T) {
	t.Parallel()
	w := bootIndividual5g(t, ind5gTemplate(ind5gDDAggrID, nil), ind5gTemplate(ind5gDDAggrTwoID, ind5gTeleport))
	n, _ := w.ind5gSeeFromAfar(t, ind5gDDAggrID)
	ind5bWantCast(t, "seeing the character 200 away", n, ind5gDDMagic, w.player.ObjectID())
	ind5bNoCast(t, "seeing the character, not set to jump", n, ind5gJump)
	if got := n.AggroHate(w.player); got <= 0 {
		t.Fatalf("hate of the character seen = %v, want it attacked", got)
	}

	w.moveCharacter(t, n, 3000, 0)
	jumper, frames := w.ind5gSeeFromAfar(t, ind5gDDAggrTwoID)
	w.ind5gWantJumped(t, "seeing the character 200 away, set to jump", jumper, frames)
	ind5bNoCast(t, "seeing the character beside it after the jump", jumper, ind5gDDMagic)
}

// TestCorpseZombiePhysicalSpecialAggressiveJumpsOnSight ports
// WarriorCorpseZombiePhysicalSpecialAggressive's see-creature hook, every
// draw 0: settled, attacking nobody and set to jump, it sees the character
// 200 away, jumps onto it and wants its jump skill on it.
func TestCorpseZombiePhysicalSpecialAggressiveJumpsOnSight(t *testing.T) {
	t.Parallel()
	w := bootIndividual5g(t, ind5gTemplate(ind5gPSAggrID, ind5gTeleport))
	n, frames := w.ind5gSeeFromAfar(t, ind5gPSAggrID)
	w.ind5gWantJumped(t, "seeing the character 200 away", n, frames)
}

// TestCorpseZombiePhysicalSpecialStrikesItsMostHated ports
// WarriorCorpseZombiePhysicalSpecial's attacked and clan-attacked hooks,
// every draw 0. The hit that makes the character its most hated wants no
// physical special; the next wants it on the character, weight 1000000,
// not on a draw of 33. A settled clan member attacking nobody wants it on
// the character at a hit on its clan.
func TestCorpseZombiePhysicalSpecialStrikesItsMostHated(t *testing.T) {
	t.Parallel()
	w := bootIndividual5g(t, ind5gStill(ind5gPSID, nil), ind5dClan(ind5gStill(ind5gPSClanID, nil)))
	me := w.player.ObjectID()
	n := w.spawn(t, ind5gPSID, 100, 0)
	w.hit(t, n, 1)
	ind5bNoCast(t, "the hit that made the character its most hated", n, ind5gStrike)
	w.draws.set(0, 33)
	w.hit(t, n, 1)
	ind5bNoCast(t, "a hit by its most hated, draw 33", n, ind5gStrike)
	w.draws.set(0)
	w.hit(t, n, 1)
	ind5bWantCast(t, "a hit by its most hated", n, ind5gStrike, me)

	victim := w.spawn(t, ind5gPSClanID, 0, 1000)
	member := w.spawn(t, ind5gPSClanID, 200, 1000)
	w.moveCharacter(t, victim, -100, 0)
	w.ticks(t, 8)
	w.ind5dClear(t, member)
	w.hit(t, victim, 1)
	ind5bWantCast(t, "a settled clan member", member, ind5gStrike, me)
}
