package npc

import (
	"slices"
	"testing"
	"time"

	"github.com/fatal10110/acis_golang/internal/gameserver/model/actor"
	"github.com/fatal10110/acis_golang/internal/gameserver/model/actor/attackable"
	"github.com/fatal10110/acis_golang/internal/gameserver/model/actor/attackable/attackabletest"
	"github.com/fatal10110/acis_golang/internal/gameserver/world"
	"github.com/fatal10110/acis_golang/internal/testsupport/scriptcontract"
)

// sightCreature is a creature a seer scans: a player, a summon of a player
// or an NPC, with the states the scans gate on.
type sightCreature struct {
	world.Presence
	attackabletest.Combatant
	id                        int32
	kind                      actor.Kind
	owner                     *sightCreature
	protected, flying, hidden bool
	silent, moving            bool
}

func (c *sightCreature) ObjectID() int32      { return c.id }
func (c *sightCreature) Kind() actor.Kind     { return c.kind }
func (c *sightCreature) SpawnProtected() bool { return c.protected }
func (c *sightCreature) Flying() bool         { return c.flying }
func (c *sightCreature) Invisible() bool      { return c.hidden }
func (c *sightCreature) SilentMoving() bool   { return c.silent }
func (c *sightCreature) IsMoving() bool       { return c.moving }
func (c *sightCreature) Owner() (attackable.Combatant, bool) {
	if c.owner == nil {
		return nil, false
	}
	return c.owner, true
}

// sightHooks reports every NPC as having see-creature scripts; any other
// hook panics.
type sightHooks struct{ ScriptHooks }

func (sightHooks) SeesCreatures(int32) bool { return true }

// sightNPC is a seer whose see-creature hooks record the creatures told.
type sightNPC struct {
	world.Presence
	attackabletest.Combatant
	w                *world.State
	raid, seeThrough bool
	moving           bool
	rng              int
	at               time.Time
	sight            sight
	clock            lookNeighborClock
	told             []int32
}

func (n *sightNPC) ObjectID() int32                         { return 1 }
func (n *sightNPC) IsMoving() bool                          { return n.moving }
func (n *sightNPC) npcID() int32                            { return 20049 }
func (n *sightNPC) raidBoss() bool                          { return n.raid }
func (n *sightNPC) canSeeThrough() bool                     { return n.seeThrough }
func (n *sightNPC) seeRange() int                           { return n.rng }
func (n *sightNPC) sightState() *sight                      { return &n.sight }
func (n *sightNPC) worldState() *world.State                { return n.w }
func (n *sightNPC) scriptHooks() ScriptHooks                { return sightHooks{} }
func (n *sightNPC) now() time.Time                          { return n.at }
func (n *sightNPC) raiseSeeCreature(c attackable.Combatant) { n.told = append(n.told, c.ObjectID()) }

// The goldens' scenes: the NPC sees 1000 around it, so a creature 100
// along x is in range even 501 above it; one 1500 along x is out of range
// but still in the NPC's surrounding regions.
const (
	sightX, sightY = 1000, 1000
	sightRange     = 1000
)

func newSightNPC(raid, seeThrough bool) (*world.State, *sightNPC) {
	w := world.New()
	n := &sightNPC{w: w, raid: raid, seeThrough: seeThrough, rng: sightRange}
	w.Spawn(n, sightX, sightY, 0, 0)
	return w, n
}

// sightTarget places the golden row's creature: a player, or the summon of
// a player, holding the row's states, in or out of range and dz above the
// NPC. The summon's owner stands out of range on the other side.
func sightTarget(t *testing.T, w *world.State, r scriptcontract.Row, inRange bool, dz int) *sightCreature {
	t.Helper()
	flag := func(key string) bool {
		v, ok := r.Value(key)
		return ok && v == "true"
	}
	player := &sightCreature{id: 2, kind: actor.KindPlayer, protected: flag("protected"), flying: flag("flying"), hidden: flag("invisible")}
	c := player
	creature, _ := r.Value("creature")
	switch creature {
	case "summon":
		c = &sightCreature{id: 3, kind: actor.KindSummon, owner: player}
		w.Spawn(player, sightX-1500, sightY, 0, 0)
	case "npc":
		c = &sightCreature{id: 4, kind: actor.KindNPC}
	}
	c.silent = flag("silent")
	if m, _ := r.Value("moving"); m == "creature" {
		c.moving = true
	}
	x := sightX + 100
	if !inRange {
		x = sightX + 1500
	}
	w.Spawn(c, x, sightY, dz, 0)
	return c
}

// TestSightScanGates replays the tick scan's gate golden: per row, the
// calls the NPC's scan makes for one playable and whether it is in the
// seen set afterwards.
func TestSightScanGates(t *testing.T) {
	scriptcontract.Run(t, "aggro.scan_gates", func(t *testing.T, r scriptcontract.Row) {
		w, n := newSightNPC(r.Bool(t, "raid"), r.Bool(t, "see_through"))
		n.moving = r.Str(t, "moving") == "npc"
		c := sightTarget(t, w, r, r.Bool(t, "in_range"), int(r.Int(t, "dz")))
		if r.Bool(t, "seen") {
			n.sight.markSeen(c)
		}
		scanSight(n)
		if got, want := len(n.told), int(r.Int(t, "calls")); got != want {
			t.Errorf("calls = %d, want %d", got, want)
		}
		if got, want := n.sight.hasSeen(c), r.Bool(t, "seen_after"); got != want {
			t.Errorf("seen after the scan = %v, want %v", got, want)
		}
	})
}

// TestSightScanSkipsNonPlayables pins that the tick scan tells nothing
// about a known NPC.
func TestSightScanSkipsNonPlayables(t *testing.T) {
	w, n := newSightNPC(false, false)
	w.Spawn(&sightCreature{id: 4, kind: actor.KindNPC}, sightX+100, sightY, 0, 0)
	scanSight(n)
	if len(n.told) != 0 {
		t.Fatalf("calls = %v, want none for an NPC", n.told)
	}
}

// TestLookNeighborGolden replays the look-neighbour golden. A throttle row
// says whether a call runs a scan, the first one or one elapsed ms after
// the last; a gate row gives the calls for one creature in range, NPCs
// included.
func TestLookNeighborGolden(t *testing.T) {
	scriptcontract.Run(t, "aggro.look_neighbor", func(t *testing.T, r scriptcontract.Row) {
		if elapsed, ok := r.Value("elapsed"); ok {
			w, n := newSightNPC(false, false)
			w.Spawn(&sightCreature{id: 2, kind: actor.KindPlayer}, sightX+100, sightY, 0, 0)
			n.at = time.Unix(1_000_000, 0)
			if elapsed != "first" {
				lookNeighbor(n, &n.clock)
				n.told = nil
				n.at = n.at.Add(time.Duration(r.Int(t, "elapsed")) * time.Millisecond)
			}
			lookNeighbor(n, &n.clock)
			if got, want := len(n.told) > 0, r.Bool(t, "runs"); got != want {
				t.Fatalf("scan ran = %v, want %v", got, want)
			}
			return
		}
		w, n := newSightNPC(r.Bool(t, "raid"), r.Bool(t, "see_through"))
		sightTarget(t, w, r, true, int(r.Int(t, "dz")))
		lookNeighbor(n, &n.clock)
		if got, want := len(n.told), int(r.Int(t, "calls")); got != want {
			t.Fatalf("calls = %d, want %d", got, want)
		}
	})
}

// TestLookNeighborReachesTheSeeRangeEdge pins the look-neighbour range
// rule, unlike the tick scan's: a creature exactly at the see range is in
// it.
func TestLookNeighborReachesTheSeeRangeEdge(t *testing.T) {
	w, n := newSightNPC(false, false)
	w.Spawn(&sightCreature{id: 2, kind: actor.KindPlayer}, sightX+sightRange, sightY, 0, 0)
	lookNeighbor(n, &n.clock)
	if !slices.Equal(n.told, []int32{2}) {
		t.Fatalf("calls = %v, want one for the creature at the edge", n.told)
	}
	n.told = nil
	n.sight = sight{}
	scanSight(n)
	if len(n.told) != 0 {
		t.Fatalf("tick scan calls = %v, want none at the edge", n.told)
	}
}

// TestForceSeeCreatureRows replays the teleport re-fire golden rows that
// reach the re-fire scan, ForceSeeCreature: a summon's spawn, a summon's
// teleport, an instant teleport, and an NPC teleporting. A player's own
// teleportTo reaches it only with a detached client, which no Go player
// teleport has yet (#3445); tests/pets pins that an attached player's
// landing makes no call.
func TestForceSeeCreatureRows(t *testing.T) {
	for _, r := range scriptcontract.Lookup(t, "aggro.teleport_see").Rows {
		if r.Str(t, "creature") == "player" && r.Str(t, "entry") == "teleportTo" {
			continue
		}
		t.Run(r.ID, func(t *testing.T) {
			w, n := newSightNPC(false, false)
			c := sightTarget(t, w, r, r.Bool(t, "in_range"), int(r.Int(t, "dz")))
			ForceSeeCreature(w, c)
			if got, want := len(n.told), int(r.Int(t, "calls")); got != want {
				t.Fatalf("calls = %d, want %d", got, want)
			}
			if n.sight.hasSeen(c) {
				t.Fatal("the re-fire filled the NPC's seen set")
			}
		})
	}
}
