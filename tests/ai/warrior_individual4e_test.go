package ai

import (
	"strconv"
	"testing"

	actorai "github.com/fatal10110/acis_golang/internal/gameserver/model/actor/ai"
	"github.com/fatal10110/acis_golang/internal/gameserver/model/actor/npc"
	"github.com/fatal10110/acis_golang/internal/gameserver/network/serverpackets"
	"github.com/fatal10110/acis_golang/internal/gameserver/script"
	"github.com/fatal10110/acis_golang/internal/gameservertest"
)

// The NPC ids the scenarios of the batch's fifth cut spawn (#189 PR 5).
const (
	ind4eSpawnPrivateID = int32(22101) // PartyPrivateSpawnPrivateAggressive
	ind4eCalledID       = int32(18327) // PartyPrivateSpawnPrivateAggressive
)

// ind4eSpawnPrivateTemplate is a fixture PartyPrivateSpawnPrivateAggressive
// of id with the AI parameters given.
func ind4eSpawnPrivateTemplate(id int32, params npc.AIParams) *npc.Template {
	tmpl := ind4Template(id, false)
	tmpl.AIParams = params
	return tmpl
}

// TestPartyPrivateSpawnPrivateAggressiveAttacksOnSight ports
// PartyPrivateSpawnPrivateAggressive's see-creature hook: it turns on the
// character it sees (hate 200) before the parent's call. With
// AttackLowLevel 1, a level 40 private already attacking by then finds the
// level 20 character more than 15 levels below it and, dropping its attack
// desires, wants it with weight 700.
func TestPartyPrivateSpawnPrivateAggressiveAttacksOnSight(t *testing.T) {
	t.Parallel()
	for _, row := range []struct {
		name   string
		params npc.AIParams
		level  int
		hate   float64
	}{{"own call only", nil, 1, 200}, {"then the parent's", npc.AIParams{"AttackLowLevel": "1"}, 40, 700}} {
		t.Run(row.name, func(t *testing.T) {
			t.Parallel()
			tmpl := ind4eSpawnPrivateTemplate(ind4eSpawnPrivateID, row.params)
			tmpl.Level = row.level
			w := bootIndividual4(t, raidBoot{}, tmpl)
			n := w.spawn(t, ind4eSpawnPrivateID, 150, 0)
			w.ticks(t, 1)
			if got := n.AggroHate(w.player); got != row.hate {
				t.Fatalf("hate of the character it sees = %v, want %v", got, row.hate)
			}
		})
	}
}

// TestPartyPrivateSpawnPrivateAggressiveWantsNothingIdle ports
// PartyPrivateSpawnPrivateAggressive's no-desire hook: wanting nothing
// beside its live master it neither follows it nor wanders, unlike its
// parent.
func TestPartyPrivateSpawnPrivateAggressiveWantsNothingIdle(t *testing.T) {
	t.Parallel()
	w := bootIndividual4(t, raidBoot{}, ind4Leader(ind4eSpawnPrivateID, 1), ind4eSpawnPrivateTemplate(ind4eSpawnPrivateID, nil))
	leader := w.spawn(t, leaderID, 1500, 0)
	private := privatesOf(t, leader, 1)[0]
	w.clearDesires(t, private)
	w.ticks(t, 2)
	for _, kind := range []actorai.Intention{actorai.IntentionFollow, actorai.IntentionWander, actorai.IntentionMoveTo} {
		if d, ok := desire(private, kind); ok {
			t.Fatalf("%v desire wanting nothing = %+v, want none", kind, d)
		}
	}
}

// TestPartyPrivateSpawnPrivateAggressiveLeavesOnItsEvents ports
// PartyPrivateSpawnPrivateAggressive's script-event hook. Its master's
// event 10002 naming the flagged character, which its parent answers with
// a line and an attack desire, does nothing; events 10023 and 11039 each
// take it out of the world (DeleteObject).
func TestPartyPrivateSpawnPrivateAggressiveLeavesOnItsEvents(t *testing.T) {
	t.Parallel()
	for _, event := range []int32{10023, 11039} {
		t.Run(strconv.Itoa(int(event)), func(t *testing.T) {
			t.Parallel()
			w := bootIndividual4(t, raidBoot{}, ind4Leader(ind4eSpawnPrivateID, 1), ind4eSpawnPrivateTemplate(ind4eSpawnPrivateID, nil))
			leader := w.spawn(t, leaderID, 400, 0)
			private := privatesOf(t, leader, 1)[0]
			me := w.player.ObjectID()
			w.onQueue(t, leader, func() { script.NPCOf(leader).SetInt(script.SlotFlag, me) })
			w.clearDesires(t, private)
			w.srv.ReadQueued(t, w.srv.Client)

			w.sendEvent(t, private, 10002, leader.ObjectID(), 0)
			if got := says(w.srv.ReadQueued(t, w.srv.Client), private.ObjectID()); len(got) != 0 {
				t.Fatalf("lines on its master's event 10002 = %q, want none", got)
			}
			if d, ok := desire(private, actorai.IntentionAttack); ok {
				t.Fatalf("attack desire on its master's event 10002 = %+v, want none", d)
			}
			if !w.inWorld(private) {
				t.Fatal("the private left the world on event 10002")
			}

			w.sendEvent(t, private, event, 0, 0)
			if got := framesOf(w.srv.ReadQueued(t, w.srv.Client), serverpackets.OpcodeDeleteObject, private.ObjectID()); len(got) != 1 {
				t.Fatalf("DeleteObject frames on event %d = %d, want 1", event, len(got))
			}
			if w.inWorld(private) {
				t.Fatalf("the private is still in the world after event %d", event)
			}
		})
	}
}

// bootIndividual4e boots the batch beside the third batch's trailing
// classes, FreyaSecretaryLeader among them, otherwise as bootIndividual4,
// with the director script sendEvent sends through.
func bootIndividual4e(t *testing.T, templates ...*npc.Template) *ind1World {
	t.Helper()
	list, catalog := partyScripts()
	for _, c := range append(ind3cBatch[:len(ind3cBatch):len(ind3cBatch)], ind4Batch...) {
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
	w := bootRaid(t, raidBoot{opts: []gameservertest.Option{
		gameservertest.WithScriptRand(draws.rand),
		gameservertest.WithNPCScripts(kinds, list, catalog),
	}}, templates...)
	return &ind1World{warriorWorld: w, draws: draws, heard: &ind1Heard{}}
}

// TestPartyPrivateSpawnPrivateAggressiveMakesWayForTheSecretarysCall runs
// a FreyaSecretaryLeader leading a PartyPrivateSpawnPrivateAggressive.
// The private hit under a third of its HP has its master call two
// privates of 18327 onto its spot, which the client sees appear (NpcInfo),
// and tell it event 10023, on which it leaves the world (DeleteObject).
// The called privates run the same class.
func TestPartyPrivateSpawnPrivateAggressiveMakesWayForTheSecretarysCall(t *testing.T) {
	t.Parallel()
	secretary := warriorTemplate(ind3cSecretaryID, nil)
	secretary.CanMove = false
	secretary.Privates = []npc.PrivateEntry{{NpcID: int(ind4eSpawnPrivateID), Weight: 1, RespawnDelay: ind4PrivateRespawn}}
	w := bootIndividual4e(t, secretary, ind4eSpawnPrivateTemplate(ind4eSpawnPrivateID, nil), ind4eSpawnPrivateTemplate(ind4eCalledID, nil))
	n := w.spawn(t, ind3cSecretaryID, 1500, 0)
	private := privatesOf(t, n, 1)[0]
	w.srv.ReadQueued(t, w.srv.Client)

	w.setHPShare(t, private, 0.3)
	px, py, _ := private.Position()
	w.hit(t, private, 1)
	frames := w.srv.ReadQueued(t, w.srv.Client)
	if got := framesOf(frames, serverpackets.OpcodeDeleteObject, private.ObjectID()); len(got) != 1 {
		t.Fatalf("DeleteObject frames of the wounded private = %d, want 1", len(got))
	}
	if w.inWorld(private) {
		t.Fatal("the wounded private is still in the world after event 10023")
	}
	var called []*npc.Hostile
	for _, m := range n.Minions() {
		if c := m.(*npc.Hostile); c.ObjectID() != private.ObjectID() {
			called = append(called, c)
		}
	}
	if len(called) != 2 {
		t.Fatalf("called privates = %d, want 2", len(called))
	}
	for _, c := range called {
		if c.NpcID() != int(ind4eCalledID) {
			t.Fatalf("called private npc id = %d, want %d", c.NpcID(), ind4eCalledID)
		}
		if cx, cy, _ := c.Position(); cx != px || cy != py {
			t.Fatalf("called private at %d,%d, want the private's spot %d,%d", cx, cy, px, py)
		}
		if got := framesOf(frames, serverpackets.OpcodeNPCInfo, c.ObjectID()); len(got) != 2 {
			t.Fatalf("NpcInfo frames of the called private = %d, want 2 (appears walking, then running)", len(got))
		}
	}

	// The called privates bind the class too: event 11039 takes one out of
	// the world and leaves the other.
	w.sendEvent(t, called[0], 11039, 0, 0)
	if got := framesOf(w.srv.ReadQueued(t, w.srv.Client), serverpackets.OpcodeDeleteObject, called[0].ObjectID()); len(got) != 1 {
		t.Fatalf("DeleteObject frames of the called private on event 11039 = %d, want 1", len(got))
	}
	if w.inWorld(called[0]) || !w.inWorld(called[1]) {
		t.Fatalf("in the world after event 11039 to the first called private: %v, %v, want false, true", w.inWorld(called[0]), w.inWorld(called[1]))
	}
}

// TestPartyPrivateSpawnPrivateAggressiveIgnoresMonstersItSees ports the
// playable test of PartyPrivateSpawnPrivateAggressive's see-creature hook:
// out of the character's sight, a look around (which another script may
// make it run) tells it about the monster beside it, and it neither wants
// to attack it nor hates it.
func TestPartyPrivateSpawnPrivateAggressiveIgnoresMonstersItSees(t *testing.T) {
	t.Parallel()
	w := bootIndividual4(t, raidBoot{}, ind4eSpawnPrivateTemplate(ind4eSpawnPrivateID, nil), warriorTemplate(siegeDummyID, nil))
	private := w.spawn(t, ind4eSpawnPrivateID, 6000, 0)
	monster := w.spawn(t, siegeDummyID, 6050, 0)
	w.ticks(t, 1)
	w.clearDesires(t, private)
	w.onQueue(t, private, private.LookNeighbor)
	if d, ok := desire(private, actorai.IntentionAttack); ok {
		t.Fatalf("attack desire after seeing the monster = %+v, want none", d)
	}
	if got := private.AggroHate(monster); got != 0 {
		t.Fatalf("hate of the monster it sees = %v, want 0", got)
	}
}
