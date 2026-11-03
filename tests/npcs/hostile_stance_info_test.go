package npcs

import (
	"testing"

	"github.com/fatal10110/acis_golang/internal/commons/wire"
	"github.com/fatal10110/acis_golang/internal/gameserver/model/location"
	"github.com/fatal10110/acis_golang/internal/gameserver/network/serverpackets"
)

// TestHostileStanceChangeShowsChangeMoveTypeThenNpcInfo pins a hostile
// NPC's run/walk switch (Npc.setWalkOrRun): an NPC that can move shows its
// observers ChangeMoveType and then its NpcInfo with the new stance; one
// that cannot move shows only the NpcInfo; asking for the stance it already
// holds sends nothing.
func TestHostileStanceChangeShowsChangeMoveTypeThenNpcInfo(t *testing.T) {
	t.Parallel()
	w := bootFolkWorld(t, nil)
	monster := w.srv.SpawnHostileNPCKindAt(t, "Monster", location.Location{X: w.at.X + 100, Y: w.at.Y, Z: w.at.Z})
	drainUntilQuiet(t, w.c)
	if !monster.Running() {
		t.Fatal("spawned monster is not running")
	}

	monster.ForceWalkStance()
	frames := drainFrames(t, w.c)
	if got := opcodes(frames); len(got) != 2 || got[0] != serverpackets.OpcodeChangeMoveType || got[1] != serverpackets.OpcodeNPCInfo {
		t.Fatalf("walk stance frames = %#x, want ChangeMoveType then NpcInfo", got)
	}
	if r := wire.NewReader(frames[0][1:]); r.ReadInt32() != monster.ObjectID() || r.ReadInt32() != 0 {
		t.Fatal("ChangeMoveType does not show the monster walking")
	}
	if wire.NewReader(frames[1][1:]).ReadInt32() != monster.ObjectID() || npcInfoRunning(npcFrame{opcode: frames[1][0], raw: frames[1]}) {
		t.Fatal("NpcInfo does not show the monster walking")
	}

	monster.ForceWalkStance()
	if frames := drainFrames(t, w.c); len(frames) != 0 {
		t.Fatalf("walk stance on a walking monster sent %#x, want nothing", opcodes(frames))
	}

	still := w.srv.SpawnHostileNPCTemplateAt(t, stillHostileTemplate("Monster"), location.Location{X: w.at.X - 100, Y: w.at.Y, Z: w.at.Z})
	drainUntilQuiet(t, w.c)
	still.ForceWalkStance()
	frames = drainFrames(t, w.c)
	if got := opcodes(frames); len(got) != 1 || got[0] != serverpackets.OpcodeNPCInfo {
		t.Fatalf("walk stance frames of a monster that cannot move = %#x, want NpcInfo only", got)
	}
	if npcInfoRunning(npcFrame{opcode: frames[0][0], raw: frames[0]}) {
		t.Fatal("NpcInfo does not show the still monster walking")
	}
}
