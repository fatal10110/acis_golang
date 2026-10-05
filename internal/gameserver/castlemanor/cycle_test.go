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

// Reference: CastleManorManager (constructor mode, scheduleModeChange,
// changeMode, getManorCost, resetManorData, storeMe), SeedProduction and
// CropProcure. Expected values are worked by hand from those methods, not
// from the Go code under test.

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
	castles, clans := testCastles(t, treasury)
	m := New(cfg, testSeeds(), castles, clans, store, nil, zerolog.Nop(), WithRoll(func(int) int { return roll }))
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

// TestStatusAt pins the mode a manor starts in by time of day, with the
// shipped times: refresh 20:00, approve 06:00, maintenance 6 minutes. A
// start at a minute below the maintenance end of an hour past the refresh
// hour starts approved.
func TestStatusAt(t *testing.T) {
	t.Parallel()
	for _, tc := range []struct {
		hour, minute int
		want         Status
	}{
		{0, 0, StatusModifiable},
		{5, 59, StatusModifiable},
		{6, 0, StatusModifiable},
		{6, 1, StatusApproved},
		{12, 30, StatusApproved},
		{19, 59, StatusApproved},
		{20, 0, StatusMaintenance},
		{20, 5, StatusMaintenance},
		{20, 6, StatusModifiable},
		{21, 3, StatusApproved},
		{21, 6, StatusModifiable},
		{23, 5, StatusApproved},
		{23, 59, StatusModifiable},
	} {
		if got := statusAt(DefaultConfig(), at(tc.hour, tc.minute)); got != tc.want {
			t.Errorf("statusAt(%02d:%02d) = %s, want %s", tc.hour, tc.minute, got, tc.want)
		}
	}
}

// TestNextChangeAt pins when each mode changes: second 0 of the change
// minute, the milliseconds of now kept; a modifiable manor past today's
// approve time waits for tomorrow's; the other modes keep today's time
// even when it has passed.
func TestNextChangeAt(t *testing.T) {
	t.Parallel()
	ms := 250 * time.Millisecond
	for _, tc := range []struct {
		status Status
		now    time.Time
		want   time.Time
	}{
		{StatusModifiable, at(5, 0).Add(30*time.Second + ms), at(6, 0).Add(ms)},
		{StatusModifiable, at(20, 6).Add(ms), at(6, 0).AddDate(0, 0, 1).Add(ms)},
		{StatusMaintenance, at(20, 2).Add(ms), at(20, 6).Add(ms)},
		{StatusApproved, at(6, 0).Add(ms), at(20, 0).Add(ms)},
		{StatusApproved, at(21, 3).Add(ms), at(20, 0).Add(ms)},
	} {
		if got := nextChangeAt(DefaultConfig(), tc.status, tc.now); !got.Equal(tc.want) {
			t.Errorf("nextChangeAt(%s, %s) = %s, want %s", tc.status, tc.now, got, tc.want)
		}
	}
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

// TestCycleRollsThePeriodOver drives a day of the cycle from noon: the
// rollover at 20:00 pays the bought crops into the owner's warehouse and
// the money left for unsold crops back as seed income, makes the next
// period's lists the running ones and saves them; the end of maintenance
// at 20:06 tells the owner's leader; the approval at 06:00 changes only
// the mode.
func TestCycleRollsThePeriodOver(t *testing.T) {
	t.Parallel()
	h := start(t, at(12, 0), 5000, gludioRows(), 89)
	if got := h.m.Status(); got != StatusApproved {
		t.Fatalf("status at noon = %s, want APPROVED", got)
	}
	if got := h.m.NextModeChange(); !got.Equal(at(20, 0)) {
		t.Fatalf("next change = %s, want 20:00", got)
	}

	h.clock.Advance(8 * time.Hour)
	if got := h.m.Status(); got != StatusMaintenance {
		t.Fatalf("status at 20:00 = %s, want MAINTENANCE", got)
	}
	// Dark Coda: 60 bought, (int)(60 * 0.9) = 54. Red Coda: 1 bought,
	// (int)(0.9) = 0, and the roll of 89 < 90 makes it 1.
	if got, want := h.effects.take(), []string{
		"warehouse 0x10000001 +54 of 5103",
		"warehouse 0x10000001 +1 of 5098",
	}; !slices.Equal(got, want) {
		t.Fatalf("rollover effects = %q, want %q", got, want)
	}
	// 40 * 50 + 9 * 7 = 2063 back to an untaxed castle.
	gl, _ := h.castles.Get(gludio)
	if got := gl.SeedIncome(); got != 2063 {
		t.Fatalf("Gludio seed income = %d, want 2063", got)
	}
	if got := ids(h.m.SeedProduction(gludio, false)); !slices.Equal(got, []int32{codaSeed}) {
		t.Fatalf("running seeds = %v, want the former next period's", got)
	}
	cur := h.m.CropProcure(gludio, false)
	if len(cur) != 1 || cur[0].ID != codaCrop || cur[0].Price != 30 {
		t.Fatalf("running crops = %+v, want the former next period's", cur)
	}
	// Treasury 5000 covers the new running cost of 10*5 + 100*30 = 3050:
	// the next period starts over from the same lists.
	if got := ids(h.m.SeedProduction(gludio, true)); !slices.Equal(got, []int32{codaSeed}) {
		t.Fatalf("next seeds = %v, want the running ones again", got)
	}
	// Dion has no owner: its lists stay as they were.
	if got := ids(h.m.SeedProduction(dion, false)); !slices.Equal(got, []int32{redSeed}) {
		t.Fatalf("Dion running seeds = %v, want them kept", got)
	}
	if _, _, saves := h.store.state(); saves != 1 {
		t.Fatalf("saves after the rollover = %d, want 1", saves)
	}
	if got := h.m.NextModeChange(); !got.Equal(at(20, 6)) {
		t.Fatalf("next change = %s, want 20:06", got)
	}

	h.clock.Advance(6 * time.Minute)
	if got := h.m.Status(); got != StatusModifiable {
		t.Fatalf("status at 20:06 = %s, want MODIFIABLE", got)
	}
	if got, want := h.effects.take(), []string{"updated 0x10000101"}; !slices.Equal(got, want) {
		t.Fatalf("maintenance end effects = %q, want %q", got, want)
	}
	if got, want := h.m.NextModeChange(), at(6, 0).AddDate(0, 0, 1); !got.Equal(want) {
		t.Fatalf("next change = %s, want %s", got, want)
	}

	h.clock.Advance(10 * time.Hour)
	if got := h.m.Status(); got != StatusApproved {
		t.Fatalf("status at 06:00 = %s, want APPROVED", got)
	}
	if got := h.effects.take(); len(got) != 0 {
		t.Fatalf("approval effects = %q, want none", got)
	}
	if got := gl.Treasury(); got != 5000 {
		t.Fatalf("treasury after approval = %d, want 5000 untouched", got)
	}
}

// A rollover leaves the running and next periods holding the same seeds:
// a sale from the running period shows in the next period's amount until
// the next rollover puts it back at its start amount.
func TestRolloverSharesTheListsBetweenPeriods(t *testing.T) {
	t.Parallel()
	h := start(t, at(12, 0), 5000, gludioRows(), 0)
	h.clock.Advance(8 * time.Hour)
	sold, ok := h.m.SeedProduct(gludio, codaSeed, false)
	if !ok || !sold.DecreaseAmount(3) {
		t.Fatal("cannot sell 3 Dark Coda seeds")
	}
	next, _ := h.m.SeedProduct(gludio, codaSeed, true)
	if got := next.Amount(); got != 7 {
		t.Fatalf("next period amount = %d, want 7 (shared with the running period)", got)
	}
	h.clock.Advance(24 * time.Hour)
	if got := sold.Amount(); got != 10 {
		t.Fatalf("amount after the next rollover = %d, want its start amount 10", got)
	}
}

// A treasury short of the new running period's cost leaves the next period
// empty.
func TestRolloverEmptiesTheNextPeriodOfAPoorCastle(t *testing.T) {
	t.Parallel()
	h := start(t, at(19, 0), 3049, gludioRows(), 90)
	h.clock.Advance(time.Hour)
	if got := h.m.SeedProduction(gludio, true); len(got) != 0 {
		t.Fatalf("next seeds = %v, want none", ids(got))
	}
	if got := h.m.CropProcure(gludio, true); len(got) != 0 {
		t.Fatalf("next crops = %+v, want none", got)
	}
	// The roll of 90 leaves the single Red Coda bought unpaid.
	if got, want := h.effects.take(), []string{"warehouse 0x10000001 +54 of 5103"}; !slices.Equal(got, want) {
		t.Fatalf("rollover effects = %q, want %q", got, want)
	}
}

// A start at 21:03 is approved past the day's refresh time, so the period
// rolls over at once.
func TestApprovedStartPastTheRefreshRollsOverAtOnce(t *testing.T) {
	t.Parallel()
	h := start(t, at(21, 3), 5000, gludioRows(), 0)
	h.clock.Advance(0)
	if got := h.m.Status(); got != StatusModifiable {
		t.Fatalf("status = %s, want MODIFIABLE after an immediate rollover and maintenance", got)
	}
	if _, _, saves := h.store.state(); saves != 1 {
		t.Fatalf("saves = %d, want the rollover's", saves)
	}
}

// The lists are saved every SavePeriod.
func TestPeriodicSave(t *testing.T) {
	t.Parallel()
	h := startWith(t, DefaultConfig(), at(6, 30), 0, gludioRows(), 0)
	h.clock.Advance(4*time.Hour + time.Minute)
	if _, _, saves := h.store.state(); saves != 2 {
		t.Fatalf("saves after 4 hours = %d, want 2", saves)
	}
}

// TestManorCost pins the cost of a period: each seed's reference price,
// or 1 for a seed the table does not hold, and each crop's price, times
// the start amount, each product a 32-bit product.
func TestManorCost(t *testing.T) {
	t.Parallel()
	h := start(t, at(12, 0), 0, &memStore{}, 0)
	h.m.SetNextSeedProduction(gludio, []*Production{
		NewProduction(codaSeed, 10, 6, 10), // 5 * 10
		NewProduction(9999, 7, 1, 7),       // 1
	})
	h.m.SetNextCropProcure(gludio, []*Procure{
		NewProcure(codaCrop, 100, 30, 100, 1),        // 3000
		NewProcure(redCrop, 30000, 100000, 30000, 2), // 3e9 wraps to -1294967296
	})
	if got, want := h.m.ManorCost(gludio, true), int64(50+1+3000-1294967296); got != want {
		t.Fatalf("ManorCost = %d, want %d", got, want)
	}
	if got := h.m.ManorCost(gludio, false); got != 0 {
		t.Fatalf("running ManorCost = %d, want 0", got)
	}
}

// Reset empties both periods' lists; the next save stores none for the
// castle.
func TestResetEmptiesTheCastlesLists(t *testing.T) {
	t.Parallel()
	h := start(t, at(12, 0), 0, gludioRows(), 0)
	h.m.Reset(gludio)
	for _, next := range []bool{false, true} {
		if len(h.m.SeedProduction(gludio, next)) != 0 || len(h.m.CropProcure(gludio, next)) != 0 {
			t.Fatalf("Gludio lists (next=%t) not emptied", next)
		}
	}
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

// The seed and crop limits scale with the crop rate.
func TestLimitsScaleWithTheCropRate(t *testing.T) {
	t.Parallel()
	castles, clans := testCastles(t, 0)
	cfg := DefaultConfig()
	cfg.CropRate = 3
	m := New(cfg, testSeeds(), castles, clans, nil, nil, zerolog.Nop())
	seed, _ := m.Seeds().Seed(codaSeed)
	if got := m.SeedsLimit(seed); got != 24300 {
		t.Fatalf("SeedsLimit = %d, want 24300", got)
	}
	if got := m.CropsLimit(seed); got != 27000 {
		t.Fatalf("CropsLimit = %d, want 27000", got)
	}
}

// DecreaseAmount takes only what is left.
func TestDecreaseAmount(t *testing.T) {
	t.Parallel()
	p := NewProduction(codaSeed, 5, 6, 10)
	if !p.DecreaseAmount(5) || p.Amount() != 0 {
		t.Fatalf("taking all 5: amount %d", p.Amount())
	}
	if p.DecreaseAmount(1) || p.Amount() != 0 {
		t.Fatalf("taking 1 of none succeeded: amount %d", p.Amount())
	}
}

func ids(list []*Production) []int32 {
	out := make([]int32, 0, len(list))
	for _, p := range list {
		out = append(out, p.ID)
	}
	return out
}
