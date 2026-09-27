package character

import (
	"testing"

	"github.com/fatal10110/acis_golang/internal/commons/wire"
	"github.com/fatal10110/acis_golang/internal/gameserver/model/location"
	"github.com/fatal10110/acis_golang/internal/gameserver/network/clientpackets"
	"github.com/fatal10110/acis_golang/internal/gameserver/network/serverpackets"
)

func TestRestartDropsTeardownFramesForSelfButNotWatcher(t *testing.T) {
	srv, c, watcher, objectID := bootObserverPair(t)
	monster := srv.SpawnHostileNPCAt(t, location.Location{X: 40, Y: 20, Z: 30})
	drainQuiet(t, c)
	drainQuiet(t, watcher)

	c.Send(encodeAction(monster.ObjectID(), 10, 20, 30, false))
	mustReadOpcode(t, c, serverpackets.OpcodeMyTargetSelected, "monster selection")
	drainQuiet(t, c)
	drainQuiet(t, watcher)

	c.Send(encodeSingleOpcode(clientpackets.OpcodeRequestRestart))
	if frame := c.Read(); frame[0] != serverpackets.OpcodeRestartResponse || wire.NewReader(frame[1:]).ReadInt32() != 1 {
		t.Fatalf("first restart frame = %x, want RestartResponse(true)", frame)
	}
	if frame := c.Read(); frame[0] != serverpackets.OpcodeCharSelectInfo {
		t.Fatalf("second restart frame = %x, want CharSelectInfo", frame)
	}
	if frame := c.ReadWithTimeout(rejectSilenceWindow); frame != nil {
		t.Fatalf("frame after CharSelectInfo = %x, want silence", frame)
	}

	for _, want := range []byte{serverpackets.OpcodeTargetUnselected, serverpackets.OpcodeDeleteObject} {
		frame := watcher.Read()
		if frame[0] != want || wire.NewReader(frame[1:]).ReadInt32() != objectID {
			t.Fatalf("watcher frame = %x, want opcode %#x for object %d", frame, want, objectID)
		}
	}
}
