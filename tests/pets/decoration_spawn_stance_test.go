package pets

import (
	"testing"

	"github.com/fatal10110/acis_golang/internal/commons/wire"
	"github.com/fatal10110/acis_golang/internal/gameserver/network/serverpackets"
)

// npcStanceFrame is one NpcInfo or ChangeMoveType frame of an NPC, decoded
// down to its stance fields.
type npcStanceFrame struct {
	opcode  byte
	running bool
	// moveMultiplier is the NpcInfo movement multiplier; swimming the
	// ChangeMoveType swim flag.
	moveMultiplier float64
	swimming       bool
}

// npcStanceFrames decodes, in order, every NpcInfo and ChangeMoveType of
// objID among frames.
func npcStanceFrames(t *testing.T, frames [][]byte, objID int32) []npcStanceFrame {
	t.Helper()
	var out []npcStanceFrame
	for _, f := range frames {
		if len(f) < 5 || (f[0] != serverpackets.OpcodeNPCInfo && f[0] != serverpackets.OpcodeChangeMoveType) {
			continue
		}
		r := wire.NewReader(f[1:])
		if r.ReadInt32() != objID {
			continue
		}
		fr := npcStanceFrame{opcode: f[0]}
		if f[0] == serverpackets.OpcodeChangeMoveType {
			fr.running, fr.swimming = r.ReadInt32() == 1, r.ReadInt32() == 1
		} else {
			for range 7 + 2 + 8 { // template .. heading, 0, C.Spd, P.Spd, speeds
				r.ReadInt32()
			}
			fr.moveMultiplier = r.ReadFloat64()
			for range 3 { // attack speed multiplier, collision radius, height
				r.ReadFloat64()
			}
			for range 3 { // right hand, chest, left hand
				r.ReadInt32()
			}
			r.ReadUint8() // name above char
			fr.running = r.ReadUint8() == 1
		}
		if err := r.Err(); err != nil {
			t.Fatalf("decode frame %#x of %d: %v", f[0], objID, err)
		}
		out = append(out, fr)
	}
	return out
}

// TestChristmasTreeSpawnsWalkingThenTakesItsRunStance places a Christmas
// Tree. Like every NPC it enters the world walking and then takes its run
// stance (Npc.onSpawn -> setWalkOrRun(true)): the player who sees it appear
// is shown its NpcInfo walking, with the walk-stance multiplier 30 * 1.1
// over the walk base 30, then ChangeMoveType to running (not swimming), as
// its run speed is not 0, then its NpcInfo again running, with the run
// multiplier 60 * 1.1 over 60.
func TestChristmasTreeSpawnsWalkingThenTakesItsRunStance(t *testing.T) {
	t.Parallel()
	h := bootOwnerWithCollar(t, seedItem{TemplateID: treeKitID, Count: 1})
	h.client.Send(encodeUseItem(h.seededItem(t, treeKitID), false))
	frames := drainFrames(t, h.client)
	if len(frames) == 0 || frames[0][0] != serverpackets.OpcodeNPCInfo {
		t.Fatal("placing the tree did not show its NpcInfo first")
	}
	treeID := wire.NewReader(frames[0][1:]).ReadInt32()

	dexBonus := 1.1
	walk := float64(float32(30*dexBonus) / float32(30))
	run := float64(float32(60*dexBonus) / float32(60))
	want := []npcStanceFrame{
		{opcode: serverpackets.OpcodeNPCInfo, running: false, moveMultiplier: walk},
		{opcode: serverpackets.OpcodeChangeMoveType, running: true},
		{opcode: serverpackets.OpcodeNPCInfo, running: true, moveMultiplier: run},
	}
	got := npcStanceFrames(t, frames, treeID)
	if len(got) != len(want) {
		t.Fatalf("tree spawn frames = %+v, want %+v", got, want)
	}
	for i := range want {
		if got[i] != want[i] {
			t.Fatalf("tree spawn frame %d = %+v, want %+v", i, got[i], want[i])
		}
	}
}
