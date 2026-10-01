package npcs

import (
	"testing"

	"github.com/fatal10110/acis_golang/internal/commons/wire"
	"github.com/fatal10110/acis_golang/internal/gameserver/model/actor/npc"
	"github.com/fatal10110/acis_golang/internal/gameserver/model/location"
	"github.com/fatal10110/acis_golang/internal/gameserver/network/clientpackets"
	"github.com/fatal10110/acis_golang/internal/gameserver/network/serverpackets"
)

// TestEnteringPlayerSeesWalkingFolkMove pins what a player coming to know a
// civilian NPC under way on its route is shown: NpcInfo, then right after it
// the NPC's MoveToLocation, from where it stands now to the end of its
// current leg, so it is not seen frozen until its next leg starts.
func TestEnteringPlayerSeesWalkingFolkMove(t *testing.T) {
	t.Parallel()
	w := bootFolkWorld(t, nil)
	// 900 units at the fixture walk speed (42/s) outlast the second
	// player's entry on any clock.
	at := location.Location{X: w.at.X + 100, Y: w.at.Y, Z: w.at.Z}
	a := location.Location{X: w.at.X + 100, Y: w.at.Y + 900, Z: w.at.Z}
	b := location.Location{X: w.at.X + 100, Y: w.at.Y - 1000, Z: w.at.Z}
	f := w.spawnWalker(t, at, a, b)
	drainUntilQuiet(t, w.c)
	if !f.IsMoving() {
		t.Fatal("walker is not walking its route")
	}

	w.srv.SeedCharacterFor(t, "player2", "Arriving", playerLevel, 0)
	c := w.srv.DialClient(t, "player2", 1)
	c.Send(encodeRequestGameStart(0))
	for frame := c.Read(); frame[0] != serverpackets.OpcodeCharSelected; frame = c.Read() {
	}
	c.Send(wire.NewPacketWriter(clientpackets.OpcodeEnterWorld).Bytes())
	frames := drainFrames(t, c)

	next := frameAfterNpcInfo(t, frames, f)
	if next == nil || next[0] != serverpackets.OpcodeMoveToLocation {
		t.Fatalf("frames after walker NpcInfo = %x, want MoveToLocation next", opcodes(frames))
	}
	dests, origins := folkMoves([][]byte{next}, f)
	if len(dests) != 1 || dests[0] != a {
		t.Fatalf("MoveToLocation = %v, want the walker's leg to %v", dests, a)
	}
	now := folkAt(f)
	if w.srv.DrivesClock() {
		if origins[0] != now {
			t.Fatalf("MoveToLocation origin = %v, want the walker's position %v", origins[0], now)
		}
	} else if o := origins[0]; o.X != at.X || o.Y < at.Y || o.Y > now.Y {
		t.Fatalf("MoveToLocation origin = %v, want on the leg between %v and %v", o, at, now)
	}
}

// TestEnteringPlayerSeesStandingFolkInfoOnly pins that a civilian NPC doing
// nothing is shown with its NpcInfo alone.
func TestEnteringPlayerSeesStandingFolkInfoOnly(t *testing.T) {
	t.Parallel()
	w := bootFolkWorld(t, nil)
	f := w.spawnFolk(t, folkTemplate("Folk", walkerID), 50)

	w.srv.SeedCharacterFor(t, "player2", "Arriving", playerLevel, 0)
	c := w.srv.DialClient(t, "player2", 1)
	c.Send(encodeRequestGameStart(0))
	for frame := c.Read(); frame[0] != serverpackets.OpcodeCharSelected; frame = c.Read() {
	}
	c.Send(wire.NewPacketWriter(clientpackets.OpcodeEnterWorld).Bytes())
	frames := drainFrames(t, c)

	if next := frameAfterNpcInfo(t, frames, f); next != nil && next[0] == serverpackets.OpcodeMoveToLocation {
		if dests, _ := folkMoves([][]byte{next}, f); len(dests) > 0 {
			t.Fatalf("standing NPC's NpcInfo followed by MoveToLocation to %v, want NpcInfo alone", dests[0])
		}
	}
}

// frameAfterNpcInfo returns the frame read right after f's NpcInfo, nil when
// the NpcInfo was the last one.
func frameAfterNpcInfo(t *testing.T, frames [][]byte, f *npc.Folk) []byte {
	t.Helper()
	for i, frame := range frames {
		if frame[0] != serverpackets.OpcodeNPCInfo || wire.NewReader(frame[1:]).ReadInt32() != f.ObjectID() {
			continue
		}
		if i+1 < len(frames) {
			return frames[i+1]
		}
		return nil
	}
	t.Fatalf("no NpcInfo for %d among %x", f.ObjectID(), opcodes(frames))
	return nil
}
