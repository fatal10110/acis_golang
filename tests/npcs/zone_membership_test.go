package npcs

import (
	"sync/atomic"
	"testing"

	"github.com/fatal10110/acis_golang/internal/commons/wire"
	"github.com/fatal10110/acis_golang/internal/gameserver/model/actor/move"
	"github.com/fatal10110/acis_golang/internal/gameserver/model/location"
	"github.com/fatal10110/acis_golang/internal/gameserver/model/zone"
	"github.com/fatal10110/acis_golang/internal/gameserver/network/serverpackets"
	"github.com/fatal10110/acis_golang/internal/gameservertest"
)

// zoneBox is a zone volume over x in [minX, maxX] around the fixture
// character's spawn row.
func zoneBox(t *testing.T, minX, maxX int) zone.Form {
	t.Helper()
	form, err := zone.NewCuboid(minX, maxX, -1_000, 1_000, -1_000, 1_000)
	if err != nil {
		t.Fatalf("zone form: %v", err)
	}
	return form
}

// npcCrossings counts the NPC-class enters and exits of one zone.
type npcCrossings struct{ enters, exits atomic.Int32 }

func countNPCCrossings(z *zone.Zone) *npcCrossings {
	c := &npcCrossings{}
	z.OnEnter(func(a zone.Actor) {
		if a.Class() == zone.ClassNPC {
			c.enters.Add(1)
		}
	})
	z.OnExit(func(a zone.Actor) {
		if a.Class() == zone.ClassNPC {
			c.exits.Add(1)
		}
	})
	return c
}

func (c *npcCrossings) assert(t *testing.T, when string, enters, exits int32) {
	t.Helper()
	if got, gotExits := c.enters.Load(), c.exits.Load(); got != enters || gotExits != exits {
		t.Fatalf("%s: NPC zone enters/exits = %d/%d, want %d/%d", when, got, gotExits, enters, exits)
	}
}

// npcInfoFrames keeps the NpcInfo frames of objID among frames.
func npcInfoFrames(frames [][]byte, opcode byte, objID int32) [][]byte {
	var out [][]byte
	for _, f := range frames {
		if len(f) >= 5 && f[0] == opcode && wire.NewReader(f[1:]).ReadInt32() == objID {
			out = append(out, f)
		}
	}
	return out
}

// npcInfoMoveType reads an NpcInfo frame's move type byte: the field after
// the ally crest, followed by the team byte, the collision radius and
// height (two doubles), the enchant effect and the flying flag.
func npcInfoMoveType(frame []byte) byte { return frame[len(frame)-1-(1+8+8+4+4)] }

// TestHostileNPCZoneMembership pins an attackable NPC's zone membership
// through its life (Creature.setRegion, Creature.java:1773-1791;
// Creature.teleportTo, Creature.java:386-429; ZoneType.revalidateInZone and
// removeCreature): spawning inside a peace zone enters it and holds the NPC
// in peace; a teleport leaves the zones around the old position and enters
// those at the destination; decaying leaves them.
func TestHostileNPCZoneMembership(t *testing.T) {
	t.Parallel()
	peace := zone.NewPeace(1, zoneBox(t, 300, 2_000))
	crossings := countNPCCrossings(peace.Core())
	zones := zone.NewIndex()
	zones.Add(peace)
	w := bootFolkWorld(t, nil, gameservertest.WithZones(zones))

	inside := location.Location{X: 800, Y: w.at.Y, Z: w.at.Z}
	outside := location.Location{X: w.at.X - 200, Y: w.at.Y, Z: w.at.Z}
	monster := w.srv.SpawnHostileNPCKindAt(t, "Monster", inside)
	drainUntilQuiet(t, w.c)
	if !monster.InPeaceZone() || !monster.InsideZone(zone.FlagPeace) {
		t.Fatal("NPC spawned inside a peace zone is not in peace")
	}
	crossings.assert(t, "after the spawn", 1, 0)

	monster.TeleportTo(outside)
	if monster.InPeaceZone() {
		t.Fatal("NPC teleported out of the peace zone is still in peace")
	}
	crossings.assert(t, "after teleporting out", 1, 1)

	monster.TeleportTo(inside)
	if !monster.InPeaceZone() {
		t.Fatal("NPC teleported into the peace zone is not in peace")
	}
	crossings.assert(t, "after teleporting back in", 2, 1)

	if !monster.Decay(w.srv.State, nil) {
		t.Fatal("decay did not run")
	}
	if monster.InPeaceZone() {
		t.Fatal("decayed NPC is still in peace")
	}
	crossings.assert(t, "after the decay", 2, 2)
	if got := len(peace.Occupants()); got != 0 {
		t.Fatalf("peace zone occupants after the decay = %d, want 0", got)
	}
}

// TestHostileNPCWalkIntoWaterSwims pins a moving NPC's water crossing: its
// zones follow its movement (Creature.revalidateZone from
// CreatureMove.updatePosition and the move's end), entering the water sets
// its swim move type (WaterZone.onEnter) and shows every known player its
// NpcInfo again carrying it (WaterZone.java:28-39), and leaving the water
// clears it and shows NpcInfo once more (WaterZone.java:48-59).
func TestHostileNPCWalkIntoWaterSwims(t *testing.T) {
	t.Parallel()
	zones := zone.NewIndex()
	zones.Add(zone.NewWater(1, zoneBox(t, 300, 2_000)))
	w := bootFolkWorld(t, nil, gameservertest.WithZones(zones))

	start := location.Location{X: w.at.X + 50, Y: w.at.Y, Z: w.at.Z}
	home := location.Location{X: 800, Y: w.at.Y, Z: w.at.Z}
	monster := w.srv.SpawnMovingHostileNPCAt(t, "Monster", home, start)
	drainUntilQuiet(t, w.c)
	if monster.InsideZone(zone.FlagWater) {
		t.Fatal("NPC spawned on dry land is in water")
	}

	// The NPC walks back to its spawn point, inside the water.
	if !monster.ReturnHome() {
		t.Fatal("NPC did not start walking home")
	}
	for i := 0; monster.IsMoving(); i++ {
		if i == 300 {
			t.Fatal("walk home still under way after 30 s of position updates")
		}
		w.srv.TickPositions()
	}
	if !monster.InsideZone(zone.FlagWater) {
		t.Fatal("NPC that walked into the water is not in it")
	}
	if got := monster.Move().MoveType(); got != move.MoveSwim {
		t.Fatalf("NPC in the water moves by %v, want swimming", got)
	}
	frames := drainFrames(t, w.c)
	infos := npcInfoFrames(frames, serverpackets.OpcodeNPCInfo, monster.ObjectID())
	if len(infos) != 1 {
		t.Fatalf("NpcInfo frames of the NPC entering the water = %d, want 1", len(infos))
	}
	if got := npcInfoMoveType(infos[0]); got != byte(move.MoveSwim) {
		t.Fatalf("NpcInfo move type on entering the water = %d, want %d (swim)", got, move.MoveSwim)
	}

	monster.TeleportTo(start)
	if monster.InsideZone(zone.FlagWater) {
		t.Fatal("NPC teleported out of the water is still in it")
	}
	frames = drainFrames(t, w.c)
	infos = npcInfoFrames(frames, serverpackets.OpcodeNPCInfo, monster.ObjectID())
	if len(infos) == 0 {
		t.Fatal("no NpcInfo of the NPC leaving the water")
	}
	if got := npcInfoMoveType(infos[0]); got != byte(move.MoveGround) {
		t.Fatalf("NpcInfo move type on leaving the water = %d, want %d (ground)", got, move.MoveGround)
	}
	if got := monster.Move().MoveType(); got != move.MoveGround {
		t.Fatalf("NPC out of the water moves by %v, want on the ground", got)
	}
}

// TestFolkNPCZoneMembership pins a civilian NPC's zone membership: one
// spawned in a peace zone is in peace, and one that cannot move spawned in
// water shows known players its stationary ServerObjectInfo view again
// (WaterZone.java:30-35) rather than NpcInfo.
func TestFolkNPCZoneMembership(t *testing.T) {
	t.Parallel()
	zones := zone.NewIndex()
	zones.Add(zone.NewPeace(1, zoneBox(t, 300, 1_000)))
	zones.Add(zone.NewWater(2, zoneBox(t, 1_200, 2_000)))
	w := bootFolkWorld(t, nil, gameservertest.WithZones(zones))

	merchant := w.spawnFolk(t, folkTemplate("Merchant", 30001), 600)
	if !merchant.InPeaceZone() {
		t.Fatal("civilian NPC spawned in a peace zone is not in peace")
	}
	if merchant.InsideZone(zone.FlagWater) {
		t.Fatal("civilian NPC spawned on dry land is in water")
	}

	tmpl := folkTemplate("Merchant", 30002)
	tmpl.RunSpeed, tmpl.WalkSpeed = 0, 0
	statue := w.srv.SpawnFolkNPCAt(t, tmpl, location.Location{X: 1_500, Y: w.at.Y, Z: w.at.Z})
	frames := drainFrames(t, w.c)
	if !statue.InsideZone(zone.FlagWater) {
		t.Fatal("civilian NPC spawned in water is not in it")
	}
	if statue.InPeaceZone() {
		t.Fatal("civilian NPC spawned outside the peace zone is in peace")
	}
	if got := npcInfoFrames(frames, serverpackets.OpcodeServerObjectInfo, statue.ObjectID()); len(got) != 1 {
		t.Fatalf("ServerObjectInfo frames of the still NPC entering the water = %d, want 1", len(got))
	}
}
