package lifecycle

import (
	"bytes"
	"sync"
	"testing"

	"github.com/rs/zerolog"

	"github.com/fatal10110/acis_golang/internal/gameserver/model/item"
	"github.com/fatal10110/acis_golang/internal/gameserver/network/serverpackets"
	"github.com/fatal10110/acis_golang/internal/gameservertest"
)

// logBuffer collects both boots' server logs so a failing restart test can
// show what the stack actually reported (handler errors are logged, not
// fatal).
type logBuffer struct {
	mu  sync.Mutex
	buf bytes.Buffer
}

func (b *logBuffer) Write(p []byte) (int, error) {
	b.mu.Lock()
	defer b.mu.Unlock()
	return b.buf.Write(p)
}

func startLoggedBoot(t *testing.T, opts ...gameservertest.Option) *gameservertest.Server {
	t.Helper()
	logs := &logBuffer{}
	srv := gameservertest.Boot(t, append(opts, gameservertest.WithLog(zerolog.New(logs)))...)
	t.Cleanup(func() {
		if !t.Failed() {
			return
		}
		logs.mu.Lock()
		defer logs.mu.Unlock()
		if logs.buf.Len() > 0 {
			t.Logf("server log:\n%s", logs.buf.String())
		}
	})
	return srv
}

// TestGroundItemsRestartRoundTrip drops an item, shuts the server down, and
// boots a fresh stack on the same database: the drop must survive in
// items_on_ground, be restored into world state at boot (with the rows then
// cleared so nothing double-restores), and be fully usable — the entering
// client picks it up and the adena is back in one inventory stack.
func TestGroundItemsRestartRoundTrip(t *testing.T) {
	srv := startLoggedBoot(t, gameservertest.WithCharacter("Newbie", 1, 0), gameservertest.WithWantChars(1))
	c := srv.Client
	objID := srv.SoleObjectID(t)
	adena := srv.GiveItem(t, objID, item.AdenaID, 100)
	startInWorld(t, c)

	c.Send(encodeRequestDropItem(adena, 40, spawnX, spawnY, spawnZ))
	groundID := readDropItemGroundID(t, c.Read(), objID, item.AdenaID, 40)

	srv.Shutdown(t)

	rows := groundRows(t, srv)
	if len(rows) != 1 || rows[0].ObjectID != groundID || rows[0].Count != 40 {
		t.Fatalf("items_on_ground rows after shutdown = %+v, want one adena row object %d count 40", rows, groundID)
	}

	srv2 := startLoggedBoot(t, gameservertest.WithWantChars(1))
	if _, ok := srv2.State.Object(groundID); !ok {
		t.Fatal("second boot did not restore the dropped item into world state")
	}
	if got := srv2.GroundItems.Len(); got != 1 {
		t.Fatalf("second boot restored GroundItems.Len() = %d, want 1", got)
	}
	if rows := groundRows(t, srv2); len(rows) != 0 {
		t.Fatalf("items_on_ground rows after second boot = %+v, want empty (rows must clear once hydrated)", rows)
	}

	c2 := srv2.Client
	startInWorld(t, c2)

	c2.Send(encodeAction(groundID, spawnX, spawnY, spawnZ, false))
	assertFrameOpcode(t, c2.Read(), serverpackets.OpcodeActionFailed, "pickup pending-action release")
	assertFrameOpcode(t, c2.Read(), serverpackets.OpcodeGetItem, "GetItem")
	assertFrameOpcode(t, c2.Read(), serverpackets.OpcodeDeleteObject, "pickup DeleteObject")

	srv2.InventoryUpdates.Tick()
	readInventoryUpdateFor(t, c2, adena, 100)

	if _, ok := srv2.State.Object(groundID); ok {
		t.Fatalf("world.Object(%d) still present after pickup", groundID)
	}
	if rows := groundRows(t, srv2); len(rows) != 0 {
		t.Fatalf("items_on_ground rows after pickup = %+v, want none", rows)
	}
	if counts := persistedAdena(t, srv2, objID); len(counts) != 1 || counts[0] != 100 {
		t.Fatalf("persisted adena stacks after pickup = %v, want one stack of 100 back", counts)
	}
}
