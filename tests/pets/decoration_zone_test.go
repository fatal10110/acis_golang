package pets

import (
	"testing"

	"github.com/fatal10110/acis_golang/internal/commons/wire"
	"github.com/fatal10110/acis_golang/internal/gameserver/model/actor/move"
	"github.com/fatal10110/acis_golang/internal/gameserver/model/actor/npc"
	"github.com/fatal10110/acis_golang/internal/gameserver/model/zone"
	"github.com/fatal10110/acis_golang/internal/gameserver/network/clientpackets"
	"github.com/fatal10110/acis_golang/internal/gameserver/network/serverpackets"
	"github.com/fatal10110/acis_golang/internal/gameservertest"
)

// npcInfoMoveTypes returns the move type byte of every NpcInfo of objID
// among frames, in order: the field after the ally crest, followed by the
// team byte, the collision radius and height (two doubles), the enchant
// effect and the flying flag.
func npcInfoMoveTypes(frames [][]byte, objID int32) []byte {
	var out []byte
	for _, f := range frames {
		if len(f) >= 5 && f[0] == serverpackets.OpcodeNPCInfo && wire.NewReader(f[1:]).ReadInt32() == objID {
			out = append(out, f[len(f)-1-(1+8+8+4+4)])
		}
	}
	return out
}

func assertMoveTypes(t *testing.T, when string, got []byte, want ...move.MoveType) {
	t.Helper()
	if len(got) != len(want) {
		t.Fatalf("%s: NpcInfo move types = %v, want %v", when, got, want)
	}
	for i := range want {
		if got[i] != byte(want[i]) {
			t.Fatalf("%s: NpcInfo move types = %v, want %v", when, got, want)
		}
	}
}

// TestDecorationJoinsTheZonesItStandsIn pins a placed Christmas Tree's zone
// membership (a ChristmasTree is a Folk, so Creature.setRegion enters and
// leaves its zones): placed in water it is the zone's occupant, and its
// known players see its NpcInfo once on sight and once more, swimming, for
// the crossing (WaterZone.java:28-39: NpcInfo, its move speed is not 0).
// Removed, it leaves the zone, showing its NpcInfo out of the water before
// it goes (WaterZone.java:48-59).
func TestDecorationJoinsTheZonesItStandsIn(t *testing.T) {
	t.Parallel()
	water := zone.NewWater(1, summonZoneBox(t, 4_000, 6_000))
	zones := zone.NewIndex()
	zones.Add(water)
	h := bootOwnerWithCollarOpts(t, []gameservertest.Option{gameservertest.WithZones(zones)},
		seedItem{TemplateID: treeKitID, Count: 1})

	x, y, z := h.srv.PlayerPosition(t, h.ownerID)
	h.character(t).TeleportTo(x+5_000, y, z, 0)
	readUntilOpcode(t, h.client, serverpackets.OpcodeTeleportToLocation, "owner teleport")
	drainUntilQuiet(t, h.client)
	h.client.Send(encodeSingleOpcode(clientpackets.OpcodeAppearing))
	drainUntilQuiet(t, h.client)

	h.client.Send(encodeUseItem(h.seededItem(t, treeKitID), false))
	frames := drainFrames(t, h.client)
	var tree *npc.Decoration
	for _, a := range water.Occupants() {
		if obj, ok := h.srv.State.Object(a.ObjectID()); ok {
			if d, isTree := obj.(*npc.Decoration); isTree {
				tree = d
			}
		}
	}
	if tree == nil {
		t.Fatal("tree placed in water is not an occupant of the water zone")
	}
	if !tree.InsideZone(zone.FlagWater) {
		t.Fatal("tree placed in water is not in it")
	}
	assertMoveTypes(t, "placed in water", npcInfoMoveTypes(frames, tree.ObjectID()), move.MoveGround, move.MoveSwim)

	tree.Despawn()
	frames = drainFrames(t, h.client)
	for _, a := range water.Occupants() {
		if a.ObjectID() == tree.ObjectID() {
			t.Fatal("removed tree is still an occupant of the water zone")
		}
	}
	assertMoveTypes(t, "removed", npcInfoMoveTypes(frames, tree.ObjectID()), move.MoveGround)
	if _, ok := h.srv.State.Object(tree.ObjectID()); ok {
		t.Fatal("removed tree is still in the world")
	}
}
