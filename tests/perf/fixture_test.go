package perf

import (
	"context"
	"fmt"
	"strings"
	"testing"
	"time"

	"github.com/fatal10110/acis_golang/internal/gameserver/model/item"
	"github.com/fatal10110/acis_golang/internal/gameserver/network"
	"github.com/fatal10110/acis_golang/internal/gameservertest"
)

// fixtureItemID is the non-stackable, weightless sword of the harness item
// table: every fixture row is a distinct instance of it.
const fixtureItemID = 30

// fixtureBackgroundIDBase is where the fixture's filler item object ids
// start: far above the sequential object ids the harness hands out, and
// below the int32 limit for any realistic row count.
const fixtureBackgroundIDBase = 1_500_000_000

// fixtureSize is how big the fixture variant makes the database: what every
// client's character carries, and how many other characters and item rows
// the tables already hold, as a long-running server's would.
type fixtureSize struct {
	itemsPerClient  int
	backgroundChars int
	backgroundItems int
}

func fixtureSizeFromEnv(t *testing.T) fixtureSize {
	t.Helper()
	size := fixtureSize{
		itemsPerClient:  envInt(t, "ACIS_PERF_ITEMS", 120),
		backgroundChars: envInt(t, "ACIS_PERF_BG_CHARS", 2000),
		backgroundItems: envInt(t, "ACIS_PERF_BG_ITEMS", 500000),
	}
	if size.itemsPerClient < 0 || size.backgroundChars < 0 || size.backgroundItems < 0 {
		t.Fatalf("ACIS_PERF_ITEMS, ACIS_PERF_BG_CHARS and ACIS_PERF_BG_ITEMS must not be negative: %+v", size)
	}
	if size.backgroundItems > 0 && size.backgroundChars == 0 {
		t.Fatal("ACIS_PERF_BG_ITEMS needs ACIS_PERF_BG_CHARS: the filler items need owners")
	}
	return size
}

// seedFixture fills the database through bulk inserts: itemsPerClient
// inventory rows for every client character, backgroundChars filler
// characters (through the real character store) and backgroundItems filler
// item rows spread over them. It logs what it wrote and how long it took.
func seedFixture(t *testing.T, srv *gameservertest.Server, size fixtureSize, clientIDs []int32) {
	t.Helper()
	start := time.Now()
	nextID := int64(fixtureBackgroundIDBase)
	var owned []int32
	for _, id := range clientIDs {
		for range size.itemsPerClient {
			owned = append(owned, id)
		}
	}
	bulkItems(t, srv, owned, &nextID)

	bgIDs := make([]int32, size.backgroundChars)
	for i := range bgIDs {
		bgIDs[i] = srv.SeedCharacterFor(t, fmt.Sprintf("bg%d", i), fmt.Sprintf("Bg%d", i), 5, 0).ID
	}
	filler := make([]int32, size.backgroundItems)
	for i := range filler {
		filler[i] = bgIDs[i%len(bgIDs)]
	}
	bulkItems(t, srv, filler, &nextID)
	t.Logf("fixture: %d client items, %d background characters, %d background items in %s",
		len(owned), size.backgroundChars, size.backgroundItems, time.Since(start).Round(time.Millisecond))
}

// bulkItems inserts one inventory item row per owner entry, in statements of
// a thousand rows, taking object ids from *next upward.
func bulkItems(t *testing.T, srv *gameservertest.Server, owners []int32, next *int64) {
	t.Helper()
	const batch = 1000
	loc := item.LocationInventory.String()
	for len(owners) > 0 {
		n := min(batch, len(owners))
		var sb strings.Builder
		sb.WriteString("INSERT INTO items (owner_id, object_id, item_id, count, loc, loc_data) VALUES ")
		for i, owner := range owners[:n] {
			if i > 0 {
				sb.WriteByte(',')
			}
			fmt.Fprintf(&sb, "(%d,%d,%d,1,'%s',0)", owner, *next, fixtureItemID, loc)
			*next++
		}
		if _, err := srv.DB.ExecContext(context.Background(), sb.String()); err != nil {
			t.Fatalf("seed fixture items: %v", err)
		}
		owners = owners[n:]
	}
}

// measureStop stops the server with every client still connected, as a
// shutdown does: the drain of the final saves, then the stop, in which each
// connection detaches at once and its handler waits for its player's saves on
// the persistence lanes. It logs both and fails when the stop outlasts
// network.LivePlayerPersistWait, the bound a production connection waits
// for those saves before giving up on them.
func measureStop(t *testing.T, srv *gameservertest.Server, clients int) {
	t.Helper()
	drainStart := time.Now()
	srv.Drain(t)
	drain := time.Since(drainStart)
	stopStart := time.Now()
	srv.Stop()
	stop := time.Since(stopStart)
	t.Logf("  stop with %d connected clients: drain=%s detach=%s (a connection waits at most %s for its saves)",
		clients, drain.Round(time.Millisecond), stop.Round(time.Millisecond), network.LivePlayerPersistWait)
	if stop > network.LivePlayerPersistWait {
		t.Errorf("stop took %s, past the %s a connection waits for its player's saves", stop, network.LivePlayerPersistWait)
	}
}
