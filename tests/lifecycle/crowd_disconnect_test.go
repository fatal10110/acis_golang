package lifecycle

import (
	"fmt"
	"slices"
	"testing"

	"github.com/fatal10110/acis_golang/internal/commons/wire"
	"github.com/fatal10110/acis_golang/internal/gameserver/network/serverpackets"
	"github.com/fatal10110/acis_golang/internal/gameservertest"
	"github.com/fatal10110/acis_golang/internal/testsupport"
)

// A player dropping its connection in a crowd leaves every watcher's view the
// way it always has: its own selection cleared (TargetUnselected) before its
// DeleteObject, and a watcher that had it selected told its own selection
// cleared before the DeleteObject too.
func TestDroppedConnectionInCrowdKeepsWatcherFrameOrder(t *testing.T) {
	t.Parallel()
	srv := gameservertest.Boot(t,
		gameservertest.WithCharacter("Leaver", 5, 0),
		gameservertest.WithWantChars(1),
	)
	leaver, leaverID := srv.Client, srv.SoleObjectID(t)
	startInWorld(t, leaver)
	watchers := make([]*testsupport.ScriptedClient, 3)
	watcherIDs := make([]int32, len(watchers))
	for i := range watchers {
		account := fmt.Sprintf("crowd%d", i)
		watcherIDs[i] = srv.SeedCharacterFor(t, account, fmt.Sprintf("Crowd%d", i), 1, 0).ID
		watchers[i] = srv.DialClient(t, account, 1)
		startInWorld(t, watchers[i])
	}
	x, y, z := srv.PlayerPosition(t, leaverID)
	watchers[0].Send(encodeAction(leaverID, int32(x), int32(y), int32(z), false))
	readUntilOpcode(t, watchers[0], serverpackets.OpcodeMyTargetSelected)
	leaver.Send(encodeAction(watcherIDs[1], int32(x), int32(y), int32(z), false))
	readUntilOpcode(t, leaver, serverpackets.OpcodeMyTargetSelected)
	for _, w := range watchers {
		drainUntilQuiet(t, w)
	}

	if err := leaver.Close(); err != nil {
		t.Fatal(err)
	}
	srv.AdvanceUntil(t, "dropped player out of the world", func() bool {
		_, ok := srv.State.Player(leaverID)
		return !ok
	})
	srv.AwaitConnectionEnded(t, leaver)

	for i, w := range watchers {
		var unselected []int32
		for {
			frame := w.Read()
			if len(frame) < 5 {
				continue
			}
			id := wire.NewReader(frame[1:]).ReadInt32()
			if frame[0] == serverpackets.OpcodeTargetUnselected {
				unselected = append(unselected, id)
			}
			if frame[0] == serverpackets.OpcodeDeleteObject && id == leaverID {
				break
			}
		}
		if !slices.Contains(unselected, leaverID) {
			t.Errorf("watcher %d: leaver's DeleteObject without its TargetUnselected first (unselected %v)", i, unselected)
		}
		if i == 0 && !slices.Contains(unselected, watcherIDs[0]) {
			t.Errorf("watcher 0: leaver's DeleteObject before its own selection was cleared (unselected %v)", unselected)
		}
	}
	if _, ok := srv.State.PlayerByName("Leaver"); ok {
		t.Fatal("leaver still in world state after its connection ended")
	}
}
