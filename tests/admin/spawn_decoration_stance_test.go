package admin

import (
	"slices"
	"testing"

	"github.com/fatal10110/acis_golang/internal/commons/wire"
	"github.com/fatal10110/acis_golang/internal/gameserver/network/serverpackets"
)

// TestAdminSpawnedTreeKeepsItsRunStance spawns a Christmas tree with
// //spawn: like every NPC it enters the world walking and takes its run
// stance (Npc.onSpawn), and, unlike a tree placed with its kit, is never
// put back to walking. The GM is shown its NpcInfo walking, ChangeMoveType
// to running and its NpcInfo running, and nothing more.
func TestAdminSpawnedTreeKeepsItsRunStance(t *testing.T) {
	t.Parallel()
	srv, _ := bootSpawnAdmin(t, nil)
	frames := exchange(t, srv.Client, encodeBuildCmd("spawn 13006"))
	trees := npcsOf(srv, treeID)
	if len(trees) != 1 {
		t.Fatalf("trees in the world = %d, want 1", len(trees))
	}
	type stance struct {
		opcode  byte
		running bool
	}
	var got []stance
	for _, f := range frames {
		if f[0] != serverpackets.OpcodeNPCInfo && f[0] != serverpackets.OpcodeChangeMoveType {
			continue
		}
		r := wire.NewReader(f[1:])
		if r.ReadInt32() != trees[0].ObjectID() {
			continue
		}
		s := stance{opcode: f[0]}
		if f[0] == serverpackets.OpcodeChangeMoveType {
			s.running = r.ReadInt32() == 1
		} else {
			for range 7 + 2 + 8 { // template .. heading, 0, C.Spd, P.Spd, speeds
				r.ReadInt32()
			}
			for range 4 { // multipliers, collision radius, height
				r.ReadFloat64()
			}
			for range 3 { // right hand, chest, left hand
				r.ReadInt32()
			}
			r.ReadUint8() // name above char
			s.running = r.ReadUint8() == 1
		}
		if err := r.Err(); err != nil {
			t.Fatalf("decode tree frame %#x: %v", f[0], err)
		}
		got = append(got, s)
	}
	want := []stance{
		{serverpackets.OpcodeNPCInfo, false},
		{serverpackets.OpcodeChangeMoveType, true},
		{serverpackets.OpcodeNPCInfo, true},
	}
	if !slices.Equal(got, want) {
		t.Fatalf("//spawn tree stance frames = %+v, want %+v", got, want)
	}
}
