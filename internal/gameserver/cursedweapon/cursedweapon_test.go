package cursedweapon

import (
	"context"
	"slices"
	"sync"
	"testing"
	"time"

	"github.com/rs/zerolog"

	"github.com/fatal10110/acis_golang/internal/gameserver/model/entity"
	"github.com/fatal10110/acis_golang/internal/gameserver/model/location"
)

const (
	zariche  int32 = 8190
	akamanah int32 = 8689
)

// release is one ReleaseHolder write.
type release struct{ playerID, itemID, karma, pkKills int32 }

// fakeStore serves rows to Restore and records the writes.
type fakeStore struct {
	mu       sync.Mutex
	rows     []Row
	deleted  []int32
	released []release
}

func (s *fakeStore) Load(context.Context) ([]Row, error) { return s.rows, nil }
func (s *fakeStore) Insert(context.Context, Row) error   { return nil }
func (s *fakeStore) Update(context.Context, Row) error   { return nil }

func (s *fakeStore) Delete(_ context.Context, itemID int32) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.deleted = append(s.deleted, itemID)
	return nil
}

func (s *fakeStore) ReleaseHolder(_ context.Context, playerID, itemID, karma, pkKills int32) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.released = append(s.released, release{playerID, itemID, karma, pkKills})
	return nil
}

func testTable(t *testing.T) *entity.CursedWeaponTable {
	t.Helper()
	zar := entity.CursedWeapon{ItemID: zariche, Name: "Demonic Sword Zariche", DropRate: 1, Duration: 72, DurationLost: 24, DisappearChance: 50, StageKills: 10}
	aka := zar
	aka.ItemID, aka.Name = akamanah, "Blood Sword Akamanah"
	table, err := entity.NewCursedWeaponTable([]entity.CursedWeapon{zar, aka})
	if err != nil {
		t.Fatalf("NewCursedWeaponTable: %v", err)
	}
	return table
}

// clock is a settable clock for the manager.
type clock struct{ now time.Time }

func (c *clock) Now() time.Time { return c.now }

func newManager(t *testing.T, store *fakeStore, at time.Time) (*Manager, *clock) {
	t.Helper()
	c := &clock{now: at}
	return New(testTable(t), store, nil, zerolog.Nop(), c.Now), c
}

func always(n int) func(int) int { return func(int) int { return n } }

// TestHashOrder: the weapons are visited in the reference's hash map
// order, Akamanah (8689) before Zariche (8190), whatever order the table
// lists them in.
func TestHashOrder(t *testing.T) {
	for _, ids := range [][]int32{{zariche, akamanah}, {akamanah, zariche}} {
		if got := hashOrder(ids); !slices.Equal(got, []int32{akamanah, zariche}) {
			t.Fatalf("hashOrder(%v) = %v, want [8689 8190]", ids, got)
		}
	}
	// Past twelve ids the table doubles to 32 buckets: 16 and 48 then
	// fold to different buckets than 0 and 32.
	ids := []int32{48, 32, 16, 0, 1, 2, 3, 4, 5, 6, 7, 8, 9}
	if got := hashOrder(ids); !slices.Equal(got, []int32{0, 32, 1, 2, 3, 4, 5, 6, 7, 8, 9, 16, 48}) {
		t.Fatalf("hashOrder over 32 buckets = %v", got)
	}
	// A monster kill rolls them in that order: with every roll passing,
	// Akamanah drops first.
	m, _ := newManager(t, &fakeStore{}, time.Unix(1000, 0))
	if id, ok := m.RollDrop(always(0)); !ok || id != akamanah {
		t.Fatalf("first drop = %d, %v, want Akamanah", id, ok)
	}
	if id, ok := m.RollDrop(always(0)); !ok || id != zariche {
		t.Fatalf("second drop = %d, %v, want Zariche", id, ok)
	}
	if _, ok := m.RollDrop(always(0)); ok {
		t.Fatal("a third drop with both weapons out")
	}
}

// TestRestore: a stored weapon with time left comes back held, its hunger
// and life checks armed a minute out; one whose life ran out while the
// server was down ends at once, its row deleted and its holder released.
func TestRestore(t *testing.T) {
	now := time.Date(2026, 10, 4, 12, 0, 0, 0, time.UTC)
	store := &fakeStore{rows: []Row{
		{ItemID: zariche, PlayerID: 7, PlayerKarma: 100, PlayerPKKills: 3, CurrentStage: 2, NumberBeforeNextStage: 5, HungryTime: 600, EndTime: now.Add(5 * time.Hour).UnixMilli()},
		{ItemID: akamanah, PlayerID: 9, PlayerKarma: 40, PlayerPKKills: 1, CurrentStage: 1, HungryTime: 600, EndTime: now.Add(-time.Minute).UnixMilli()},
	}}
	m, _ := newManager(t, store, now)
	if err := m.Restore(context.Background()); err != nil {
		t.Fatalf("Restore: %v", err)
	}
	if id, stage, ok := m.Held(7); !ok || id != zariche || stage != 2 {
		t.Fatalf("Held(7) = %d, %d, %v, want Zariche at stage 2", id, stage, ok)
	}
	if left := m.TimeLeft(zariche); left != 5*time.Hour {
		t.Fatalf("TimeLeft = %v, want 5h", left)
	}
	if _, _, ok := m.Held(9); ok {
		t.Fatal("the expired Akamanah came back held")
	}
	if active := m.Active(); len(active) != 1 || active[0].ItemID != zariche || !active[0].Activated {
		t.Fatalf("Active = %+v, want Zariche held alone", active)
	}
	if !slices.Equal(store.deleted, []int32{akamanah}) {
		t.Fatalf("rows deleted = %v, want Akamanah's", store.deleted)
	}
	if !slices.Equal(store.released, []release{{9, akamanah, 40, 1}}) {
		t.Fatalf("holders released = %+v, want Akamanah's holder 9 with karma 40, 1 PK", store.released)
	}
	// Nothing is due before the first minute.
	if ends, reminders := m.Tick(now.Add(59 * time.Second)); len(ends) != 0 || len(reminders) != 0 {
		t.Fatalf("Tick before a minute = %+v, %+v, want nothing", ends, reminders)
	}
}

// TestReminderCadence: every 60th hunger check tells the holder its time
// left; the others say nothing.
func TestReminderCadence(t *testing.T) {
	now := time.Date(2026, 10, 4, 12, 0, 0, 0, time.UTC)
	store := &fakeStore{rows: []Row{{ItemID: zariche, PlayerID: 7, CurrentStage: 1, HungryTime: 600, EndTime: now.Add(50 * time.Hour).UnixMilli()}}}
	m, _ := newManager(t, store, now)
	if err := m.Restore(context.Background()); err != nil {
		t.Fatalf("Restore: %v", err)
	}
	if _, reminders := m.Tick(now.Add(59 * time.Minute)); len(reminders) != 0 {
		t.Fatalf("59 checks reminded %+v, want nothing", reminders)
	}
	_, reminders := m.Tick(now.Add(60 * time.Minute))
	if want := []Reminder{{ItemID: zariche, HolderID: 7, Left: 49 * time.Hour}}; !slices.Equal(reminders, want) {
		t.Fatalf("60th check reminded %+v, want %+v", reminders, want)
	}
	if _, reminders := m.Tick(now.Add(119 * time.Minute)); len(reminders) != 0 {
		t.Fatalf("checks 61 to 119 reminded %+v, want nothing", reminders)
	}
	// A tick that catches up checks 120 to 240 at once reminds at 120,
	// 180 and 240.
	_, reminders = m.Tick(now.Add(240 * time.Minute))
	if len(reminders) != 3 {
		t.Fatalf("checks 120 to 240 reminded %d times, want 3", len(reminders))
	}
}

// TestGroundExpiry: a weapon left an hour on the ground ends, naming the
// ground item to take out of the world; a minute short of the hour it is
// still out.
func TestGroundExpiry(t *testing.T) {
	now := time.Date(2026, 10, 4, 12, 0, 0, 0, time.UTC)
	store := &fakeStore{}
	m, _ := newManager(t, store, now)
	id, ok := m.RollDrop(always(0))
	if !ok {
		t.Fatal("no drop")
	}
	m.PlaceOnGround(id, 4242, location.Location{X: 1, Y: 2, Z: 3})
	if ends, _ := m.Tick(now.Add(59 * time.Minute)); len(ends) != 0 {
		t.Fatalf("ends after 59 minutes = %+v, want none", ends)
	}
	ends, _ := m.Tick(now.Add(time.Hour))
	if want := []EndOfLife{{ItemID: id, GroundObjectID: 4242}}; !slices.Equal(ends, want) {
		t.Fatalf("ends after an hour = %+v, want %+v", ends, want)
	}
	if len(m.Active()) != 0 {
		t.Fatalf("Active = %+v, want none", m.Active())
	}
	if len(store.released) != 0 {
		t.Fatalf("a ground weapon released a holder: %+v", store.released)
	}
}

// TestOverallLifeEndsHeldWeapon: a held weapon whose hunger is far off
// still ends once its life runs out, on the first life check at or past
// its end time, with its holder to release.
func TestOverallLifeEndsHeldWeapon(t *testing.T) {
	now := time.Date(2026, 10, 4, 12, 0, 0, 0, time.UTC)
	store := &fakeStore{rows: []Row{{ItemID: zariche, PlayerID: 7, PlayerKarma: 100, PlayerPKKills: 3, CurrentStage: 1, HungryTime: 600, EndTime: now.Add(5 * time.Minute).UnixMilli()}}}
	m, _ := newManager(t, store, now)
	if err := m.Restore(context.Background()); err != nil {
		t.Fatalf("Restore: %v", err)
	}
	if ends, _ := m.Tick(now.Add(4 * time.Minute)); len(ends) != 0 {
		t.Fatalf("ends after 4 minutes = %+v, want none", ends)
	}
	ends, _ := m.Tick(now.Add(5 * time.Minute))
	if want := []EndOfLife{{ItemID: zariche, Held: true, HolderID: 7, Karma: 100, PKKills: 3}}; !slices.Equal(ends, want) {
		t.Fatalf("ends at the end time = %+v, want %+v", ends, want)
	}
	if _, _, ok := m.Held(7); ok {
		t.Fatal("holder still holds the weapon")
	}
	if !slices.Equal(store.deleted, []int32{zariche}) {
		t.Fatalf("rows deleted = %v, want Zariche's", store.deleted)
	}
}
