package ai

import (
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"testing"
	"time"

	gamexml "github.com/fatal10110/acis_golang/internal/gameserver/data/xml"
	actorai "github.com/fatal10110/acis_golang/internal/gameserver/model/actor/ai"
	"github.com/fatal10110/acis_golang/internal/gameserver/model/actor/npc"
	"github.com/fatal10110/acis_golang/internal/gameserver/model/location"
	modelskill "github.com/fatal10110/acis_golang/internal/gameserver/model/skill"
	"github.com/fatal10110/acis_golang/internal/gameserver/script"
	"github.com/fatal10110/acis_golang/internal/gameservertest"
	"github.com/rs/zerolog"
)

// The NPC ids the scenarios of the batch's fourth cut spawn (#189 PR 4).
const (
	ind4dPhysicalID = int32(20971) // PartyPrivatePhysicalSpecial
	ind4dHealID     = int32(22122) // PartyPrivatePhysicalSpecialHeal
	ind4dShotID     = int32(20985) // PartyPrivatePhysicalSpecialPowerShot
	ind4dPrimevalID = int32(22204) // PartyPrivatePrimeval
	ind4dZealotID   = int32(22161) // PartyPrivateHereticZealot
	ind4dZealot2ID  = int32(22156) // PartyPrivateHereticZealot2
	ind4dBabyID     = int32(22120) // PartyPrivateCoupleFollowerBabyAggressive
	ind4dFemaleID   = int32(22108) // PartyPrivateCoupleFollowerFemaleAggressive
)

// ind4dSelfRangeSkill is the self range physical special skill the cut's
// fixture NPCs list by type beside ind4bTemplate's.
const ind4dSelfRangeSkill = modelskill.ID(9717)

// ind4dTemplate is ind4bTemplate listing also a self range physical
// special skill by type.
func ind4dTemplate(id int32, clan bool) *npc.Template {
	tmpl := ind4bTemplate(id, clan)
	tmpl.SkillsByType[npc.SkillTypeSelfRangePhysicalSpecial] = modelskill.Ref{ID: ind4dSelfRangeSkill, Level: 1}
	return tmpl
}

// ind4dSkills are ind4bSkills with the self range physical special skill,
// a self-centred attack.
func ind4dSkills() *modelskill.Table {
	return modelskill.NewTable(append(ind4bSkills().All(), ind1Skill(ind4dSelfRangeSkill, 0, true)))
}

// ind4dTop sets h's top desire target to the character, or empties it,
// and drops its queued desires.
func (w *ind1World) ind4dTop(t *testing.T, h *npc.Hostile, on bool) {
	t.Helper()
	w.clearDesires(t, h)
	w.ind4cTopDesireTarget(t, h, on)
}

// TestPartyPrivatePhysicalSpecialStrikesItsTopDesireTarget ports
// PartyPrivatePhysicalSpecial's attacked hook. A hit by a player that is
// not the creature it most wants to act on wants nothing; a hit by that
// creature, on a draw under 33, wants its physical special skill on it
// with weight 1000000, and a draw of 33 wants none.
func TestPartyPrivatePhysicalSpecialStrikesItsTopDesireTarget(t *testing.T) {
	t.Parallel()
	w := bootIndividual4(t, raidBoot{}, ind4bTemplate(ind4dPhysicalID, false))
	n := w.spawn(t, ind4dPhysicalID, 300, 0)
	w.srv.InstallNPCCastSeam(t, n, ind4bSkills())
	me := w.player.ObjectID()

	w.ind4dTop(t, n, false)
	w.hit(t, n, 1)
	w.wantCasts(t, "after a hit by a player not its top desire target", n, nil)
	w.ind4dTop(t, n, true)
	w.hit(t, n, 1)
	w.wantCasts(t, "after a hit by its top desire target", n, map[modelskill.ID]ind1Cast{ind4bSpecialSkill: {me, 1000000}})
	w.ind4dTop(t, n, true)
	w.draws.set(33)
	w.hit(t, n, 1)
	w.wantCasts(t, "after a hit with a draw of 33", n, nil)
}

// TestPartyPrivatePhysicalSpecialAnswersItsParty ports
// PartyPrivatePhysicalSpecial's party-attacked hook, every draw 0: a
// settled private that is not attacking wants its physical special skill
// on its master's attacker with weight 1000000; a young one, and a settled
// one that is attacking, want nothing.
func TestPartyPrivatePhysicalSpecialAnswersItsParty(t *testing.T) {
	t.Parallel()
	for _, row := range []struct {
		name      string
		settle    int
		attacking bool
		strike    bool
	}{{"settled", 8, false, true}, {"young", 0, false, false}, {"attacking", 8, true, false}} {
		t.Run(row.name, func(t *testing.T) {
			t.Parallel()
			w := bootIndividual4(t, raidBoot{}, ind4Leader(ind4dPhysicalID, 1), ind4bTemplate(ind4dPhysicalID, false))
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

// TestPartyPrivatePhysicalSpecialHealHealsItsParty ports
// PartyPrivatePhysicalSpecialHeal's attacked and party-attacked hooks for
// two privates of one master, every draw 0 (the template lists no physical
// special skill, so the parent's strikes want nothing). The hooks run
// before the hit takes its HP. A hit on a private at 40% of its HP makes
// it want its heal on itself, and the other private its heal on it, weight
// 1000000 each; its own party call heals nobody, as it is the caller. A
// hit on a private at full HP while their master stands at 40% makes both
// privates want their heal on the master, the one hit through its own
// party call; a draw of 33 wants none.
func TestPartyPrivatePhysicalSpecialHealHealsItsParty(t *testing.T) {
	t.Parallel()
	w := bootIndividual4(t, raidBoot{}, ind4Leader(ind4dHealID, 2), ind4Template(ind4dHealID, false))
	leader := w.spawn(t, leaderID, 300, 0)
	privates := privatesOf(t, leader, 2)
	victim, healer := privates[0], privates[1]
	for _, h := range privates {
		w.srv.InstallNPCCastSeam(t, h, ind4Skills())
	}
	w.hit(t, victim, int(victim.MaxHPValue()*60/100))
	w.clearDesires(t, victim)
	w.clearDesires(t, healer)
	w.wantCasts(t, "setup", healer, nil)
	w.hit(t, victim, 1)
	vid := victim.ObjectID()
	w.wantCasts(t, "victim after a hit at 40%", victim, map[modelskill.ID]ind1Cast{ind4HealSkill: {vid, 1000000}})
	w.wantCasts(t, "other private after a hit on the victim at 40%", healer, map[modelskill.ID]ind1Cast{ind4HealSkill: {vid, 1000000}})

	w.hit(t, leader, int(leader.MaxHPValue()*60/100))
	w.clearDesires(t, victim)
	w.clearDesires(t, healer)
	w.hit(t, healer, 1)
	lid := leader.ObjectID()
	heal := map[modelskill.ID]ind1Cast{ind4HealSkill: {lid, 1000000}}
	w.wantCasts(t, "victim after a hit on the other private with their master at 40%", victim, heal)
	w.wantCasts(t, "other private after a hit on itself with its master at 40%", healer, heal)

	w.clearDesires(t, victim)
	w.clearDesires(t, healer)
	w.draws.set(33)
	w.hit(t, healer, 1)
	w.wantCasts(t, "victim after a hit with a draw of 33", victim, nil)
	w.wantCasts(t, "other private after a hit with a draw of 33", healer, nil)
}

// TestPartyPrivatePhysicalSpecialPowerShotRunsFromCloseAttackers ports
// PartyPrivatePhysicalSpecialPowerShot's created, attacked and timer
// hooks. It spawns with i_ai2 clear. A hit from 50 sets i_ai2, keeps the
// attacker and arms the 2 s timer "100002", whose firing makes it want to
// run from that attacker its maximum drift range, weight 10000, and clears
// i_ai2. A hit from 300 leaves i_ai2 clear and arms nothing.
func TestPartyPrivatePhysicalSpecialPowerShotRunsFromCloseAttackers(t *testing.T) {
	t.Parallel()
	w := bootIndividual4(t, raidBoot{}, ind4Template(ind4dShotID, false))
	far := w.spawn(t, ind4dShotID, 300, 0)
	w.hit(t, far, 1)
	if got := ind1Slot(far, script.SlotIntAI2); got != 0 {
		t.Fatalf("i_ai2 after a hit from 300 = %d, want 0", got)
	}

	near := w.spawn(t, ind4dShotID, 50, 0)
	if got := ind1Slot(near, script.SlotIntAI2); got != 0 {
		t.Fatalf("created i_ai2 = %d, want 0", got)
	}
	w.hit(t, near, 1)
	if got := ind1Slot(near, script.SlotIntAI2); got != 1 {
		t.Fatalf("i_ai2 after a hit from 50 = %d, want 1", got)
	}
	me := w.player.ObjectID()
	if c := script.NPCOf(near).Slot(script.SlotCreature1); c == nil || c.ObjectID() != me {
		t.Fatalf("kept attacker = %v, want the character", c)
	}
	if d, ok := desire(near, actorai.IntentionFlee); ok {
		t.Fatalf("flee desire before the timer = %+v, want none", d)
	}
	drift := script.NPCOf(near).MaxDriftRange()
	w.srv.Advance(t, 2*time.Second)
	d, ok := desire(near, actorai.IntentionFlee)
	if !ok || d.Weight != 10000 || d.Distance != drift || d.FinalTarget == nil || d.FinalTarget.ObjectID() != me {
		t.Fatalf("flee desire after the timer = %+v (%v), want a %d run from the character, weight 10000", d, ok, drift)
	}
	if got := ind1Slot(near, script.SlotIntAI2); got != 0 {
		t.Fatalf("i_ai2 after the timer = %d, want 0", got)
	}
	if d, ok := desire(far, actorai.IntentionFlee); ok {
		t.Fatalf("flee desire of the one hit from 300 = %+v, want none", d)
	}
}

// ind4dPrimevalTemplate is a fixture PartyPrivatePrimeval listening to
// lair shouts when listen is set, aggressive when aggressive is set.
func ind4dPrimevalTemplate(listen, aggressive bool) *npc.Template {
	tmpl := ind4Template(ind4dPrimevalID, false)
	flag := func(on bool) string {
		if on {
			return "1"
		}
		return "0"
	}
	tmpl.AIParams = npc.AIParams{"BroadCastReception": flag(listen), "ag_type": flag(aggressive)}
	return tmpl
}

// TestPartyPrivatePrimevalAnswersLairShouts ports PartyPrivatePrimeval's
// script-event hook: event 10016 naming the character drops its attack
// desires and wants to attack the character with weight 100 when its
// BroadCastReception AI parameter is 1, and wants nothing when it is 0.
func TestPartyPrivatePrimevalAnswersLairShouts(t *testing.T) {
	t.Parallel()
	for _, row := range []struct {
		name   string
		listen bool
	}{{"listening", true}, {"deaf", false}} {
		t.Run(row.name, func(t *testing.T) {
			t.Parallel()
			w := bootIndividual4(t, raidBoot{}, ind4dPrimevalTemplate(row.listen, false))
			n := w.spawn(t, ind4dPrimevalID, 300, 0)
			me := w.player.ObjectID()
			w.clearDesires(t, n)
			w.sendEvent(t, n, 10016, me, 0)
			if !row.listen {
				if d, ok := desire(n, actorai.IntentionAttack); ok {
					t.Fatalf("attack desire after a shout it does not listen to = %+v, want none", d)
				}
				return
			}
			wantDesire(t, "after the shout", n, actorai.IntentionAttack, me, 100)
		})
	}
}

// TestPartyPrivatePrimevalAttacksOnSightWhenAggressive ports
// PartyPrivatePrimeval's see-creature hook: with its ag_type AI parameter
// 1 it turns on the character it sees (hate 200); with ag_type 0 it
// ignores the character.
func TestPartyPrivatePrimevalAttacksOnSightWhenAggressive(t *testing.T) {
	t.Parallel()
	for _, row := range []struct {
		name       string
		aggressive bool
		hate       float64
	}{{"aggressive", true, 200}, {"passive", false, 0}} {
		t.Run(row.name, func(t *testing.T) {
			t.Parallel()
			w := bootIndividual4(t, raidBoot{}, ind4dPrimevalTemplate(false, row.aggressive))
			n := w.spawn(t, ind4dPrimevalID, 150, 0)
			w.ticks(t, 1)
			if got := n.AggroHate(w.player); got != row.hate {
				t.Fatalf("hate of the character it sees = %v, want %v", got, row.hate)
			}
		})
	}
}

// TestPartyPrivatePrimevalFollowsItsMasterOrWalksHome ports
// PartyPrivatePrimeval's no-desire hook: wanting nothing, a private of a
// live master wants to follow it with weight 5; once its master is dead,
// standing away from its spawn point, it wants to walk there with weight
// 30.
func TestPartyPrivatePrimevalFollowsItsMasterOrWalksHome(t *testing.T) {
	t.Parallel()
	w := bootIndividual4(t, raidBoot{}, ind4Leader(ind4dPrimevalID, 1), ind4dPrimevalTemplate(false, false))
	leader := w.spawn(t, leaderID, 1500, 0)
	private := privatesOf(t, leader, 1)[0]
	w.clearDesires(t, private)
	w.ticks(t, 2)
	wantDesire(t, "wanting nothing beside its live master", private, actorai.IntentionFollow, leader.ObjectID(), 5)

	w.kill(t, leader)
	home := script.NPCOf(private).SpawnLocation()
	w.teleport(t, private, 200, 0)
	w.clearDesires(t, private)
	w.ticks(t, 2)
	if d, ok := desire(private, actorai.IntentionMoveTo); !ok || d.Weight != 30 || d.Location.X != home.X || d.Location.Y != home.Y {
		t.Fatalf("move desire with its master dead = %+v (%v), want weight 30 to its spawn point %d,%d", d, ok, home.X, home.Y)
	}
	if d, ok := desire(private, actorai.IntentionFollow); ok {
		t.Fatalf("follow desire with its master dead = %+v, want none", d)
	}
}

// TestPartyPrivateHereticZealotDiesStrikingForItsMaster ports
// PartyPrivateHereticZealot's party-died and skill-finished hooks for two
// zealots of one master. The other zealot's death changes nothing. Its
// master's death sets i_ai0 and makes it want its second physical special
// skill on the creature it most wants to act on, the character, with
// weight 1000000; it casts it (MagicSkillUse) and, the cast finished,
// dies.
func TestPartyPrivateHereticZealotDiesStrikingForItsMaster(t *testing.T) {
	t.Parallel()
	w := bootIndividual4(t, raidBoot{}, ind4Leader(ind4dZealotID, 2), ind4Template(ind4dZealotID, false))
	leader := w.spawn(t, leaderID, 300, 0)
	privates := privatesOf(t, leader, 2)
	zealot, other := privates[0], privates[1]
	w.srv.InstallNPCCastSeam(t, zealot, ind4Skills())
	me := w.player.ObjectID()
	w.hit(t, zealot, 10)
	w.ticks(t, 1)
	if top := zealot.AI().TopDesireTarget(); top == nil || top.ObjectID() != me {
		t.Fatalf("setup: top desire target = %v, want the character", top)
	}

	w.clearDesires(t, zealot)
	w.kill(t, other)
	if got := ind1Slot(zealot, script.SlotIntAI0); got != 0 {
		t.Fatalf("i_ai0 after the other zealot died = %d, want 0", got)
	}
	w.wantCasts(t, "after the other zealot died", zealot, nil)

	w.kill(t, leader)
	if got := ind1Slot(zealot, script.SlotIntAI0); got != 1 {
		t.Fatalf("i_ai0 after its master died = %d, want 1", got)
	}
	w.wantCasts(t, "after its master died", zealot, map[modelskill.ID]ind1Cast{ind4Special2Skill: {me, 1000000}})

	frames := w.ticks(t, 1)
	w.srv.AdvanceUntil(t, "the zealot dying after its strike", zealot.Dead)
	frames = append(frames, w.srv.ReadQueued(t, w.srv.Client)...)
	if got := ind4bCastsOf(frames, zealot, ind4Special2Skill); len(got) != 1 || got[0] != me {
		t.Fatalf("second physical special casts before it died = %v, want one on the character", got)
	}
}

// TestPartyPrivateHereticZealotWalksHomeFromOutside ports the
// out-of-territory hook PartyPrivateHereticZealot and
// PartyPrivateHereticZealot2 share with PartyPrivateHereticCastCurse, on
// one their npcmaker spawned: arriving outside its territory with i_ai4
// clear, it wants to walk to its spawn point with weight 100.
func TestPartyPrivateHereticZealotWalksHomeFromOutside(t *testing.T) {
	t.Parallel()
	for _, id := range []int32{ind4dZealotID, ind4dZealot2ID} {
		t.Run(strconv.Itoa(int(id)), func(t *testing.T) {
			t.Parallel()
			dir := t.TempDir()
			den := strings.Replace(ind4HereticDen, `npc id="22157"`, `npc id="`+strconv.Itoa(int(id))+`"`, 1)
			if err := os.WriteFile(filepath.Join(dir, "den.xml"), []byte(den), 0o644); err != nil {
				t.Fatalf("write spawnlist: %v", err)
			}
			table, err := gamexml.LoadSpawnlist(dir, zerolog.Nop(), 1)
			if err != nil {
				t.Fatalf("load spawnlist: %v", err)
			}
			w := bootIndividual4(t, raidBoot{opts: []gameservertest.Option{gameservertest.WithNpcSpawns(table)}}, ind4Template(id, false))
			var zealot *npc.Hostile
			for _, obj := range w.srv.State.Objects() {
				if h, ok := obj.(*npc.Hostile); ok && h.NpcID() == int(id) {
					zealot = h
				}
			}
			if zealot == nil {
				t.Fatal("the den spawned no zealot")
			}
			home := script.NPCOf(zealot).SpawnLocation()
			ind4WalkTo(t, w, zealot, location.Location{X: 40, Y: 700, Z: 30})
			if zealot.InTerritory() {
				t.Fatal("setup: the walk ended inside the territory")
			}
			if d, ok := desire(zealot, actorai.IntentionMoveTo); !ok || d.Weight != 100 || d.Location.X != home.X || d.Location.Y != home.Y {
				t.Fatalf("move desire after arriving outside = %+v (%v), want weight 100 to its spawn point %d,%d", d, ok, home.X, home.Y)
			}
		})
	}
}

// TestPartyPrivateHereticZealot2StrikesOnceItsMasterIsDead ports
// PartyPrivateHereticZealot2's created, attacked, party-attacked,
// script-event and party-died hooks for two zealots of one master, every
// draw 0. It spawns with i_ai0 and i_ai4 clear, and a hit while it
// neither attacks nor casts sets i_ai4; event 10035 from its master clears
// it again. While its master lives a hit wants no strike. Its master's
// death sets i_ai0 and wants its second physical special skill on the
// character it fights, weight 1000000. From then on a hit by the
// character wants that skill on the creature it most wants to act on, or
// on the character when there is none. A hit on the other zealot with
// i_ai0 set and no top desire target wants it on the character on a draw
// under 33, and a draw of 33 wants none.
func TestPartyPrivateHereticZealot2StrikesOnceItsMasterIsDead(t *testing.T) {
	t.Parallel()
	w := bootIndividual4(t, raidBoot{}, ind4Leader(ind4dZealot2ID, 2), ind4Template(ind4dZealot2ID, false))
	leader := w.spawn(t, leaderID, 300, 0)
	privates := privatesOf(t, leader, 2)
	zealot, other := privates[0], privates[1]
	w.srv.InstallNPCCastSeam(t, zealot, ind4Skills())
	me, oid := w.player.ObjectID(), other.ObjectID()
	if a0, a4 := ind1Slot(zealot, script.SlotIntAI0), ind1Slot(zealot, script.SlotIntAI4); a0 != 0 || a4 != 0 {
		t.Fatalf("created i_ai0, i_ai4 = %d, %d, want 0, 0", a0, a4)
	}
	w.hit(t, zealot, 10)
	if got := ind1Slot(zealot, script.SlotIntAI4); got != 1 {
		t.Fatalf("i_ai4 after a hit while idle = %d, want 1", got)
	}
	w.sendEvent(t, zealot, 10035, 0, leader.ObjectID())
	if got := ind1Slot(zealot, script.SlotIntAI4); got != 0 {
		t.Fatalf("i_ai4 after event 10035 = %d, want 0", got)
	}
	w.wantCasts(t, "after a hit with its master alive", zealot, nil)

	// Its master's death lets its privates go, so their party calls end
	// with it: the party-attacked strike is driven with i_ai0 set by hand.
	w.onQueue(t, zealot, func() { script.NPCOf(zealot).SetInt(script.SlotIntAI0, 1) })
	w.ind4dTop(t, zealot, false)
	w.hit(t, other, 1)
	w.wantCasts(t, "after a hit on the other zealot with i_ai0 set", zealot, map[modelskill.ID]ind1Cast{ind4Special2Skill: {me, 1000000}})
	w.ind4dTop(t, zealot, false)
	w.draws.set(33)
	w.hit(t, other, 1)
	w.wantCasts(t, "after a hit on the other zealot with a draw of 33", zealot, nil)
	w.draws.set(0)
	w.onQueue(t, zealot, func() { script.NPCOf(zealot).SetInt(script.SlotIntAI0, 0) })

	w.hit(t, zealot, 10)
	w.ticks(t, 1)
	w.clearDesires(t, zealot)
	w.kill(t, leader)
	if got := ind1Slot(zealot, script.SlotIntAI0); got != 1 {
		t.Fatalf("i_ai0 after its master died = %d, want 1", got)
	}
	w.wantCasts(t, "after its master died", zealot, map[modelskill.ID]ind1Cast{ind4Special2Skill: {me, 1000000}})

	w.clearDesires(t, zealot)
	w.onQueue(t, zealot, func() { script.NPCOf(zealot).SetTopDesireTarget(script.NPCOf(other)) })
	w.hit(t, zealot, 1)
	w.wantCasts(t, "after a hit with the other zealot its top desire target", zealot, map[modelskill.ID]ind1Cast{ind4Special2Skill: {oid, 1000000}})
	w.ind4dTop(t, zealot, false)
	w.hit(t, zealot, 1)
	w.wantCasts(t, "after a hit with no top desire target", zealot, map[modelskill.ID]ind1Cast{ind4Special2Skill: {me, 1000000}})
}

// TestPartyPrivateCoupleFollowerBabyAggressiveStrikesAroundItself ports
// PartyPrivateCoupleFollowerBabyAggressive's created, see-creature and
// attacked hooks, every draw 0. It spawns wanting its buff on itself
// (1000000) and turns on the character it sees (hate 200). A hit by the
// creature it most wants to act on wants its self range physical special
// skill on itself with weight 1000000; a hit by another player wants
// nothing, and so does a draw of 33.
func TestPartyPrivateCoupleFollowerBabyAggressiveStrikesAroundItself(t *testing.T) {
	t.Parallel()
	w := bootIndividual4(t, raidBoot{opts: []gameservertest.Option{gameservertest.WithNpcSpawnSkills(ind4dSkills())}}, ind4dTemplate(ind4dBabyID, false))
	n := w.spawn(t, ind4dBabyID, 150, 0)
	nid := n.ObjectID()
	w.wantCasts(t, "as it spawns", n, map[modelskill.ID]ind1Cast{ind4bBuffSkill: {nid, 1000000}})
	w.ticks(t, 1)
	if got := n.AggroHate(w.player); got != 200 {
		t.Fatalf("hate of the character it sees = %v, want 200", got)
	}

	w.ind4dTop(t, n, false)
	w.hit(t, n, 1)
	w.wantCasts(t, "after a hit by a player not its top desire target", n, nil)
	w.ind4dTop(t, n, true)
	w.hit(t, n, 1)
	w.wantCasts(t, "after a hit by its top desire target", n, map[modelskill.ID]ind1Cast{ind4dSelfRangeSkill: {nid, 1000000}})
	w.ind4dTop(t, n, true)
	w.draws.set(33)
	w.hit(t, n, 1)
	w.wantCasts(t, "after a hit with a draw of 33", n, nil)
}

// TestPartyPrivateCoupleFollowerBabyAggressiveAnswersItsClan ports
// PartyPrivateCoupleFollowerBabyAggressive's clan-attacked hook, every
// draw 0: a settled clan member that is not attacking wants its self range
// physical special skill on itself with weight 1000000; a young one wants
// nothing.
func TestPartyPrivateCoupleFollowerBabyAggressiveAnswersItsClan(t *testing.T) {
	t.Parallel()
	for _, row := range []struct {
		name   string
		settle int
		strike bool
	}{{"settled", 8, true}, {"young", 0, false}} {
		t.Run(row.name, func(t *testing.T) {
			t.Parallel()
			w := bootIndividual4(t, raidBoot{}, ind4dTemplate(ind4dBabyID, true))
			victim := w.spawn(t, ind4dBabyID, 1500, 0)
			member := w.spawn(t, ind4dBabyID, 1500, 200)
			w.srv.InstallNPCCastSeam(t, member, ind4dSkills())
			w.ticks(t, row.settle)
			w.clearDesires(t, member)
			w.hit(t, victim, 10)
			want := map[modelskill.ID]ind1Cast{}
			if row.strike {
				want[ind4dSelfRangeSkill] = ind1Cast{member.ObjectID(), 1000000}
			}
			w.wantCasts(t, "member after the hit", member, want)
		})
	}
}

// TestPartyPrivateCoupleFollowerFemaleAggressiveFightsWithThreeSkills
// ports PartyPrivateCoupleFollowerFemaleAggressive's attacked hook, every
// draw 0. A hit by the creature it most wants to act on wants its debuff
// and physical special skill on the attacker and its buff on itself,
// weight 1000000 each; under half its HP it wants no buff. A hit by
// another player wants nothing.
func TestPartyPrivateCoupleFollowerFemaleAggressiveFightsWithThreeSkills(t *testing.T) {
	t.Parallel()
	w := bootIndividual4(t, raidBoot{}, ind4dTemplate(ind4dFemaleID, false))
	n := w.spawn(t, ind4dFemaleID, 1500, 0)
	w.srv.InstallNPCCastSeam(t, n, ind4dSkills())
	me, nid := w.player.ObjectID(), n.ObjectID()

	w.ind4dTop(t, n, false)
	w.hit(t, n, 1)
	w.wantCasts(t, "after a hit by a player not its top desire target", n, nil)
	w.ind4dTop(t, n, true)
	w.hit(t, n, 1)
	w.wantCasts(t, "after a hit by its top desire target", n, map[modelskill.ID]ind1Cast{
		ind4DebuffSkill: {me, 1000000}, ind4bSpecialSkill: {me, 1000000}, ind4bBuffSkill: {nid, 1000000},
	})
	w.hit(t, n, int(n.MaxHPValue()*60/100))
	w.ind4dTop(t, n, true)
	w.hit(t, n, 1)
	w.wantCasts(t, "after a hit under half its HP", n, map[modelskill.ID]ind1Cast{
		ind4DebuffSkill: {me, 1000000}, ind4bSpecialSkill: {me, 1000000},
	})
}

// TestPartyPrivateCoupleFollowerFemaleAggressiveAnswersItsClanAndParty
// ports PartyPrivateCoupleFollowerFemaleAggressive's clan-attacked and
// party-attacked hooks, every draw 0: a settled member that is not
// attacking wants its physical special skill and debuff on the attacker of
// a clan member, and on its master's also its buff on itself, weight
// 1000000 each. A young one wants nothing.
func TestPartyPrivateCoupleFollowerFemaleAggressiveAnswersItsClanAndParty(t *testing.T) {
	t.Parallel()
	for _, row := range []struct {
		name   string
		settle int
		strike bool
	}{{"settled", 8, true}, {"young", 0, false}} {
		t.Run("clan/"+row.name, func(t *testing.T) {
			t.Parallel()
			w := bootIndividual4(t, raidBoot{}, ind4dTemplate(ind4dFemaleID, true))
			victim := w.spawn(t, ind4dFemaleID, 1500, 0)
			member := w.spawn(t, ind4dFemaleID, 1500, 200)
			w.srv.InstallNPCCastSeam(t, member, ind4dSkills())
			w.ticks(t, row.settle)
			w.clearDesires(t, member)
			w.hit(t, victim, 10)
			want := map[modelskill.ID]ind1Cast{}
			if row.strike {
				me := w.player.ObjectID()
				want[ind4bSpecialSkill] = ind1Cast{me, 1000000}
				want[ind4DebuffSkill] = ind1Cast{me, 1000000}
			}
			w.wantCasts(t, "member after the hit", member, want)
		})
		t.Run("party/"+row.name, func(t *testing.T) {
			t.Parallel()
			w := bootIndividual4(t, raidBoot{}, ind4Leader(ind4dFemaleID, 1), ind4dTemplate(ind4dFemaleID, false))
			leader := w.spawn(t, leaderID, 1500, 0)
			private := privatesOf(t, leader, 1)[0]
			w.srv.InstallNPCCastSeam(t, private, ind4dSkills())
			w.ticks(t, row.settle)
			w.clearDesires(t, private)
			w.hit(t, leader, 10)
			want := map[modelskill.ID]ind1Cast{}
			if row.strike {
				me := w.player.ObjectID()
				want[ind4bSpecialSkill] = ind1Cast{me, 1000000}
				want[ind4DebuffSkill] = ind1Cast{me, 1000000}
				want[ind4bBuffSkill] = ind1Cast{private.ObjectID(), 1000000}
			}
			w.wantCasts(t, "private after the hit on its master", private, want)
		})
	}
}
