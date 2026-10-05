package network

import (
	"context"
	"fmt"
	"slices"
	"sync"
	"testing"

	"github.com/rs/zerolog"

	"github.com/fatal10110/acis_golang/internal/gameserver/model/item"
	"github.com/fatal10110/acis_golang/internal/gameserver/persist"
	"github.com/fatal10110/acis_golang/internal/gameserver/task"
)

// landingFlusher records every row the item persistence task flushes. No
// tick runs in these tests, so a row it records was landed by the bank's
// Landing and by nothing else.
type landingFlusher struct {
	mu   sync.Mutex
	rows []string
}

func (f *landingFlusher) Flush(_ context.Context, batch item.FlushBatch) error {
	f.mu.Lock()
	defer f.mu.Unlock()
	for _, st := range batch.Saves {
		f.rows = append(f.rows, fmt.Sprintf("save(%d,owner %d,%d)", st.ObjectID, st.OwnerID, st.Count))
	}
	for _, id := range batch.Deletes {
		f.rows = append(f.rows, fmt.Sprintf("delete(%d)", id))
	}
	return nil
}

func (f *landingFlusher) flushed() []string {
	f.mu.Lock()
	defer f.mu.Unlock()
	return slices.Clone(f.rows)
}

const (
	landingClan  int32 = 0x10000040
	landingAdena int32 = 7000
)

// newLandingLink is a link whose clan landingClan's warehouse, in the item
// store, holds adena (none for 0), with a real item persistence task flushing
// to the returned flusher and no tick.
func newLandingLink(t *testing.T, adena int) (*GameClientLink, *landingFlusher) {
	t.Helper()
	store := newFakeItemStore()
	if adena > 0 {
		row := item.Instance{ObjectID: landingAdena, TemplateID: item.AdenaID, OwnerID: landingClan, Count: adena, Location: item.LocationClanWarehouse}
		if err := store.Create(context.Background(), landingClan, row); err != nil {
			t.Fatal(err)
		}
	}
	flusher := &landingFlusher{}
	link := &GameClientLink{
		itemTemplates: testItemTemplates(),
		items:         store,
		itemWrites:    persist.NewOrder(),
		ids:           &sequentialIDs{next: 9000},
		log:           zerolog.Nop(),
	}
	link.itemInstances = task.NewItemInstances(flusher, link.itemTemplates, nil, link.itemWrites, zerolog.Nop())
	return link, flusher
}

// land runs landing and returns what it flushed.
func land(t *testing.T, landing func(context.Context) error, flusher *landingFlusher) []string {
	t.Helper()
	if landing == nil {
		t.Fatal("no landing returned: the adena row is left to the item tick, which a bid or sale row can overtake")
	}
	if got := flusher.flushed(); len(got) != 0 {
		t.Fatalf("rows flushed before the landing ran: %v", got)
	}
	if err := landing(context.Background()); err != nil {
		t.Fatalf("landing: %v", err)
	}
	return flusher.flushed()
}

// A fee taken from the clan warehouse is in the database once its landing
// has run: the stack with what is left, or its delete when the fee took all
// of it.
func TestTakeAdenaLandingWritesTheWarehouseAdenaRow(t *testing.T) {
	for _, tc := range []struct {
		name string
		fee  int
		want []string
	}{
		{"part of the stack", 3000, []string{fmt.Sprintf("save(%d,owner %d,2000)", landingAdena, landingClan)}},
		{"the whole stack", 5000, []string{fmt.Sprintf("delete(%d)", landingAdena)}},
	} {
		t.Run(tc.name, func(t *testing.T) {
			link, flusher := newLandingLink(t, 5000)
			landing, ok := link.TakeAdena(landingClan, tc.fee)
			if !ok {
				t.Fatal("TakeAdena refused a fee the warehouse covers")
			}
			if got := land(t, landing, flusher); !slices.Equal(got, tc.want) {
				t.Fatalf("landing flushed %v, want %v", got, tc.want)
			}
		})
	}
}

// A refund to the clan warehouse is in the database once its landing has
// run: the stack it merged into with the new count, or the new stack's
// insert when the warehouse held no adena.
func TestReturnAdenaLandingWritesTheWarehouseAdenaRow(t *testing.T) {
	for _, tc := range []struct {
		name string
		held int
		want []string
	}{
		{"merged into the stack", 5000, []string{fmt.Sprintf("save(%d,owner %d,7700)", landingAdena, landingClan)}},
		{"a new stack", 0, []string{fmt.Sprintf("save(9001,owner %d,2700)", landingClan)}},
	} {
		t.Run(tc.name, func(t *testing.T) {
			link, flusher := newLandingLink(t, tc.held)
			landing := link.ReturnAdena(landingClan, 2700)
			if got := land(t, landing, flusher); !slices.Equal(got, tc.want) {
				t.Fatalf("landing flushed %v, want %v", got, tc.want)
			}
		})
	}
}
