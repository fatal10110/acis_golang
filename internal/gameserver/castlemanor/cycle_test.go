package castlemanor

import (
	"context"
	"fmt"
	"slices"
	"sync"
	"testing"
	"time"

	"github.com/rs/zerolog"

	"github.com/fatal10110/acis_golang/internal/gameserver/castle"
	"github.com/fatal10110/acis_golang/internal/gameserver/clan"
	"github.com/fatal10110/acis_golang/internal/gameserver/model/manor"
	"github.com/fatal10110/acis_golang/internal/gameserver/model/residence"
	castledata "github.com/fatal10110/acis_golang/internal/gameserver/model/residence/castle"
	"github.com/fatal10110/acis_golang/internal/gameserver/sim"
)

// Every value of the cycle (start mode, change times, cost, rollover, days
// of the cycle with their saves and effects, reset, autosave, limits,
// DecreaseAmount) comes from testdata/manor.golden, the reference's own code
// run by testdata/oracle; manor_oracle_test.go, approval_oracle_test.go and
// cycle_oracle_test.go replay it. The tests here pin what that code reaches
// only through the database: which rows a restore keeps and a save writes,
// and that a disabled manor neither restores nor saves.

// The test manor: Gludio (1) is owned by lordsClan, led by lordsLeader;
// Dion (2) has no owner. Neither castle is taxed, so seed income lands
// whole. Seed 5016 grows crop 5073, matured as 5103; its seed reference
// price is 5. Seed 5017 grows crop 5068, matured as 5098.
const (
	gludio = 1
	dion   = 2

	lordsClan   int32 = 0x10000001
	lordsLeader int32 = 0x10000101

	codaSeed, codaCrop, codaMature int32 = 5016, 5073, 5103
	redSeed, redCrop, redMature    int32 = 5017, 5068, 5098
)

func testSeeds() *manor.Table {
	t := manor.NewTable([]manor.Manor{{ID: gludio, Name: "gludio", Seeds: []manor.Seed{
		{CropID: int(codaCrop), SeedID: int(codaSeed), MatureID: int(codaMature), CastleID: gludio, Reward1: 1864, Reward2: 1878, SeedsLimit: 8100, CropsLimit: 9000},
		{CropID: int(redCrop), SeedID: int(redSeed), MatureID: int(redMature), CastleID: gludio, Reward1: 1865, Reward2: 1879},
	}}})
	t.ApplyReferencePrices(func(id int32) (int32, bool) {
		switch id {
		case codaSeed:
			return 5, true
		case codaCrop:
			return 50, true
		}
		return 0, false
	})
	return t
}

// testCastles returns Gludio, owned by lordsClan with treasury in its
// treasury, and Dion, free.
func testCastles(t *testing.T, treasury int64) (*castle.Manager, *clan.Table) {
	t.Helper()
	var castles []*castledata.Castle
	for _, id := range []int{gludio, dion} {
		c, err := castledata.NewCastle(castledata.CastleAttrs{ID: id, Alias: fmt.Sprintf("castle_%d", id), Name: fmt.Sprintf("Castle %d", id), Tax: residence.Tax{Rate: 15}}, nil, nil, nil, nil, nil)
		if err != nil {
			t.Fatal(err)
		}
		castles = append(castles, c)
	}
	data, err := castledata.NewTable(castles)
	if err != nil {
		t.Fatal(err)
	}
	clans := clan.NewTable()
	clans.Restore(clan.Snapshot{Clans: []clan.Row{{ID: lordsClan, Name: "Lords", LeaderID: lordsLeader, CastleID: gludio}}}, time.Now(), 1)
	m := castle.NewManager(data, clans, nil, nil, zerolog.Nop())
	m.Restore([]castle.Row{{ID: gludio, Treasury: treasury}, {ID: dion}}, []castle.Owner{{ClanID: lordsClan, CastleID: gludio}})
	return m, clans
}

// recordingEffects records each effect as one line.
type recordingEffects struct {
	mu    sync.Mutex
	lines []string
}

func (e *recordingEffects) AddToClanWarehouse(clanID, itemID int32, count int) {
	e.mu.Lock()
	defer e.mu.Unlock()
	e.lines = append(e.lines, fmt.Sprintf("warehouse %#x +%d of %d", clanID, count, itemID))
}

func (e *recordingEffects) TellManorUpdated(leaderID int32) {
	e.mu.Lock()
	defer e.mu.Unlock()
	e.lines = append(e.lines, fmt.Sprintf("updated %#x", leaderID))
}

func (e *recordingEffects) take() []string {
	e.mu.Lock()
	defer e.mu.Unlock()
	out := e.lines
	e.lines = nil
	return out
}

// memStore holds the rows it is given and counts its saves.
type memStore struct {
	mu         sync.Mutex
	production []ProductionRow
	procure    []ProcureRow
	saves      int
}

func (s *memStore) Load(context.Context) ([]ProductionRow, []ProcureRow, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	return slices.Clone(s.production), slices.Clone(s.procure), nil
}

func (s *memStore) Save(_ context.Context, production []ProductionRow, procure []ProcureRow) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.production, s.procure = production, procure
	s.saves++
	return nil
}

func (s *memStore) state() ([]ProductionRow, []ProcureRow, int) {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.production, s.procure, s.saves
}

// at is a time of the test day, 5 October 2026, in UTC.
func at(hour, minute int) time.Time { return time.Date(2026, 10, 5, hour, minute, 0, 0, time.UTC) }

type harness struct {
	m       *Manager
	clock   *sim.Inline
	castles *castle.Manager
	store   *memStore
	effects *recordingEffects
}

// start runs a manor over testCastles from start, restored from store,
// rolling roll, with no periodic save.
func start(t *testing.T, start time.Time, treasury int64, store *memStore, roll int) *harness {
	t.Helper()
	cfg := DefaultConfig()
	cfg.SavePeriod = 0
	return startWith(t, cfg, start, treasury, store, roll)
}

func startWith(t *testing.T, cfg Config, start time.Time, treasury int64, store *memStore, roll int) *harness {
	t.Helper()
	return startOver(t, cfg, testSeeds(), start, treasury, store, roll)
}

// startOver is startWith over a seed table.
func startOver(t *testing.T, cfg Config, seeds *manor.Table, start time.Time, treasury int64, store *memStore, roll int) *harness {
	t.Helper()
	castles, clans := testCastles(t, treasury)
	m := New(cfg, seeds, castles, clans, store, nil, zerolog.Nop(), WithRoll(func(int) int { return roll }))
	if err := m.Restore(context.Background()); err != nil {
		t.Fatal(err)
	}
	clock := sim.NewInline(start)
	queue := clock.NewQueue("castle-manor")
	t.Cleanup(queue.Close)
	effects := &recordingEffects{}
	m.Start(queue, effects)
	return &harness{m: m, clock: clock, castles: castles, store: store, effects: effects}
}

// gludioRows is Gludio's stored manor: the running period buys 100 Dark
// Coda at 50, 40 still wanted, and 10 Red Coda at 7, 9 still wanted; the
// next period sells 10 Dark Coda seeds at 5 and buys 100 Dark Coda at 30.
// A Dion row and a row of a castle not loaded come with it.
func gludioRows() *memStore {
	return &memStore{
		production: []ProductionRow{
			{CastleID: gludio, SeedID: codaSeed, Amount: 10, StartAmount: 10, Price: 5, NextPeriod: true},
			{CastleID: dion, SeedID: redSeed, Amount: 3, StartAmount: 3, Price: 9},
			{CastleID: 42, SeedID: redSeed, Amount: 3, StartAmount: 3, Price: 9},
		},
		procure: []ProcureRow{
			{CastleID: gludio, CropID: codaCrop, Amount: 40, StartAmount: 100, Price: 50, RewardType: 1},
			{CastleID: gludio, CropID: redCrop, Amount: 9, StartAmount: 10, Price: 7, RewardType: 2},
			{CastleID: gludio, CropID: codaCrop, Amount: 100, StartAmount: 100, Price: 30, RewardType: 1, NextPeriod: true},
		},
	}
}

// After a reset the next save stores no row for the castle.
func TestResetThenSaveStoresNoRows(t *testing.T) {
	t.Parallel()
	h := start(t, at(12, 0), 0, gludioRows(), 0)
	h.m.Reset(gludio)
	if err := h.m.Stop(context.Background()); err != nil {
		t.Fatal(err)
	}
	production, procure, _ := h.store.state()
	if len(procure) != 0 || len(production) != 1 || production[0].CastleID != dion {
		t.Fatalf("saved rows = %+v / %+v, want Dion's seed alone", production, procure)
	}
}

// Restore splits the rows by period and leaves out a castle not loaded;
// a save writes them back castle by castle, the running period first.
func TestRestoreThenSaveRoundTrips(t *testing.T) {
	t.Parallel()
	rows := gludioRows()
	h := start(t, at(12, 0), 0, rows, 0)
	if got := len(h.m.CropProcure(gludio, false)); got != 2 {
		t.Fatalf("running crops = %d, want 2", got)
	}
	if err := h.m.Stop(context.Background()); err != nil {
		t.Fatal(err)
	}
	production, procure, _ := h.store.state()
	wantProduction := []ProductionRow{
		{CastleID: gludio, SeedID: codaSeed, Amount: 10, StartAmount: 10, Price: 5, NextPeriod: true},
		{CastleID: dion, SeedID: redSeed, Amount: 3, StartAmount: 3, Price: 9},
	}
	wantProcure := gludioRows().procure
	if !slices.Equal(production, wantProduction) || !slices.Equal(procure, wantProcure) {
		t.Fatalf("saved = %+v / %+v, want %+v / %+v", production, procure, wantProduction, wantProcure)
	}
}

// A disabled manor holds nothing, keeps StatusDisabled, and neither
// restores nor saves.
func TestDisabledManorDoesNothing(t *testing.T) {
	t.Parallel()
	castles, clans := testCastles(t, 0)
	cfg := DefaultConfig()
	cfg.Enabled = false
	store := gludioRows()
	m := New(cfg, testSeeds(), castles, clans, store, nil, zerolog.Nop())
	if err := m.Restore(context.Background()); err != nil {
		t.Fatal(err)
	}
	m.Start(sim.NewInline(at(12, 0)).NewQueue("castle-manor"), &recordingEffects{})
	m.Reset(gludio)
	if err := m.Stop(context.Background()); err != nil {
		t.Fatal(err)
	}
	if m.Status() != StatusDisabled || m.SeedProduction(gludio, false) != nil {
		t.Fatalf("disabled manor: status %s, lists %v", m.Status(), m.SeedProduction(gludio, false))
	}
	if _, _, saves := store.state(); saves != 0 {
		t.Fatalf("disabled manor saved %d times", saves)
	}
}
