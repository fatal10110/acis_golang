package party

import (
	"strconv"
	"testing"
	"time"

	"github.com/fatal10110/acis_golang/internal/commons/wire"
	gamexml "github.com/fatal10110/acis_golang/internal/gameserver/data/xml"
	"github.com/fatal10110/acis_golang/internal/gameserver/model/actor/npc"
	"github.com/fatal10110/acis_golang/internal/gameserver/model/location"
	"github.com/fatal10110/acis_golang/internal/gameserver/network/clientpackets"
	"github.com/fatal10110/acis_golang/internal/gameserver/network/serverpackets"
	"github.com/fatal10110/acis_golang/internal/gameservertest"
	"github.com/fatal10110/acis_golang/internal/testsupport/datapack"
)

// TestObserverLeavesItsParty pins observer entry from a party of three:
// the observer is expelled (HAVE_BEEN_EXPELLED_FROM_PARTY, its window
// cleared), the others are told who was expelled and drop it from their
// windows, then see it stand, jump away and vanish. Back from the
// viewpoint, it is shown to them again.
func TestObserverLeavesItsParty(t *testing.T) {
	groups, err := gamexml.LoadObserverGroups(datapack.Path(t, "data", "xml", "observerGroups.xml"))
	if err != nil {
		t.Fatalf("load observer groups: %v", err)
	}
	g := bootGroup(t, []seat{{"Leader", 20}, {"Member", 30}, {"Third", 40}},
		gameservertest.WithObserverGroups(groups), gameservertest.WithReuseDelays(3*time.Second, 0))
	g.invite(t, 0, 1, 0)
	g.invite(t, 0, 2, 0)
	third := g.players[2]
	x, y, z := g.srv.PlayerPosition(t, third.id)
	tower := g.srv.SpawnFolkNPCAt(t, gameservertest.FolkTemplate("Folk", 31031), location.Location{X: x + 30, Y: y, Z: z})
	tower.SetObserverGroups([]int{618})
	g.quiet(t)

	// Select, talk, open group 618 (free seats), then take seat 632.
	third.c.Send(encodeAction(tower.ObjectID(), x, y, z))
	third.c.Send(encodeAction(tower.ObjectID(), x, y, z))
	drainFrames(t, third.c)
	third.c.Send(encodeBypass(tower, "observe_group 618"))
	drainFrames(t, third.c)
	g.quiet(t)
	third.c.Send(encodeBypass(tower, "observe 632"))

	mine := drainFrames(t, third.c)
	assertOpcodes(t, mine[:2], []byte{serverpackets.OpcodeSystemMessage, serverpackets.OpcodePartySmallWindowDeleteAll}, "observer")
	assertStaticSystemMessage(t, mine[0], serverpackets.SystemMessageHaveBeenExpelledFromParty)
	if _, ok := firstFrame(mine, serverpackets.OpcodeObserverStart); !ok {
		t.Fatalf("observer frames = %x, want ObserverStart", opcodes(mine))
	}
	for _, p := range g.players[:2] {
		frames := drainFrames(t, p.c)
		assertSystemMessageText(t, frames[0], serverpackets.SystemMessageS1WasExpelledFromParty, "Third")
		if id, name := readWindowDelete(t, frames[1]); id != third.id || name != "Third" {
			t.Fatalf("%s PartySmallWindowDelete = %d %q", p.name, id, name)
		}
		if _, ok := firstFrame(frames, serverpackets.OpcodeDeleteObject); !ok {
			t.Fatalf("%s frames = %x, want the observer deleted", p.name, opcodes(frames))
		}
	}

	third.c.Send(encodeSingle(clientpackets.OpcodeAppearing))
	g.quiet(t)
	third.c.Send(encodeSingle(clientpackets.OpcodeObserverReturn))
	drainFrames(t, third.c)
	third.c.Send(encodeSingle(clientpackets.OpcodeAppearing))
	drainFrames(t, third.c)
	for _, p := range g.players[:2] {
		if _, ok := firstFrame(drainFrames(t, p.c), serverpackets.OpcodeCharInfo); !ok {
			t.Fatalf("%s did not see the observer come back", p.name)
		}
	}
}

func encodeBypass(f *npc.Folk, command string) []byte {
	w := wire.NewPacketWriter(clientpackets.OpcodeRequestBypassToServer)
	w.WriteString("npc_" + strconv.Itoa(int(f.ObjectID())) + "_" + command)
	return w.Bytes()
}

func firstFrame(frames [][]byte, opcode byte) ([]byte, bool) {
	for _, f := range frames {
		if f[0] == opcode {
			return f, true
		}
	}
	return nil, false
}
