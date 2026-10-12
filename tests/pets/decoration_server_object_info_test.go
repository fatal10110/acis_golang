package pets

import (
	"testing"

	"github.com/fatal10110/acis_golang/internal/commons/wire"
	"github.com/fatal10110/acis_golang/internal/gameserver/model/actor/npc"
	"github.com/fatal10110/acis_golang/internal/gameserver/model/zone"
	"github.com/fatal10110/acis_golang/internal/gameserver/network/clientpackets"
	"github.com/fatal10110/acis_golang/internal/gameserver/network/serverpackets"
	"github.com/fatal10110/acis_golang/internal/gameservertest"
)

// serverObjectInfos decodes the attackable flag and the HP pair of every
// ServerObjectInfo of objID among frames, in order.
func serverObjectInfos(frames [][]byte, objID int32) [][3]int32 {
	var out [][3]int32
	for _, f := range frames {
		if len(f) < 5 || f[0] != serverpackets.OpcodeServerObjectInfo {
			continue
		}
		r := wire.NewReader(f[1:])
		if r.ReadInt32() != objID {
			continue
		}
		r.ReadInt32() // template id
		r.ReadString()
		attackable := r.ReadInt32()
		for range 4 { // x, y, z, heading
			r.ReadInt32()
		}
		for range 4 { // the two multipliers, collision radius and height
			r.ReadFloat64()
		}
		out = append(out, [3]int32{attackable, r.ReadInt32(), r.ReadInt32()})
	}
	return out
}

// TestStillDecorationInWaterShowsAttackableWithFullHP pins the stationary
// view of a placed Christmas Tree that cannot move: the tree is a Folk, so
// a player may attack it by force (Creature.isAttackableBy), and crossing
// the water shows its known players ServerObjectInfo (WaterZone.java:28-59,
// its move speed is 0) attackable 1 with its full HP, on placement and
// again on removal (ServerObjectInfo.writeImpl). 2444 base HP times the
// CON 43 bonus 1.58 is 3861.
func TestStillDecorationInWaterShowsAttackableWithFullHP(t *testing.T) {
	t.Parallel()
	water := zone.NewWater(1, summonZoneBox(t, 4_000, 6_000))
	zones := zone.NewIndex()
	zones.Add(water)
	still := treeTemplate()
	still.RunSpeed, still.WalkSpeed = 0, 0
	still.HPMax, still.CON = 2444, 43
	h := bootOwnerWithCollarOpts(t, []gameservertest.Option{
		gameservertest.WithZones(zones),
		gameservertest.WithNPCs(npc.NewTable([]*npc.Template{wolfTemplate(), still})),
	}, seedItem{TemplateID: treeKitID, Count: 1})

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
		t.Fatal("still tree placed in water is not an occupant of the water zone")
	}
	want := [3]int32{1, 3861, 3861}
	if got := serverObjectInfos(frames, tree.ObjectID()); len(got) != 1 || got[0] != want {
		t.Fatalf("placed still tree ServerObjectInfo (attackable, HP, max HP) = %v, want one %v", got, want)
	}

	tree.Despawn()
	if got := serverObjectInfos(drainFrames(t, h.client), tree.ObjectID()); len(got) != 1 || got[0] != want {
		t.Fatalf("removed still tree ServerObjectInfo (attackable, HP, max HP) = %v, want one %v", got, want)
	}
}
