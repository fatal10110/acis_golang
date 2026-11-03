package ai

import (
	"encoding/binary"
	"testing"
	"time"

	actorai "github.com/fatal10110/acis_golang/internal/gameserver/model/actor/ai"
	"github.com/fatal10110/acis_golang/internal/gameserver/model/actor/npc"
	modelskill "github.com/fatal10110/acis_golang/internal/gameserver/model/skill"
	"github.com/fatal10110/acis_golang/internal/gameserver/network/serverpackets"
	"github.com/fatal10110/acis_golang/internal/gameserver/script"
	"github.com/fatal10110/acis_golang/internal/gameservertest"
)

// The NPC ids the scenarios of the fourth individual behavior batch's
// second PR (#189 PR 2) spawn, each a fixture monster of level 1.
const (
	ind4bEnchantID   = int32(20976) // PartyPrivateCastEnchant
	ind4bHealID      = int32(20774) // PartyPrivateCastHeal
	ind4bHealCurseID = int32(21060) // PartyPrivateCastHealCurse
	ind4bCoupleID    = int32(21433) // PartyPrivateCouplePhysicalSpecial
	ind4bCowardID    = int32(21313) // PartyPrivateCoward
	ind4bPabelID     = int32(22061) // PartyPrivatePabelgolem
	ind4bStoneID     = int32(21344) // PartyPrivatePhysicalStone
	ind4bPriestID    = int32(20970) // PartyPrivatePriest
)

// The skills the PR's fixture NPCs list by type beside those of
// ind4Template.
const (
	ind4bBuffSkill     = modelskill.ID(9708)
	ind4bSpecialSkill  = modelskill.ID(9709)
	ind4bSelfBuffSkill = modelskill.ID(9710)
	ind4bBuffStack     = "ind4b_buff"
)

// ind4bTemplate is ind4Template listing also a buff, a physical special
// skill and a self buff by type.
func ind4bTemplate(id int32, clan bool) *npc.Template {
	tmpl := ind4Template(id, clan)
	tmpl.SkillsByType[npc.SkillTypeBuff] = modelskill.Ref{ID: ind4bBuffSkill, Level: 1}
	tmpl.SkillsByType[npc.SkillTypePhysicalSpecial] = modelskill.Ref{ID: ind4bSpecialSkill, Level: 1}
	tmpl.SkillsByType[npc.SkillTypeSelfBuff] = modelskill.Ref{ID: ind4bSelfBuffSkill, Level: 1}
	return tmpl
}

// ind4bBuff is the fixture buff the NPCs cast: a one-target spell on a
// friend reaching 2000, landing no effect.
func ind4bBuff() modelskill.Definition {
	d := ind1Skill(ind4bBuffSkill, 2000, false)
	d.Offensive, d.Magic, d.SkillType, d.Power = false, false, "BUFF", 0
	return d
}

// ind4bHeldBuff is the fixture buff as the scripts read it, whose effect
// lasts 600 s.
func ind4bHeldBuff() modelskill.Definition {
	d := ind4bBuff()
	d.Effects = []modelskill.EffectTemplate{{Name: "Buff", Time: 600, Icon: true, StackType: ind4bBuffStack, StackOrder: 1}}
	return d
}

// ind4bSkills are ind4Skills with the buff, the physical special skill (a
// one-target attack reaching 2000) and the self buff.
func ind4bSkills() *modelskill.Table {
	return modelskill.NewTable(append(ind4Skills().All(), ind4bBuff(), ind1Skill(ind4bSpecialSkill, 2000, false), ind1Skill(ind4bSelfBuffSkill, 0, true)))
}

// ind4bCastsOf returns the target object id of each MagicSkillUse frame h
// cast id with.
func ind4bCastsOf(frames [][]byte, h *npc.Hostile, id modelskill.ID) []int32 {
	var out []int32
	for _, f := range framesOf(frames, serverpackets.OpcodeMagicSkillUse, h.ObjectID()) {
		// opcode, caster, target, skill id, then the rest of the cast.
		if modelskill.ID(binary.LittleEndian.Uint32(f[9:13])) == id {
			out = append(out, int32(binary.LittleEndian.Uint32(f[5:9])))
		}
	}
	return out
}

// TestPartyPrivateCastEnchantBuffsItselfOnce ports PartyPrivateCastEnchant's
// created and attacked hooks. It spawns with i_ai0 and i_ai1 clear. A
// player's hit while it is above half its HP, on a draw under 33, makes it
// want its buff on itself with weight 1000000, which it then casts
// (MagicSkillUse), and sets i_ai1: the next hit wants no buff. A failed
// draw wants none and leaves i_ai1 clear, and so does a hit at half its HP
// or below.
func TestPartyPrivateCastEnchantBuffsItselfOnce(t *testing.T) {
	t.Parallel()
	w := bootIndividual4(t, raidBoot{}, ind4bTemplate(ind4bEnchantID, false))
	n := w.spawn(t, ind4bEnchantID, 300, 0)
	w.srv.InstallNPCCastSeam(t, n, ind4bSkills())
	if a0, a1 := ind1Slot(n, script.SlotIntAI0), ind1Slot(n, script.SlotIntAI1); a0 != 0 || a1 != 0 {
		t.Fatalf("created i_ai0, i_ai1 = %d, %d, want 0, 0", a0, a1)
	}

	w.hit(t, n, 10)
	w.wantCasts(t, "after the first hit", n, map[modelskill.ID]ind1Cast{ind4bBuffSkill: {n.ObjectID(), 1000000}})
	if got := ind1Slot(n, script.SlotIntAI1); got != 1 {
		t.Fatalf("i_ai1 after the buff = %d, want 1", got)
	}
	frames := w.ticks(t, 2)
	if got := ind4bCastsOf(frames, n, ind4bBuffSkill); len(got) != 1 || got[0] != n.ObjectID() {
		t.Fatalf("buff casts = %v, want one on itself", got)
	}
	w.clearDesires(t, n)
	w.hit(t, n, 10)
	w.wantCasts(t, "after a hit with i_ai1 set", n, nil)

	low := w.spawn(t, ind4bEnchantID, 0, 300)
	w.srv.InstallNPCCastSeam(t, low, ind4bSkills())
	w.draws.set(33)
	w.hit(t, low, int(low.MaxHPValue()*6/10))
	w.wantCasts(t, "after a hit with a failed draw", low, nil)
	w.draws.set(0)
	w.hit(t, low, 1)
	w.wantCasts(t, "after a hit under half its HP", low, nil)
	if got := ind1Slot(low, script.SlotIntAI1); got != 0 {
		t.Fatalf("i_ai1 with no buff wanted = %d, want 0", got)
	}
}

// TestPartyPrivateCastHealHealsItsParty ports PartyPrivateCastHeal's
// attacked and party-attacked hooks for two privates of one master, every
// draw 0. The hooks run before the hit takes its HP, so the hit that takes
// the victim to half its HP heals nothing. The next hit, the victim under
// 70%, makes the victim want its heal on itself twice over (weight
// 2000000: its attacked hook and its own party call) and the other
// private its heal on the victim (weight 1000000). With a failed draw,
// neither wants a heal.
func TestPartyPrivateCastHealHealsItsParty(t *testing.T) {
	t.Parallel()
	w := bootIndividual4(t, raidBoot{}, ind4Leader(ind4bHealID, 2), ind4bTemplate(ind4bHealID, false))
	leader := w.spawn(t, leaderID, 400, 0)
	privates := privatesOf(t, leader, 2)
	victim, other := privates[0], privates[1]
	for _, h := range privates {
		w.srv.InstallNPCCastSeam(t, h, ind4bSkills())
	}
	reset := func() {
		for _, h := range privates {
			w.clearDesires(t, h)
		}
	}
	reset()

	w.hit(t, victim, int(victim.MaxHPValue()/2))
	w.wantCasts(t, "victim after the hit to half its HP", victim, nil)
	w.wantCasts(t, "other private after the hit to half its HP", other, nil)

	reset()
	w.draws.set(33)
	w.hit(t, victim, 1)
	w.wantCasts(t, "victim after a hit with a failed draw", victim, nil)
	w.wantCasts(t, "other private after a hit with a failed draw", other, nil)

	reset()
	w.draws.set(0)
	w.hit(t, victim, 1)
	w.wantCasts(t, "victim after a hit under 70%", victim, map[modelskill.ID]ind1Cast{ind4HealSkill: {victim.ObjectID(), 2000000}})
	w.wantCasts(t, "other private after a hit under 70%", other, map[modelskill.ID]ind1Cast{ind4HealSkill: {victim.ObjectID(), 1000000}})
}

// TestPartyPrivateCastHealCurseCursesItsMostHated ports
// PartyPrivateCastHealCurse's attacked and party-attacked hooks, every draw
// 0. Its first hit, hating nobody yet, wants nothing; a hit by its most
// hated wants the debuff on it with weight 1000000. As a private, the first
// hit on its master wants nothing and the second, the character now its
// most hated, wants the debuff. Its master and itself well above 70% of
// their HP, it wants no heal.
func TestPartyPrivateCastHealCurseCursesItsMostHated(t *testing.T) {
	t.Parallel()
	t.Run("attacked", func(t *testing.T) {
		t.Parallel()
		w := bootIndividual4(t, raidBoot{}, ind4bTemplate(ind4bHealCurseID, false))
		n := w.spawn(t, ind4bHealCurseID, 300, 0)
		w.srv.InstallNPCCastSeam(t, n, ind4bSkills())
		w.hit(t, n, 10)
		w.wantCasts(t, "after the first hit", n, nil)
		w.clearDesires(t, n)
		w.hit(t, n, 10)
		w.wantCasts(t, "after a hit by the most hated", n, map[modelskill.ID]ind1Cast{ind4DebuffSkill: {w.player.ObjectID(), 1000000}})
	})
	t.Run("party attacked", func(t *testing.T) {
		t.Parallel()
		w := bootIndividual4(t, raidBoot{}, ind4Leader(ind4bHealCurseID, 1), ind4bTemplate(ind4bHealCurseID, false))
		leader := w.spawn(t, leaderID, 400, 0)
		private := privatesOf(t, leader, 1)[0]
		w.srv.InstallNPCCastSeam(t, private, ind4bSkills())
		w.clearDesires(t, private)
		w.hit(t, leader, 10)
		w.wantCasts(t, "after the first hit on its master", private, nil)
		w.clearDesires(t, private)
		w.hit(t, leader, 10)
		w.wantCasts(t, "after the second hit on its master", private, map[modelskill.ID]ind1Cast{ind4DebuffSkill: {w.player.ObjectID(), 1000000}})
	})
}

// TestPartyPrivateCouplePhysicalSpecialStrikesOnceWhenLow ports
// PartyPrivateCouplePhysicalSpecial's attacked hook, every draw 0. A hit
// by its top desire target wants nothing while it stands at 20% of its HP
// or above, nor, under 20%, a hit by a player that is not its top desire
// target. Under 20%, a hit by its top desire target wants its physical
// special skill on it with weight 1000000 and sets i_ai0, after which the
// next such hit wants nothing.
func TestPartyPrivateCouplePhysicalSpecialStrikesOnceWhenLow(t *testing.T) {
	t.Parallel()
	w := bootIndividual4(t, raidBoot{}, ind4bTemplate(ind4bCoupleID, false))
	n := w.spawn(t, ind4bCoupleID, 300, 0)
	w.srv.InstallNPCCastSeam(t, n, ind4bSkills())
	me := w.player.ObjectID()
	top := func(on bool) {
		w.clearDesires(t, n)
		w.onQueue(t, n, func() {
			if on {
				script.NPCOf(n).SetTopDesireTarget(script.PlayerOf(w.player))
			} else {
				script.NPCOf(n).SetTopDesireTarget(nil)
			}
		})
	}

	top(true)
	w.hit(t, n, int(n.MaxHPValue()*85/100))
	w.wantCasts(t, "after a hit at full HP", n, nil)
	if r := script.NPCOf(n).HPRatio(); r >= 0.2 {
		t.Fatalf("setup: HP ratio after the hit = %v, want under 0.2", r)
	}
	top(false)
	w.hit(t, n, 1)
	w.wantCasts(t, "after a hit by a player not its top desire target", n, nil)
	top(true)
	w.hit(t, n, 1)
	w.wantCasts(t, "after a hit by its top desire target under 20%", n, map[modelskill.ID]ind1Cast{ind4bSpecialSkill: {me, 1000000}})
	if got := ind1Slot(n, script.SlotIntAI0); got != 1 {
		t.Fatalf("i_ai0 after the strike = %d, want 1", got)
	}
	top(true)
	w.hit(t, n, 1)
	w.wantCasts(t, "after a hit with i_ai0 set", n, nil)
}

// TestPartyPrivateCouplePhysicalSpecialAnswersItsClanOnce ports
// PartyPrivateCouplePhysicalSpecial's clan-attacked hook, every draw 0: a
// settled clan member wants its physical special skill on another
// member's attacker with weight 1000000 and sets i_ai0, so the next hit
// wants nothing; a young one wants nothing.
func TestPartyPrivateCouplePhysicalSpecialAnswersItsClanOnce(t *testing.T) {
	t.Parallel()
	for _, row := range []struct {
		name   string
		settle int
		strike bool
	}{{"settled member", 8, true}, {"young member", 0, false}} {
		t.Run(row.name, func(t *testing.T) {
			t.Parallel()
			w := bootIndividual4(t, raidBoot{}, ind4bTemplate(ind4bCoupleID, true))
			victim := w.spawn(t, ind4bCoupleID, 1500, 0)
			member := w.spawn(t, ind4bCoupleID, 1500, 200)
			w.srv.InstallNPCCastSeam(t, member, ind4bSkills())
			w.ticks(t, row.settle)
			w.clearDesires(t, member)
			w.hit(t, victim, 10)
			want := map[modelskill.ID]ind1Cast{}
			if row.strike {
				want[ind4bSpecialSkill] = ind1Cast{w.player.ObjectID(), 1000000}
			}
			w.wantCasts(t, "member after the hit", member, want)
			if !row.strike {
				return
			}
			if got := ind1Slot(member, script.SlotIntAI0); got != 1 {
				t.Fatalf("member i_ai0 after the strike = %d, want 1", got)
			}
			w.clearDesires(t, member)
			w.hit(t, victim, 10)
			w.wantCasts(t, "member after the next hit", member, nil)
		})
	}
}

// TestPartyPrivateCouplePhysicalSpecialRallies ports
// PartyPrivateCouplePhysicalSpecial's party-died hook for two privates of
// one master. When the other private dies within 100 of it, it wants to
// heal and buff itself, weight 1000000 each, and drops its attack desires
// to want to attack only the creature it most wants to attack, weight
// 1000. The other private dying 300 away changes nothing.
func TestPartyPrivateCouplePhysicalSpecialRallies(t *testing.T) {
	t.Parallel()
	for _, row := range []struct {
		name  string
		dx    int
		rally bool
	}{{"near", 50, true}, {"far", 300, false}} {
		t.Run(row.name, func(t *testing.T) {
			t.Parallel()
			w := bootIndividual4(t, raidBoot{}, ind4Leader(ind4bCoupleID, 2), ind4bTemplate(ind4bCoupleID, false))
			leader := w.spawn(t, leaderID, 300, 0)
			privates := privatesOf(t, leader, 2)
			survivor, dying := privates[0], privates[1]
			w.srv.InstallNPCCastSeam(t, survivor, ind4bSkills())
			me := w.player.ObjectID()

			w.hit(t, survivor, 10)
			w.ticks(t, 1)
			if top := survivor.AI().TopDesireTarget(); top == nil || top.ObjectID() != me {
				t.Fatalf("setup: top desire target = %v, want the character", top)
			}
			w.clearDesires(t, survivor)
			sx, sy, sz := survivor.Position()
			w.onQueue(t, dying, func() {
				script.NPCOf(dying).InstantTeleportTo(script.Loc{X: sx + row.dx, Y: sy, Z: sz})
			})
			w.kill(t, dying)

			want := map[modelskill.ID]ind1Cast{}
			if row.rally {
				want[ind4HealSkill] = ind1Cast{survivor.ObjectID(), 1000000}
				want[ind4bSelfBuffSkill] = ind1Cast{survivor.ObjectID(), 1000000}
			}
			w.wantCasts(t, "after the other private died", survivor, want)
			d, ok := desire(survivor, actorai.IntentionAttack)
			if !row.rally {
				if ok {
					t.Fatalf("attack desire after a far private died = %+v, want none", d)
				}
				return
			}
			wantDesire(t, "after the other private died", survivor, actorai.IntentionAttack, me, 1000)
		})
	}
}

// TestPartyPrivateCowardStrikesWhatItFights ports PartyPrivateCoward's
// attacked hook, every draw 0: its first hit, hating nobody yet, wants
// nothing; a hit by its most hated wants its physical special skill on it
// with weight 1000000.
func TestPartyPrivateCowardStrikesWhatItFights(t *testing.T) {
	t.Parallel()
	w := bootIndividual4(t, raidBoot{}, ind4bTemplate(ind4bCowardID, false))
	n := w.spawn(t, ind4bCowardID, 300, 0)
	w.srv.InstallNPCCastSeam(t, n, ind4bSkills())
	w.hit(t, n, 10)
	w.wantCasts(t, "after the first hit", n, nil)
	w.clearDesires(t, n)
	w.hit(t, n, 10)
	w.wantCasts(t, "after a hit by the most hated", n, map[modelskill.ID]ind1Cast{ind4bSpecialSkill: {w.player.ObjectID(), 1000000}})
}

// TestPartyPrivateCowardAnswersItsClanAndParty ports PartyPrivateCoward's
// clan-attacked and party-attacked hooks, every draw 0: a settled member
// that is not attacking wants its physical special skill on the attacker
// of a clan member, and on its master's, with weight 1000000. A young one
// wants nothing, and neither does a settled one that is attacking.
func TestPartyPrivateCowardAnswersItsClanAndParty(t *testing.T) {
	t.Parallel()
	for _, row := range []struct {
		name      string
		settle    int
		attacking bool
		strike    bool
	}{{"settled", 8, false, true}, {"young", 0, false, false}, {"attacking", 8, true, false}} {
		t.Run("clan/"+row.name, func(t *testing.T) {
			t.Parallel()
			w := bootIndividual4(t, raidBoot{}, ind4bTemplate(ind4bCowardID, true))
			victim := w.spawn(t, ind4bCowardID, 1500, 0)
			member := w.spawn(t, ind4bCowardID, 1500, 200)
			w.ticks(t, row.settle)
			if row.attacking {
				ind4bTakeUpAttack(t, w, member)
			}
			w.srv.InstallNPCCastSeam(t, member, ind4bSkills())
			w.clearDesires(t, member)
			w.hit(t, victim, 10)
			want := map[modelskill.ID]ind1Cast{}
			if row.strike {
				want[ind4bSpecialSkill] = ind1Cast{w.player.ObjectID(), 1000000}
			}
			w.wantCasts(t, "member after the hit", member, want)
		})
		t.Run("party/"+row.name, func(t *testing.T) {
			t.Parallel()
			w := bootIndividual4(t, raidBoot{}, ind4Leader(ind4bCowardID, 1), ind4bTemplate(ind4bCowardID, false))
			leader := w.spawn(t, leaderID, 1500, 0)
			private := privatesOf(t, leader, 1)[0]
			w.ticks(t, row.settle)
			if row.attacking {
				ind4bTakeUpAttack(t, w, private)
			}
			w.srv.InstallNPCCastSeam(t, private, ind4bSkills())
			w.clearDesires(t, private)
			w.hit(t, leader, 10)
			want := map[modelskill.ID]ind1Cast{}
			if row.strike {
				want[ind4bSpecialSkill] = ind1Cast{w.player.ObjectID(), 1000000}
			}
			w.wantCasts(t, "private after the hit on its master", private, want)
		})
	}
}

// ind4bTakeUpAttack makes h, which knows no skills yet, take up its attack
// on the character: the character steps beside it and hits it, the AI
// thinks once, and the character steps back to 300.
func ind4bTakeUpAttack(t *testing.T, w *ind1World, h *npc.Hostile) {
	t.Helper()
	w.moveCharacter(t, h, 50, 0)
	w.hit(t, h, 10)
	w.ticks(t, 1)
	if h.AI().CurrentIntention() != actorai.IntentionAttack {
		t.Fatalf("setup: intention = %v, want attack", h.AI().CurrentIntention())
	}
	w.moveCharacter(t, h, 300, 0)
}

// TestPartyPrivateCowardRunsFromItsDeadMaster ports PartyPrivateCoward's
// party-died hook for two privates of one master. The other private's
// death changes nothing. Its master's death makes it want to run 500 from
// the master, weight 10000000; a flee from a dead creature leaves the
// queue before a desire is chosen, so it never runs: the coward stays
// idle and sends no MoveToLocation.
func TestPartyPrivateCowardRunsFromItsDeadMaster(t *testing.T) {
	t.Parallel()
	w := bootIndividual4(t, raidBoot{}, ind4Leader(ind4bCowardID, 2), ind4bTemplate(ind4bCowardID, false))
	leader := w.spawn(t, leaderID, 300, 0)
	privates := privatesOf(t, leader, 2)
	coward, other := privates[0], privates[1]
	w.clearDesires(t, coward)

	w.kill(t, other)
	if d, ok := desire(coward, actorai.IntentionFlee); ok {
		t.Fatalf("flee desire after the other private died = %+v, want none", d)
	}

	w.kill(t, leader)
	d, ok := desire(coward, actorai.IntentionFlee)
	if !ok || d.Distance != 500 || d.Weight != 10000000 || d.FinalTarget == nil || d.FinalTarget.ObjectID() != leader.ObjectID() {
		t.Fatalf("flee desire after its master died = %+v (%v), want a 500 run from the master, weight 10000000", d, ok)
	}
	frames := w.ticks(t, 1)
	if d, ok := desire(coward, actorai.IntentionFlee); ok {
		t.Fatalf("flee desire after a think = %+v, want it dropped with its dead target", d)
	}
	if got := coward.AI().CurrentIntention(); got == actorai.IntentionFlee {
		t.Fatalf("intention after its master died = %v, want no flee", got)
	}
	if got := framesOf(frames, serverpackets.OpcodeMoveToLocation, coward.ObjectID()); len(got) != 0 {
		t.Fatalf("MoveToLocation frames for the coward = %d, want none", len(got))
	}
}

// TestPartyPrivatePabelgolemCastsAtWhatHitsIt ports PartyPrivatePabelgolem's
// attacked hook: on a draw under 33 a hit makes it want its DD magic on
// the attacker with weight 1000000, which it then casts (MagicSkillUse); a
// draw of 33 wants none.
func TestPartyPrivatePabelgolemCastsAtWhatHitsIt(t *testing.T) {
	t.Parallel()
	w := bootIndividual4(t, raidBoot{}, ind4bTemplate(ind4bPabelID, false))
	n := w.spawn(t, ind4bPabelID, 300, 0)
	w.srv.InstallNPCCastSeam(t, n, ind4bSkills())
	me := w.player.ObjectID()

	w.draws.set(33)
	w.hit(t, n, 10)
	w.wantCasts(t, "after a hit with a draw of 33", n, nil)

	w.draws.set(0)
	w.hit(t, n, 10)
	w.wantCasts(t, "after a hit with a draw of 0", n, map[modelskill.ID]ind1Cast{ind4DDMagicSkill: {me, 1000000}})
	frames := w.ticks(t, 2)
	if got := ind4bCastsOf(frames, n, ind4DDMagicSkill); len(got) != 1 || got[0] != me {
		t.Fatalf("DD magic casts = %v, want one on the character", got)
	}
}

// TestPartyPrivatePriestBuffsItsMaster ports PartyPrivatePriest's created,
// timer and see-creature hooks. Seeing its master only when it looks
// around, it wants nothing before its 20 s timer first fires. Each look
// that finds its master, while it is not attacking and the master holds
// no effect of its buff's stack, wants the buff on the master with weight
// 1000000, which it then casts (MagicSkillUse). A look comes at most once
// per 30 s, so the 40 s tick finds nothing. With the buff on the master,
// the look wants none.
func TestPartyPrivatePriestBuffsItsMaster(t *testing.T) {
	t.Parallel()
	w := bootIndividual4(t, raidBoot{opts: []gameservertest.Option{gameservertest.WithScriptSkills(ind4bHeldBuff())}}, ind4Leader(ind4bPriestID, 1), ind4bTemplate(ind4bPriestID, false))
	leader := w.spawn(t, leaderID, 400, 0)
	priest := privatesOf(t, leader, 1)[0]
	w.srv.InstallNPCCastSeam(t, priest, ind4bSkills())
	buff := map[modelskill.ID]ind1Cast{ind4bBuffSkill: {leader.ObjectID(), 1000000}}
	w.clearDesires(t, priest)
	w.ticks(t, 2)
	w.wantCasts(t, "before the first look", priest, nil)

	w.srv.Advance(t, 20*time.Second)
	w.wantCasts(t, "after the 20 s look", priest, buff)
	frames := w.ticks(t, 2)
	if got := ind4bCastsOf(frames, priest, ind4bBuffSkill); len(got) != 1 || got[0] != leader.ObjectID() {
		t.Fatalf("buff casts = %v, want one on its master", got)
	}

	w.clearDesires(t, priest)
	w.srv.Advance(t, 20*time.Second)
	w.wantCasts(t, "after the 40 s tick, 20 s after the last look", priest, nil)
	w.srv.Advance(t, 20*time.Second)
	w.wantCasts(t, "after the 60 s look", priest, buff)

	me := w.player.ObjectID()
	if !w.srv.RunScript(t, me, "Ind1Director", func(s *script.Script, _ *script.Player) {
		s.ApplyEffects(modelskill.Ref{ID: ind4bBuffSkill, Level: 1}, script.NPCOf(priest), script.NPCOf(leader))
	}) {
		t.Fatal("the effect landing panicked")
	}
	w.srv.AdvanceUntil(t, "the buff on the master", func() bool {
		var level int32
		w.srv.RunScript(t, me, "Ind1Director", func(s *script.Script, _ *script.Player) {
			level = s.AbnormalLevel(script.NPCOf(leader), modelskill.Ref{ID: ind4bBuffSkill, Level: 1})
		})
		return level > 0
	})
	w.clearDesires(t, priest)
	w.srv.Advance(t, 40*time.Second)
	w.wantCasts(t, "after a look with the buff on the master", priest, nil)
}

// TestPartyPrivatePriestSparesItsMasterWhileAttacking ports the intention
// gate of PartyPrivatePriest's see-creature hook: a priest attacking the
// character when its 20 s look finds its master wants no buff on it.
func TestPartyPrivatePriestSparesItsMasterWhileAttacking(t *testing.T) {
	t.Parallel()
	w := bootIndividual4(t, raidBoot{}, ind4Leader(ind4bPriestID, 1), ind4bTemplate(ind4bPriestID, false))
	leader := w.spawn(t, leaderID, 400, 0)
	priest := privatesOf(t, leader, 1)[0]
	ind4bTakeUpAttack(t, w, priest)
	w.srv.InstallNPCCastSeam(t, priest, ind4bSkills())
	w.srv.Advance(t, 20*time.Second)
	if got := priest.AI().CurrentIntention(); got != actorai.IntentionAttack {
		t.Fatalf("setup: intention at the look = %v, want attack", got)
	}
	if d, ok := castDesireOf(priest, ind4bBuffSkill); ok {
		t.Fatalf("buff desire at the look while attacking = %+v, want none", d)
	}
}
