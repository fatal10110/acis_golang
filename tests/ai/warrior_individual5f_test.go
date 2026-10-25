package ai

import (
	"fmt"
	"testing"

	"github.com/fatal10110/acis_golang/internal/commons/wire"
	actorai "github.com/fatal10110/acis_golang/internal/gameserver/model/actor/ai"
	"github.com/fatal10110/acis_golang/internal/gameserver/model/actor/npc"
	modelskill "github.com/fatal10110/acis_golang/internal/gameserver/model/skill"
	"github.com/fatal10110/acis_golang/internal/gameserver/network/clientpackets"
	"github.com/fatal10110/acis_golang/internal/gameserver/network/serverpackets"
	"github.com/fatal10110/acis_golang/internal/gameserver/script"
	"github.com/fatal10110/acis_golang/internal/gameserver/script/ai/warrior"
	"github.com/fatal10110/acis_golang/internal/gameservertest"
)

// The NPC ids of the fifth individual behavior batch's sixth PR the tests
// spawn.
const (
	ind5fSummonID     = int32(20213) // WarriorCastSummonPC
	ind5fSummonAggrID = int32(20221) // WarriorCastSummonPCAggressive
	ind5fLabClan1ID   = int32(27318) // WarriorCastingEnchantLabClan1
	ind5fLabID        = int32(22059) // WarriorPhysicalSpecialLabAggressive
	ind5fLabRunID     = int32(22063) // WarriorPhysicalSpecialLabRunAggressive
	ind5fHealID       = int32(20686) // WarriorCastingHeal
	ind5fHealAggrID   = int32(29972) // WarriorCastingHealAggressive, bound by the test
	ind5fHealAggrPath = "test.WarriorCastingHealAggressive"
)

// The skills the batch's fixture NPCs list by type.
const (
	ind5fSummon      = modelskill.ID(9550) // SUMMON_MAGIC
	ind5fStrike      = modelskill.ID(9551) // PHYSICAL_SPECIAL
	ind5fBuff1       = modelskill.ID(9552) // BUFF1
	ind5fSelfBuff1   = modelskill.ID(9553) // SELF_BUFF1
	ind5fSelfBuff2   = modelskill.ID(9554) // SELF_BUFF2
	ind5fRangeStrike = modelskill.ID(9555) // SELF_RANGE_PHYSICAL_SPECIAL
	ind5fHeal        = modelskill.ID(9556) // MAGIC_HEAL
)

// ind5fBatch are the classes of the PR, in scripts.xml order, and the
// test binding of WarriorCastingHealAggressive, which binds no NPC.
var ind5fBatch = []struct {
	path string
	ctor func() script.Script
}{
	{"script.ai.individual.Monster.WarriorBase.Warrior.WarriorCastSummonPC.WarriorCastSummonPC", warrior.CastSummonPC},
	{"script.ai.individual.Monster.WarriorBase.Warrior.WarriorCastSummonPC.WarriorCastSummonPCAggressive", warrior.CastSummonPCAggressive},
	{"script.ai.individual.Monster.WarriorBase.Warrior.WarriorCastingEnchantLabClan1.WarriorCastingEnchantLabClan1", warrior.CastingEnchantLabClan1},
	{"script.ai.individual.Monster.WarriorBase.Warrior.WarriorCastingEnchantLabClan1.WarriorCastingEnchantLabClan2.WarriorCastingEnchantLabClan2", warrior.CastingEnchantLabClan2},
	{"script.ai.individual.Monster.WarriorBase.Warrior.WarriorCastingEnchantLabClan1.WarriorCastingEnchantLabClan2.WarriorPhysicalSpecialLabAggressive", warrior.PhysicalSpecialLabAggressive},
	{"script.ai.individual.Monster.WarriorBase.Warrior.WarriorCastingEnchantLabClan1.WarriorCastingEnchantLabClan2.WarriorPhysicalSpecialLabRunAggressive", warrior.PhysicalSpecialLabRunAggressive},
	{"script.ai.individual.Monster.WarriorBase.Warrior.WarriorCastingHeal.WarriorCastingHeal", warrior.CastingHeal},
	{"script.ai.individual.Monster.WarriorBase.Warrior.WarriorCastingHeal.WarriorCastingHealAggressive", warrior.CastingHealAggressive},
	{ind5fHealAggrPath, func() script.Script {
		s := warrior.CastingHealAggressive()
		s.NPCs = []int32{ind5fHealAggrID}
		return s
	}},
}

// ind5fBuff is a fixture one-target buff of id, reach 600, with an effect
// of its own stack.
func ind5fBuff(id modelskill.ID) modelskill.Definition {
	d := ind1Skill(id, 600, false)
	d.Offensive, d.Magic, d.SkillType, d.Power = false, false, "BUFF", 0
	return ind5dStacked(d, fmt.Sprintf("ind5f_%d", id), 1)
}

// ind5fDefs are the batch's skills: the summon magic and the strike
// one-target attacks of reach 2000, the first buff and the heal one-target
// buffs of reach 600, the self buffs and the ranged strike self skills,
// each buff with an effect of its own stack.
func ind5fDefs() []modelskill.Definition {
	return []modelskill.Definition{
		ind1Skill(ind5fSummon, 2000, false), ind1Skill(ind5fStrike, 2000, false),
		ind5fBuff(ind5fBuff1), ind5fBuff(ind5fHeal),
		ind5dStacked(ind1Skill(ind5fSelfBuff1, 0, true), "ind5f_self1", 1),
		ind5dStacked(ind1Skill(ind5fSelfBuff2, 0, true), "ind5f_self2", 1),
		ind1Skill(ind5fRangeStrike, 0, true),
	}
}

// ind5fTemplate is a fixture monster of id listing the batch's skills by
// type.
func ind5fTemplate(id int32) *npc.Template {
	return ind5dTemplate(id, nil, map[npc.SkillType]modelskill.ID{
		npc.SkillTypeSummonMagic: ind5fSummon, npc.SkillTypePhysicalSpecial: ind5fStrike, npc.SkillTypeBuff1: ind5fBuff1,
		npc.SkillTypeSelfBuff1: ind5fSelfBuff1, npc.SkillTypeSelfBuff2: ind5fSelfBuff2,
		npc.SkillTypeSelfRangePhysicalSpecial: ind5fRangeStrike, npc.SkillTypeMagicHeal: ind5fHeal,
	})
}

// bootIndividual5f boots the PR's classes after the Warrior chain beside
// templates and an unbound decoy monster, with a director script to land
// effects through, every NPC casting the batch's skills, every draw 0
// until a scenario queues others.
func bootIndividual5f(t *testing.T, templates ...*npc.Template) *ind1World {
	t.Helper()
	list, catalog := warriorScripts()
	for _, c := range ind5fBatch {
		list = append(list, script.Listing{Path: c.path})
		catalog[c.path] = c.ctor
	}
	const director = "test.Ind1Director"
	catalog[director] = func() script.Script { return script.Script{} }
	list = append(list, script.Listing{Path: director})
	templates = append(templates, warriorTemplate(unboundID, nil))
	kinds := map[int32]script.NPCKind{}
	for _, tmpl := range templates {
		kinds[int32(tmpl.ID)] = script.KindHostile
	}
	draws := &ind1Draws{}
	opts := []gameservertest.Option{
		gameservertest.WithScriptRand(draws.rand),
		gameservertest.WithNPCScripts(kinds, list, catalog),
		gameservertest.WithNpcSpawnSkills(modelskill.NewTable(ind5fDefs())),
		gameservertest.WithScriptSkills(ind5fDefs()...),
	}
	w := bootRaid(t, raidBoot{opts: opts}, templates...)
	return &ind1World{warriorWorld: w, draws: draws, heard: &ind1Heard{}}
}

// ind5fSetSlots sets h's IntAI0 and IntAI1 on its queue.
func (w *ind1World) ind5fSetSlots(t *testing.T, h *npc.Hostile, ai0, ai1 int32) {
	t.Helper()
	w.onQueue(t, h, func() {
		script.NPCOf(h).SetInt(script.SlotIntAI0, ai0)
		script.NPCOf(h).SetInt(script.SlotIntAI1, ai1)
	})
}

// ind5fJoinMates brings the characters Golem<from> to Golem<to> into the
// world and into the party the scenario's character leads, and fails
// unless the party then holds to+1 members.
func (w *ind1World) ind5fJoinMates(t *testing.T, from, to int) {
	t.Helper()
	for i := from; i <= to; i++ {
		account, name := fmt.Sprintf("ind5fmate%d", i), fmt.Sprintf("Golem%d", i)
		w.srv.SeedCharacterFor(t, account, name, 20, 0)
		mate := w.srv.DialClient(t, account, 1)
		mate.Send(encodeGameStart())
		for j := 0; ; j++ {
			if j == 100 {
				t.Fatalf("no CharSelected for %s within 100 frames", name)
			}
			if mate.Read()[0] == serverpackets.OpcodeCharSelected {
				break
			}
		}
		mate.Send(wire.NewPacketWriter(clientpackets.OpcodeEnterWorld).Bytes())
		w.srv.ReadQueued(t, mate)
		w.srv.ReadQueued(t, w.srv.Client)
		w.srv.Client.Send(encodeJoinParty(name))
		w.srv.ReadQueued(t, mate)
		mate.Send(encodeAnswerJoinParty(1))
		w.srv.ReadQueued(t, mate)
		w.srv.ReadQueued(t, w.srv.Client)
	}
	size := make(chan int, 1)
	if !w.srv.PlayerQueue(t, w.player.ObjectID()).Post(func() { size <- len(script.PlayerOf(w.player).PartyMembers()) }) {
		t.Fatal("the character's queue is closed")
	}
	if got := <-size; got != to+1 {
		t.Fatalf("party members = %d, want %d", got, to+1)
	}
}

// ind5fAttacksOnSight fails unless an NPC of id spawned 150 from the
// character attacks it within one AI cycle.
func ind5fAttacksOnSight(t *testing.T, id int32) {
	t.Helper()
	w := bootIndividual5f(t, ind5fTemplate(id))
	n := w.spawn(t, id, 150, 0)
	w.ticks(t, 1)
	if got := n.AggroHate(w.player); got <= 0 {
		t.Fatalf("hate of the character seen = %v, want it attacked", got)
	}
}

// TestCastSummonPCWantsToSummonItsFarAttacker ports WarriorCastSummonPC's
// created and attacked hooks. It appears with i_ai0 and i_ai1 at 0. Every
// hit by the character stores it in c_ai0. With i_ai0 clear, a hit from
// over 300 away on a first draw under 50, or from over 100 away by the one
// it hates most on a draw under 50, wants its summon magic on the
// character, weight 1000000, and sets i_ai0 and i_ai1; not on a draw of 50,
// nor from 100 or closer, nor once i_ai0 is set. Hating a decoy most, a
// hit from 150 away wants it on a draw under 10, not on 10.
func TestCastSummonPCWantsToSummonItsFarAttacker(t *testing.T) {
	t.Parallel()
	w := bootIndividual5f(t, ind5fTemplate(ind5fSummonID))
	me := w.player.ObjectID()
	n := w.spawn(t, ind5fSummonID, 80, 0)
	if a0, a1 := ind1Slot(n, script.SlotIntAI0), ind1Slot(n, script.SlotIntAI1); a0 != 0 || a1 != 0 {
		t.Fatalf("created i_ai0, i_ai1 = %d, %d, want 0, 0", a0, a1)
	}
	for _, row := range []struct {
		dx, draw int
		summon   bool
	}{
		{80, 0, false}, {350, 49, true}, {350, 50, false}, {150, 49, true}, {150, 50, false}, {100, 0, false},
	} {
		when := fmt.Sprintf("a hit from %d, draw %d", row.dx, row.draw)
		w.moveCharacter(t, n, -row.dx, 0)
		w.clearDesires(t, n)
		w.ind5fSetSlots(t, n, 0, 0)
		w.draws.set(0, row.draw)
		w.hit(t, n, 1)
		if got := slotID(n, script.SlotCreature0); got != me {
			t.Fatalf("%s: c_ai0 = %d, want the character %d", when, got, me)
		}
		a0, a1 := ind1Slot(n, script.SlotIntAI0), ind1Slot(n, script.SlotIntAI1)
		if row.summon {
			ind5bWantCast(t, when, n, ind5fSummon, me)
			if a0 != 1 || a1 != 1 {
				t.Fatalf("%s: i_ai0, i_ai1 = %d, %d, want 1, 1", when, a0, a1)
			}
			continue
		}
		ind5bNoCast(t, when, n, ind5fSummon)
		if a0 != 0 || a1 != 0 {
			t.Fatalf("%s: i_ai0, i_ai1 = %d, %d, want 0, 0", when, a0, a1)
		}
	}

	w.moveCharacter(t, n, -350, 0)
	w.clearDesires(t, n)
	w.ind5fSetSlots(t, n, 1, 0)
	w.draws.set(0)
	w.hit(t, n, 1)
	ind5bNoCast(t, "a hit from 350 with i_ai0 set", n, ind5fSummon)

	decoyed := w.spawn(t, ind5fSummonID, 0, 600)
	w.ind5dDecoy(t, decoyed)
	w.moveCharacter(t, decoyed, -150, 0)
	w.draws.set(0, 10)
	w.hit(t, decoyed, 1)
	ind5bNoCast(t, "a hit from 150 hating a decoy most, draw 10", decoyed, ind5fSummon)
	w.draws.set(0, 9)
	w.hit(t, decoyed, 1)
	ind5bWantCast(t, "a hit from 150 hating a decoy most, draw 9", decoyed, ind5fSummon, me)
}

// TestCastSummonPCPullsItsAttackerAndStrikes ports WarriorCastSummonPC's
// skill-finished hook, which calls no parent. Once the summon magic its
// hit from 350 away wanted is cast, the character sees the MagicSkillUse
// and one TeleportToLocation, not fast, onto the NPC, and stands there;
// i_ai1 clears and i_ai0 stays set. Hating the character most, every
// draw 0, the NPC then casts its physical special skill on it, whose own
// finish pulls no more.
func TestCastSummonPCPullsItsAttackerAndStrikes(t *testing.T) {
	t.Parallel()
	w := bootIndividual5f(t, ind5fTemplate(ind5fSummonID))
	me := w.player.ObjectID()
	n := w.spawn(t, ind5fSummonID, 350, 0)
	w.hit(t, n, 1)
	w.ind5dClear(t, n)
	w.ind5fSetSlots(t, n, 0, 0)
	w.hit(t, n, 1)
	ind5bWantCast(t, "a hit from 350", n, ind5fSummon, me)

	var frames [][]byte
	w.srv.AdvanceUntil(t, "the character jumps onto the NPC", func() bool {
		frames = append(frames, w.srv.ReadQueued(t, w.srv.Client)...)
		x, _, _ := w.srv.PlayerPosition(t, me)
		nx, _, _ := n.Position()
		return x == nx
	})
	frames = append(frames, w.srv.ReadQueued(t, w.srv.Client)...)
	if got := castsOf(frames, n, ind5fSummon); len(got) != 1 || got[0] != me {
		t.Fatalf("summon magic MagicSkillUse targets = %v, want one on %d", got, me)
	}
	jumps := framesOf(frames, serverpackets.OpcodeTeleportToLocation, me)
	if len(jumps) != 1 {
		t.Fatalf("TeleportToLocation frames of the character = %d, want 1", len(jumps))
	}
	nx, ny, _ := n.Position()
	if at, fast := teleportFrame(jumps[0]); at.X != nx || at.Y != ny || fast {
		t.Fatalf("TeleportToLocation = %v fast=%v, want onto the NPC at %d,%d, not fast", at, fast, nx, ny)
	}
	if a0, a1 := ind1Slot(n, script.SlotIntAI0), ind1Slot(n, script.SlotIntAI1); a0 != 1 || a1 != 0 {
		t.Fatalf("after the pull i_ai0, i_ai1 = %d, %d, want 1, 0", a0, a1)
	}

	w.srv.AdvanceUntil(t, "the physical special strike on the character", func() bool {
		frames = append(frames, w.srv.ReadQueued(t, w.srv.Client)...)
		return len(castsOf(frames, n, ind5fStrike)) > 0
	})
	if got := castsOf(frames, n, ind5fStrike); got[0] != me {
		t.Fatalf("physical special MagicSkillUse target = %d, want the character %d", got[0], me)
	}
	w.ticks(t, 3)
	frames = append(frames, w.srv.ReadQueued(t, w.srv.Client)...)
	if got := len(framesOf(frames, serverpackets.OpcodeTeleportToLocation, me)); got != 1 {
		t.Fatalf("TeleportToLocation frames of the character after the strike = %d, want still 1", got)
	}
}

// TestCastSummonPCAggressiveAttacksOnSight ports
// WarriorCastSummonPCAggressive's see-creature hook: it attacks the
// character it sees.
func TestCastSummonPCAggressiveAttacksOnSight(t *testing.T) {
	t.Parallel()
	ind5fAttacksOnSight(t, ind5fSummonAggrID)
}

// TestCastingEnchantLabClan1BuffsItsClan ports
// WarriorCastingEnchantLabClan1's clan-attacked hook, every draw 0: told
// of the character's hit on a clan member, it wants its first buff on that
// member, weight 1000000, and still does once the member holds it.
func TestCastingEnchantLabClan1BuffsItsClan(t *testing.T) {
	t.Parallel()
	w := bootIndividual5f(t, ind5dClan(ind5fTemplate(ind5fLabClan1ID)), ind5dClan(warriorTemplate(warriorID, nil)))
	victim := w.spawn(t, warriorID, 200, 0)
	member := w.spawn(t, ind5fLabClan1ID, 100, 0)
	w.hit(t, victim, 10)
	ind5bWantCast(t, "a hit on a clan member", member, ind5fBuff1, victim.ObjectID())

	w.ind5dLand(t, modelskill.Ref{ID: ind5fBuff1, Level: 1}, script.NPCOf(victim))
	w.ind5dClear(t, member)
	w.hit(t, victim, 10)
	ind5bWantCast(t, "a hit on a clan member holding the buff", member, ind5fBuff1, victim.ObjectID())
}

// TestCastingEnchantLabClan2BuffsAgainstAParty ports
// WarriorCastingEnchantLabClan2's attacked hook through
// WarriorPhysicalSpecialLabRunAggressive at full HP, every draw 0. A hit by
// the character alone, or leading a party of 5, wants no self buff; leading
// 6, it wants its first self buff, weight 1000000, which resolves to
// itself, unless it holds it; leading 8, its second, unless it holds it.
func TestCastingEnchantLabClan2BuffsAgainstAParty(t *testing.T) {
	t.Parallel()
	w := bootIndividual5f(t, ind5fTemplate(ind5fLabRunID))
	n := w.spawn(t, ind5fLabRunID, 100, 0)
	noBuff := func(when string) {
		t.Helper()
		ind5bNoCast(t, when, n, ind5fSelfBuff1)
		ind5bNoCast(t, when, n, ind5fSelfBuff2)
	}
	w.hit(t, n, 1)
	noBuff("a hit by the character alone")

	w.ind5fJoinMates(t, 1, 4)
	w.ind5dClear(t, n)
	w.hit(t, n, 1)
	noBuff("a hit by a party of 5")

	w.ind5fJoinMates(t, 5, 5)
	w.ind5dClear(t, n)
	w.hit(t, n, 1)
	ind5bWantCast(t, "a hit by a party of 6", n, ind5fSelfBuff1, n.ObjectID())
	ind5bNoCast(t, "a hit by a party of 6", n, ind5fSelfBuff2)
	w.ind5dLand(t, modelskill.Ref{ID: ind5fSelfBuff1, Level: 1}, script.NPCOf(n))
	w.ind5dClear(t, n)
	w.hit(t, n, 1)
	noBuff("a hit by a party of 6, holding the first self buff")

	w.ind5fJoinMates(t, 6, 7)
	w.ind5dClear(t, n)
	w.hit(t, n, 1)
	ind5bWantCast(t, "a hit by a party of 8", n, ind5fSelfBuff2, n.ObjectID())
	ind5bNoCast(t, "a hit by a party of 8", n, ind5fSelfBuff1)
	w.ind5dLand(t, modelskill.Ref{ID: ind5fSelfBuff2, Level: 1}, script.NPCOf(n))
	w.ind5dClear(t, n)
	w.hit(t, n, 1)
	noBuff("a hit by a party of 8, holding the second self buff")
}

// TestPhysicalSpecialLabAggressiveStrikesAroundItself ports
// WarriorPhysicalSpecialLabAggressive's attacked hook: a hit on a draw
// under 33 wants its self-centred physical special skill at the
// character, weight 1000000, which resolves to itself; on a draw of 33,
// not.
func TestPhysicalSpecialLabAggressiveStrikesAroundItself(t *testing.T) {
	t.Parallel()
	w := bootIndividual5f(t, ind5fTemplate(ind5fLabID))
	n := w.spawn(t, ind5fLabID, 100, 0)
	w.draws.set(33)
	w.hit(t, n, 1)
	ind5bNoCast(t, "a hit, draw 33", n, ind5fRangeStrike)
	w.draws.set(0, 32)
	w.hit(t, n, 1)
	ind5bWantCast(t, "a hit, draw 32", n, ind5fRangeStrike, n.ObjectID())
}

// TestPhysicalSpecialLabAggressiveAttacksOnSight ports
// WarriorPhysicalSpecialLabAggressive's see-creature hook: it attacks the
// character it sees.
func TestPhysicalSpecialLabAggressiveAttacksOnSight(t *testing.T) {
	t.Parallel()
	ind5fAttacksOnSight(t, ind5fLabID)
}

// TestPhysicalSpecialLabRunAggressiveRunsHomeHurt ports
// WarriorPhysicalSpecialLabRunAggressive's attacked hook: at 40% of its
// HP a hit wants nothing more; below, it wants its physical special skill
// on the character, weight 1000000, and, moved out of its territory, to
// walk back to where it spawned, weight 1000000.
func TestPhysicalSpecialLabRunAggressiveRunsHomeHurt(t *testing.T) {
	t.Parallel()
	w := bootIndividual5f(t, ind5fTemplate(ind5fLabRunID))
	me := w.player.ObjectID()
	n := w.spawn(t, ind5fLabRunID, 100, 0)
	home := positionOf(n)
	noMove := func(when string) {
		t.Helper()
		if d, ok := desire(n, actorai.IntentionMoveTo); ok {
			t.Fatalf("%s: unexpected move desire %+v", when, d)
		}
	}
	w.setHPShare(t, n, 0.41)
	w.hit(t, n, 1)
	ind5bNoCast(t, "a hit leaving it above 40%", n, ind5fStrike)
	noMove("a hit leaving it above 40%")

	w.setHPShare(t, n, 0.3)
	w.ind5dClear(t, n)
	w.hit(t, n, 1)
	ind5bWantCast(t, "a hit below 40% in its territory", n, ind5fStrike, me)
	noMove("a hit below 40% in its territory")

	w.teleport(t, n, 1000, 0)
	if script.NPCOf(n).InMyTerritory() {
		t.Fatal("setup: the NPC is still in its territory")
	}
	w.ind5dClear(t, n)
	w.hit(t, n, 1)
	ind5bWantCast(t, "a hit below 40% out of its territory", n, ind5fStrike, me)
	d, ok := desire(n, actorai.IntentionMoveTo)
	if !ok || (d.Weight != 1000000 && d.Weight != 934000) || d.Location.X != home.X || d.Location.Y != home.Y {
		t.Fatalf("move desire = %+v (%v), want weight 1000000 to its spawn location %v", d, ok, home)
	}
}

// TestCastingHealHealsItselfHurt ports WarriorCastingHeal's attacked hook:
// a hit leaving it below 70% of its HP on a draw under 33 wants its heal
// on itself, weight 1000000; on a draw of 33, or above 70%, not.
func TestCastingHealHealsItselfHurt(t *testing.T) {
	t.Parallel()
	w := bootIndividual5f(t, ind5fTemplate(ind5fHealID))
	n := w.spawn(t, ind5fHealID, 100, 0)
	w.setHPShare(t, n, 0.9)
	w.hit(t, n, 1)
	ind5bNoCast(t, "a hit leaving it above 70%", n, ind5fHeal)
	w.setHPShare(t, n, 0.6)
	w.draws.set(33)
	w.hit(t, n, 1)
	ind5bNoCast(t, "a hit below 70%, draw 33", n, ind5fHeal)
	w.draws.set(0, 32)
	w.hit(t, n, 1)
	ind5bWantCast(t, "a hit below 70%, draw 32", n, ind5fHeal, n.ObjectID())
}

// TestCastingHealHealsItsClan ports WarriorCastingHeal's clan-attacked
// hook: told of the character's hit on a clan member while it does not
// attack, on a draw under 33, it wants its heal on that member, weight
// 1000000; on a draw of 33, or while it attacks, not.
func TestCastingHealHealsItsClan(t *testing.T) {
	t.Parallel()
	w := bootIndividual5f(t, ind5dClan(ind5fTemplate(ind5fHealID)), ind5dClan(warriorTemplate(warriorID, nil)))
	victim := w.spawn(t, warriorID, 200, 0)
	healer := w.spawn(t, ind5fHealID, 100, 0)
	w.draws.set(33)
	w.hit(t, victim, 10)
	ind5bNoCast(t, "a hit on a clan member, draw 33", healer, ind5fHeal)
	w.draws.set(0)
	w.ind5dClear(t, healer)
	w.hit(t, victim, 10)
	ind5bWantCast(t, "a hit on a clan member, draw 0", healer, ind5fHeal, victim.ObjectID())

	w.ind5dClear(t, healer)
	w.hit(t, healer, 10)
	w.ticks(t, 1)
	if got := healer.AI().CurrentIntention(); got != actorai.IntentionAttack {
		t.Fatalf("setup: the healer's intention = %v, want attack", got)
	}
	w.ind5dClear(t, healer)
	w.hit(t, victim, 10)
	ind5bNoCast(t, "a hit on a clan member while attacking", healer, ind5fHeal)
}

// TestCastingHealAggressiveAttacksOnSight ports
// WarriorCastingHealAggressive's see-creature hook, the class bound to a
// fixture NPC: it attacks the character it sees.
func TestCastingHealAggressiveAttacksOnSight(t *testing.T) {
	t.Parallel()
	ind5fAttacksOnSight(t, ind5fHealAggrID)
}
