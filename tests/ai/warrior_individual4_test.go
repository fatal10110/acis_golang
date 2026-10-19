package ai

import (
	"fmt"
	"slices"
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

// The fourth individual behavior batch's NPC ids the scenarios spawn (#189
// PR 1), each a fixture monster of level 1 unless a scenario says
// otherwise.
const (
	ind4FreyaBearID  = int32(22103) // FreyaBear
	ind4RaikelID     = int32(29016) // OrfenPrivateRaikelLeos
	ind4CannibaliID  = int32(22112) // PartyPrivateCannibaliAggressive
	ind4Cast3ID      = int32(29993) // PartyPrivateCast3SkillsMagical, bound by the test
	ind4Cast3TwoID   = int32(29994) // PartyPrivateCast3SkillsMagical, bound by the test
	ind4CurseID      = int32(20982) // PartyPrivateCastCurse
	ind4HereticID    = int32(22157) // PartyPrivateHereticCastCurse
	ind4DDHealID     = int32(20948) // PartyPrivateCastDDHeal
	ind4StoneID      = int32(21349) // PartyPrivateCastDDMagicCurseStone
	ind4Cast3Path    = "test.PartyPrivateCast3SkillsMagical"
	ind4MortalBlowID = modelskill.ID(4067) // OrfenPrivateRaikelLeos's mortal blow, level 4
)

// The skills the batch's fixture NPCs list by type.
const (
	ind4DDMagicSkill   = modelskill.ID(9701)
	ind4DDMagic1Skill  = modelskill.ID(9702)
	ind4DDMagic2Skill  = modelskill.ID(9703)
	ind4DebuffSkill    = modelskill.ID(9704)
	ind4HealSkill      = modelskill.ID(9705)
	ind4Special1Skill  = modelskill.ID(9706)
	ind4Special2Skill  = modelskill.ID(9707)
	ind4PrivateRespawn = 240 * time.Second
)

// ind4Batch are the batch's classes, in scripts.xml order.
var ind4Batch = []struct {
	path string
	ctor func() script.Script
}{
	{"script.ai.individual.Monster.WarriorBase.Warrior.PartyPrivateWarrior.FreyaBear", warrior.FreyaBear},
	{"script.ai.individual.Monster.WarriorBase.Warrior.PartyPrivateWarrior.OrfenPrivateRaikelLeos", warrior.OrfenPrivateRaikelLeos},
	{"script.ai.individual.Monster.WarriorBase.Warrior.PartyPrivateWarrior.PartyPrivateCannibaliAggressive", warrior.PartyPrivateCannibaliAggressive},
	{"script.ai.individual.Monster.WarriorBase.Warrior.PartyPrivateWarrior.PartyPrivateCast3SkillsMagical", warrior.PartyPrivateCast3SkillsMagical},
	{"script.ai.individual.Monster.WarriorBase.Warrior.PartyPrivateWarrior.PartyPrivateCastCurse", warrior.PartyPrivateCastCurse},
	{"script.ai.individual.Monster.WarriorBase.Warrior.PartyPrivateWarrior.PartyPrivateCastDDHeal", warrior.PartyPrivateCastDDHeal},
	{"script.ai.individual.Monster.WarriorBase.Warrior.PartyPrivateWarrior.PartyPrivateCastDDMagicCurseStone", warrior.PartyPrivateCastDDMagicCurseStone},
	{"script.ai.individual.Monster.WarriorBase.Warrior.PartyPrivateWarrior.PartyPrivateHereticCastCurse", warrior.PartyPrivateHereticCastCurse},
	// PartyPrivateCast3SkillsMagical binds no NPC of its own; the
	// scenarios bind its hooks to fixture ids to drive them.
	{ind4Cast3Path, func() script.Script {
		s := warrior.PartyPrivateCast3SkillsMagical()
		s.NPCs = []int32{ind4Cast3ID, ind4Cast3TwoID}
		return s
	}},
}

// bootIndividual4 boots the batch after the party Warriors beside
// templates, with extra's options added last and its step run before the
// character enters the world, every draw 0 until a scenario queues others,
// with the director script sendEvent sends through.
func bootIndividual4(t *testing.T, extra raidBoot, templates ...*npc.Template) *ind1World {
	t.Helper()
	list, catalog := partyScripts()
	for _, c := range ind4Batch {
		list = append(list, script.Listing{Path: c.path})
		catalog[c.path] = c.ctor
	}
	const director = "test.Ind1Director"
	catalog[director] = func() script.Script { return script.Script{} }
	list = append(list, script.Listing{Path: director})
	kinds := map[int32]script.NPCKind{}
	for _, tmpl := range templates {
		kinds[int32(tmpl.ID)] = script.KindHostile
	}
	draws := &ind1Draws{}
	opts := append([]gameservertest.Option{
		gameservertest.WithScriptRand(draws.rand),
		gameservertest.WithNPCScripts(kinds, list, catalog),
	}, extra.opts...)
	w := bootRaid(t, raidBoot{opts: opts, ready: extra.ready}, templates...)
	return &ind1World{warriorWorld: w, draws: draws, heard: &ind1Heard{}}
}

// ind4Template is a fixture batch NPC of id, listing by type its DD
// magics, debuff, heal and physical specials; with clan set, in the clan
// the scenario's NPCs share.
func ind4Template(id int32, clan bool) *npc.Template {
	tmpl := warriorTemplate(id, nil)
	tmpl.SkillsByType = map[npc.SkillType]modelskill.Ref{
		npc.SkillTypeDDMagic:          {ID: ind4DDMagicSkill, Level: 1},
		npc.SkillTypeDDMagic1:         {ID: ind4DDMagic1Skill, Level: 1},
		npc.SkillTypeDDMagic2:         {ID: ind4DDMagic2Skill, Level: 1},
		npc.SkillTypeDebuff:           {ID: ind4DebuffSkill, Level: 1},
		npc.SkillTypeMagicHeal:        {ID: ind4HealSkill, Level: 1},
		npc.SkillTypePhysicalSpecial1: {ID: ind4Special1Skill, Level: 1},
		npc.SkillTypePhysicalSpecial2: {ID: ind4Special2Skill, Level: 1},
	}
	if clan {
		tmpl.Clans, tmpl.ClanRange = []string{"party"}, 500
	}
	return tmpl
}

// ind4Leader is a fixture PartyLeaderWarrior that neither walks nor
// wanders far, declaring privates of the fixture privateID.
func ind4Leader(privateID int32, privates int) *npc.Template {
	tmpl := warriorTemplate(leaderID, nil)
	tmpl.CanMove = false
	for range privates {
		tmpl.Privates = append(tmpl.Privates, npc.PrivateEntry{NpcID: int(privateID), Weight: 1, RespawnDelay: ind4PrivateRespawn})
	}
	return tmpl
}

// ind4Skills are the batch NPCs' skills: the DD magics, debuff and
// physical specials one-target attacks reaching 2000, the heal a
// one-target spell on a friend.
func ind4Skills() *modelskill.Table {
	heal := ind1Skill(ind4HealSkill, 2000, false)
	heal.Offensive, heal.SkillType, heal.Power = false, "HEAL", 0
	return modelskill.NewTable([]modelskill.Definition{
		ind1Skill(ind4DDMagicSkill, 2000, false), ind1Skill(ind4DDMagic1Skill, 2000, false),
		ind1Skill(ind4DDMagic2Skill, 2000, false), ind1Skill(ind4DebuffSkill, 2000, false),
		ind1Skill(ind4Special1Skill, 2000, false), ind1Skill(ind4Special2Skill, 2000, false), heal,
	})
}

// wantDesire fails unless h has a queued desire of kind on the creature of
// the object id target, weight as given.
func wantDesire(t *testing.T, when string, h *npc.Hostile, kind actorai.Intention, target int32, weight float64) {
	t.Helper()
	d, ok := desire(h, kind)
	if !ok || d.FinalTarget == nil || d.FinalTarget.ObjectID() != target || d.Weight != weight {
		t.Fatalf("%s: %v desire = %+v (%v), want weight %v on %d", when, kind, d, ok, weight, target)
	}
}

// TestFreyaBearAnswersItsMastersEvents ports FreyaBear's script-event hook.
// Event 10002 from a creature other than its master is ignored. From its
// master it says the line its draw picks, 1000402 for a draw of 3, drops
// all its hate and attack desires and wants to attack the creature its
// master flagged with weight 1000000. With that creature its top desire
// target, the event says nothing. Event 10034 makes it want to heal its
// master, and 11039 takes it out of the world (DeleteObject).
func TestFreyaBearAnswersItsMastersEvents(t *testing.T) {
	t.Parallel()
	w := bootIndividual4(t, raidBoot{}, ind4Leader(ind4FreyaBearID, 1), ind4Template(ind4FreyaBearID, false))
	leader := w.spawn(t, leaderID, 400, 0)
	bear := privatesOf(t, leader, 1)[0]
	w.srv.InstallNPCCastSeam(t, bear, ind4Skills())
	me := w.player.ObjectID()
	w.onQueue(t, leader, func() { script.NPCOf(leader).SetInt(script.SlotFlag, me) })
	w.clearDesires(t, bear)

	w.sendEvent(t, bear, 10002, me, 0)
	if got := says(w.srv.ReadQueued(t, w.srv.Client), bear.ObjectID()); len(got) != 0 {
		t.Fatalf("lines on an event from another creature = %q, want none", got)
	}
	if d, ok := desire(bear, actorai.IntentionAttack); ok {
		t.Fatalf("attack desire on an event from another creature = %+v, want none", d)
	}

	// A queued attack desire on the character, which the bear has not taken
	// up: its top desire target stays empty.
	w.onQueue(t, bear, func() {
		n := script.NPCOf(bear)
		n.AddAttackDesire(script.PlayerOf(w.player), 50)
		n.SetTopDesireTarget(nil)
	})
	w.draws.set(0, 3)
	w.sendEvent(t, bear, 10002, leader.ObjectID(), 0)
	if got := says(w.srv.ReadQueued(t, w.srv.Client), bear.ObjectID()); !slices.Equal(got, []string{script.NpcString(1000402)}) {
		t.Fatalf("lines on its master's event = %q, want the 1000402 line", got)
	}
	// The earlier desire of 50 is gone, not added to.
	wantDesire(t, "after its master's event", bear, actorai.IntentionAttack, me, 1000000)

	w.onQueue(t, bear, func() { script.NPCOf(bear).SetTopDesireTarget(script.PlayerOf(w.player)) })
	w.sendEvent(t, bear, 10002, leader.ObjectID(), 0)
	if got := says(w.srv.ReadQueued(t, w.srv.Client), bear.ObjectID()); len(got) != 0 {
		t.Fatalf("lines with the flagged creature its top desire target = %q, want none", got)
	}

	w.sendEvent(t, bear, 10034, 0, 0)
	w.wantCasts(t, "after event 10034", bear, map[modelskill.ID]ind1Cast{ind4HealSkill: {leader.ObjectID(), 1000000}})

	w.sendEvent(t, bear, 11039, 0, 0)
	if got := framesOf(w.srv.ReadQueued(t, w.srv.Client), serverpackets.OpcodeDeleteObject, bear.ObjectID()); len(got) != 1 {
		t.Fatalf("DeleteObject frames on event 11039 = %d, want 1", len(got))
	}
	if w.inWorld(bear) {
		t.Fatal("the bear is still in the world after event 11039")
	}
}

// TestOrfenPrivateRaikelLeosCursesAndMutesOverLevelled ports
// OrfenPrivateRaikelLeos's attacked and see-spell hooks. Against the level
// 20 character, a level 11 Raikel (9 levels below, more than 8) wants its
// raid curse on the character when hit and its raid mute when it sees the
// character cast, both with weight 1000000; a level 12 one wants neither.
// Its attacked hook calls no parent: a hit of level + 7 adds only its own
// party call's hate, (level + 7) * 10, with no draw under 5 for the
// mortal blow.
func TestOrfenPrivateRaikelLeosCursesAndMutesOverLevelled(t *testing.T) {
	t.Parallel()
	for _, row := range []struct {
		level int
		over  bool
	}{{11, true}, {12, false}} {
		t.Run(fmt.Sprintf("level %d", row.level), func(t *testing.T) {
			t.Parallel()
			tmpl := ind4Template(ind4RaikelID, false)
			tmpl.Level, tmpl.CanMove = row.level, false
			w := bootIndividual4(t, raidSpellBoot(t), ind4Leader(ind4RaikelID, 1), tmpl)
			w.draws.set(99)
			leader := w.spawn(t, leaderID, 400, 0)
			raikel := privatesOf(t, leader, 1)[0]
			w.srv.InstallNPCCastSeam(t, raikel, modelskill.NewTable([]modelskill.Definition{
				raidCurseDefinition(), ind1Skill(premoMuteID, 600, false), npcSkill(ind4MortalBlowID, 4, true),
			}))
			me := w.player.ObjectID()
			w.clearDesires(t, raikel)

			w.hit(t, raikel, row.level+7)
			want := map[modelskill.ID]ind1Cast{}
			if row.over {
				want[raidCurseID] = ind1Cast{me, 1000000}
			}
			w.wantCasts(t, "after the hit", raikel, want)
			if got, want := raikel.AggroHate(w.player), float64((row.level+7)*10); got != want {
				t.Fatalf("hate after the hit = %v, want %v from the party call alone", got, want)
			}

			w.castSelfBuff()
			if row.over {
				w.srv.AdvanceUntil(t, "the raid mute desire", func() bool {
					_, ok := ind1Casts(raikel)[premoMuteID]
					return ok
				})
				want[premoMuteID] = ind1Cast{me, 1000000}
			} else {
				w.srv.Advance(t, 2*time.Second)
			}
			w.wantCasts(t, "after the spell", raikel, want)
		})
	}
}

// TestOrfenPrivateRaikelLeosStrikesForItsParty ports
// OrfenPrivateRaikelLeos's party-attacked hook: a hit on its master, with
// a draw under 5, makes it want its mortal blow (4067 level 4) on the
// attacker with weight 1000000; with a draw of 5 it wants none.
func TestOrfenPrivateRaikelLeosStrikesForItsParty(t *testing.T) {
	t.Parallel()
	for _, row := range []struct {
		draw   int
		strike bool
	}{{4, true}, {5, false}} {
		t.Run(fmt.Sprintf("draw %d", row.draw), func(t *testing.T) {
			t.Parallel()
			tmpl := ind4Template(ind4RaikelID, false)
			tmpl.CanMove = false
			w := bootIndividual4(t, raidBoot{}, ind4Leader(ind4RaikelID, 1), tmpl)
			leader := w.spawn(t, leaderID, 400, 0)
			raikel := privatesOf(t, leader, 1)[0]
			w.srv.InstallNPCCastSeam(t, raikel, modelskill.NewTable([]modelskill.Definition{npcSkill(ind4MortalBlowID, 4, true)}))
			w.clearDesires(t, raikel)

			w.draws.set(row.draw)
			w.hit(t, leader, 10)
			want := map[modelskill.ID]ind1Cast{}
			if row.strike {
				want[ind4MortalBlowID] = ind1Cast{w.player.ObjectID(), 1000000}
			}
			w.wantCasts(t, "after the hit on its master", raikel, want)
		})
	}
}

// TestOrfenPrivateRaikelLeosRejoinsItsMaster ports OrfenPrivateRaikelLeos's
// created, timer and party-died hooks. It spawns with i_ai0 its master's
// flag (0) and i_ai1 1, and none of its parents' timers. Every 10 s, with
// its master alive, it stays put while the flag is unchanged and its
// master within 3000. Once the master's flag changes, or the master stands
// more than 3000 away, it jumps to its master (TeleportToLocation), drops
// its attack desires and keeps the new flag. Its master's death clears
// i_ai1, and with no 20 s check it stays in the world, masterless, though
// it neither attacks nor casts.
func TestOrfenPrivateRaikelLeosRejoinsItsMaster(t *testing.T) {
	t.Parallel()
	tmpl := ind4Template(ind4RaikelID, false)
	tmpl.CanMove = false
	w := bootIndividual4(t, raidBoot{}, ind4Leader(ind4RaikelID, 1), tmpl)
	// The master's moves keep both within the character's sight.
	leader := w.spawn(t, leaderID, -1500, 0)
	raikel := privatesOf(t, leader, 1)[0]
	if a0, a1 := ind1Slot(raikel, script.SlotIntAI0), ind1Slot(raikel, script.SlotIntAI1); a0 != 0 || a1 != 1 {
		t.Fatalf("created i_ai0, i_ai1 = %d, %d, want 0, 1", a0, a1)
	}
	teleports := func(when string, want bool) {
		t.Helper()
		frames := w.srv.ReadQueued(t, w.srv.Client)
		if got := len(framesOf(frames, serverpackets.OpcodeTeleportToLocation, raikel.ObjectID())); (got > 0) != want {
			t.Fatalf("%s: TeleportToLocation frames = %d, want them = %v", when, got, want)
		}
		if !want {
			return
		}
		rx, ry, _ := raikel.Position()
		lx, ly, _ := leader.Position()
		if rx != lx || ry != ly {
			t.Fatalf("%s: Raikel at %d,%d, want on its master at %d,%d", when, rx, ry, lx, ly)
		}
		if d, ok := desire(raikel, actorai.IntentionAttack); ok {
			t.Fatalf("%s: attack desire after the jump = %+v, want none", when, d)
		}
	}

	w.srv.Advance(t, 10*time.Second)
	teleports("first tick, flag unchanged", false)

	w.onQueue(t, raikel, func() { script.NPCOf(raikel).AddAttackDesire(script.PlayerOf(w.player), 100) })
	w.onQueue(t, leader, func() { script.NPCOf(leader).SetInt(script.SlotFlag, 777) })
	w.srv.Advance(t, 10*time.Second)
	teleports("tick after the flag changed", true)
	if got := ind1Slot(raikel, script.SlotIntAI0); got != 777 {
		t.Fatalf("i_ai0 after the jump = %d, want the new flag 777", got)
	}

	w.teleport(t, leader, 2900, 0)
	w.srv.Advance(t, 10*time.Second)
	teleports("tick with the master 2900 away", false)
	w.teleport(t, leader, 200, 0)
	w.srv.Advance(t, 10*time.Second)
	teleports("tick with the master 3100 away", true)

	w.kill(t, leader)
	if got := ind1Slot(raikel, script.SlotIntAI1); got != 0 {
		t.Fatalf("i_ai1 after its master died = %d, want 0", got)
	}
	w.clearDesires(t, raikel)
	w.srv.Advance(t, 30*time.Second)
	if !w.inWorld(raikel) {
		t.Fatal("Raikel left the world after its master died, want it kept: it has no 20 s check")
	}
}

// TestPartyPrivateCannibaliAggressiveRunsToItsMaster ports
// PartyPrivateCannibaliAggressive's attacked hook, every draw 0: hit more
// than 200 from its live master, it wants to walk to where the master
// stands with weight 1000; at 50 it does not. Either way it wants both
// physical specials on the attacker with weight 1000000.
func TestPartyPrivateCannibaliAggressiveRunsToItsMaster(t *testing.T) {
	t.Parallel()
	for _, row := range []struct {
		name string
		dx   int
		walk bool
	}{{"far", 300, true}, {"near", 50, false}} {
		t.Run(row.name, func(t *testing.T) {
			t.Parallel()
			w := bootIndividual4(t, raidBoot{}, ind4Leader(ind4CannibaliID, 1), ind4Template(ind4CannibaliID, false))
			leader := w.spawn(t, leaderID, 600, 0)
			private := privatesOf(t, leader, 1)[0]
			w.srv.InstallNPCCastSeam(t, private, ind4Skills())
			lx, ly, lz := leader.Position()
			w.onQueue(t, private, func() {
				script.NPCOf(private).InstantTeleportTo(script.Loc{X: lx + row.dx, Y: ly, Z: lz})
			})
			w.clearDesires(t, private)

			w.hit(t, private, 10)
			me := w.player.ObjectID()
			w.wantCasts(t, "after the hit", private, map[modelskill.ID]ind1Cast{ind4Special1Skill: {me, 1000000}, ind4Special2Skill: {me, 1000000}})
			d, ok := desire(private, actorai.IntentionMoveTo)
			if !row.walk {
				if ok {
					t.Fatalf("move desire hit 50 from its master = %+v, want none", d)
				}
				return
			}
			if !ok || d.Weight != 1000 || d.Location.X != lx || d.Location.Y != ly {
				t.Fatalf("move desire hit 300 from its master = %+v (%v), want weight 1000 to %d,%d", d, ok, lx, ly)
			}
		})
	}
}

// TestPartyPrivateCannibaliAggressiveSpawnsQuietly ports
// PartyPrivateCannibaliAggressive's created, see-creature and script-event
// hooks. It spawns with none of its parents' timers: out of the
// character's sight, its master's death leaves it in the world past the
// 20 s check a party private runs. Spawned beside the character, it wants
// to attack it on sight with weight 200. Event 10028 takes it out of the
// world (DeleteObject).
func TestPartyPrivateCannibaliAggressiveSpawnsQuietly(t *testing.T) {
	t.Parallel()
	w := bootIndividual4(t, raidBoot{}, ind4Leader(ind4CannibaliID, 1), ind4Template(ind4CannibaliID, false))
	far := w.spawn(t, leaderID, 4000, 0)
	lonely := privatesOf(t, far, 1)[0]
	w.kill(t, far)
	w.srv.Advance(t, 25*time.Second)
	if !w.inWorld(lonely) {
		t.Fatal("the masterless private left the world, want it kept: it has no 20 s check")
	}

	near := w.spawn(t, leaderID, 150, 0)
	private := privatesOf(t, near, 1)[0]
	w.ticks(t, 2)
	if got := private.AggroHate(w.player); got != 200 {
		t.Fatalf("hate of the character it sees = %v, want 200", got)
	}

	w.sendEvent(t, private, 10028, 0, 0)
	if got := framesOf(w.srv.ReadQueued(t, w.srv.Client), serverpackets.OpcodeDeleteObject, private.ObjectID()); len(got) != 1 {
		t.Fatalf("DeleteObject frames on event 10028 = %d, want 1", len(got))
	}
	if w.inWorld(private) {
		t.Fatal("the private is still in the world after event 10028")
	}
}
