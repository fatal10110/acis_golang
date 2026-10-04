package castle

import (
	"context"
	"fmt"
	"math"
	"slices"
	"sync"
	"testing"
	"time"

	"github.com/fatal10110/acis_golang/internal/gameserver/clan"
	"github.com/fatal10110/acis_golang/internal/gameserver/model/residence"
	castledata "github.com/fatal10110/acis_golang/internal/gameserver/model/residence/castle"
	"github.com/rs/zerolog"
)

// recordingStore records each write as one line.
type recordingStore struct {
	mu    sync.Mutex
	lines []string
}

func (s *recordingStore) add(format string, args ...any) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.lines = append(s.lines, fmt.Sprintf(format, args...))
	return nil
}

func (s *recordingStore) take() []string {
	s.mu.Lock()
	defer s.mu.Unlock()
	out := s.lines
	s.lines = nil
	return out
}

func (s *recordingStore) UpdateTreasury(_ context.Context, id int32, v int64) error {
	return s.add("treasury %d=%d", id, v)
}

func (s *recordingStore) UpdateTaxRevenue(_ context.Context, id int32, v int64) error {
	return s.add("taxRevenue %d=%d", id, v)
}

func (s *recordingStore) UpdateSeedIncome(_ context.Context, id int32, v int64) error {
	return s.add("seedIncome %d=%d", id, v)
}

func (s *recordingStore) UpdateCertificates(_ context.Context, id int32, n int) error {
	return s.add("certificates %d=%d", id, n)
}

func (s *recordingStore) UpdateCurrentTax(_ context.Context, id int32, p int) error {
	return s.add("currentTax %d=%d", id, p)
}

func (s *recordingStore) UpdateNextTax(_ context.Context, id int32, p int) error {
	return s.add("nextTax %d=%d", id, p)
}

func (s *recordingStore) UpdateFinances(_ context.Context, id int32, f Finances) error {
	return s.add("finances %d=%d/%d/%d/%d/%d", id, f.Treasury, f.TaxRevenue, f.SeedIncome, f.CurrentTaxPercent, f.NextTaxPercent)
}

func (s *recordingStore) UpdateOwner(_ context.Context, id, clanID int32) error {
	return s.add("owner %d=%d", id, clanID)
}

func (s *recordingStore) UnequipCirclets(_ context.Context, circlet, owner int32) error {
	return s.add("circlets %d of %d", circlet, owner)
}

// The shipped Gludio (1) and Aden (5) tax settings: Gludio pays Aden a
// tribute.
func testCastles(t *testing.T) *castledata.Table {
	t.Helper()
	gludio, err := castledata.NewCastle(castledata.CastleAttrs{
		ID: 1, ParentID: 5, CircletID: 6838, Alias: "gludio_castle", Name: "Gludio Castle",
		Tax: residence.Tax{Rate: 15, SysgetRate: 40, TributeRate: 25},
	}, nil, nil, nil, nil, nil)
	if err != nil {
		t.Fatal(err)
	}
	aden, err := castledata.NewCastle(castledata.CastleAttrs{
		ID: 5, CircletID: 6840, Alias: "aden_castle", Name: "Aden Castle",
		Tax: residence.Tax{Rate: 15, SysgetRate: 40},
	}, nil, nil, nil, nil, nil)
	if err != nil {
		t.Fatal(err)
	}
	table, err := castledata.NewTable([]*castledata.Castle{gludio, aden})
	if err != nil {
		t.Fatal(err)
	}
	return table
}

// Clan ids of the test clans.
const (
	lords  int32 = 0x10000001
	rivals int32 = 0x10000002
	kings  int32 = 0x10000003
)

func testClans() *clan.Table {
	table := clan.NewTable()
	table.Restore(clan.Snapshot{Clans: []clan.Row{
		{ID: lords, Name: "Lords"}, {ID: rivals, Name: "Rivals"}, {ID: kings, Name: "Kings"},
	}}, time.Now(), 1)
	return table
}

// newTestManager restores the two castles at the shipped default row,
// Gludio owned by owner and Aden by parentOwner (0 for none).
func newTestManager(t *testing.T, owner, parentOwner int32) (*Manager, *recordingStore, *clan.Table) {
	t.Helper()
	store := &recordingStore{}
	clans := testClans()
	m := NewManager(testCastles(t), clans, store, nil, zerolog.Nop())
	var owners []Owner
	for _, o := range []Owner{{owner, 1}, {parentOwner, 5}} {
		if o.ClanID != 0 {
			cl, _ := clans.Get(o.ClanID)
			cl.SetCastleID(o.CastleID)
			owners = append(owners, o)
		}
	}
	m.Restore([]Row{
		{ID: 1, CurrentTaxPercent: 15, NextTaxPercent: 15, LeftCertificates: 300, RegTimeOver: true},
		{ID: 5, CurrentTaxPercent: 15, NextTaxPercent: 15, LeftCertificates: 300, RegTimeOver: true},
	}, owners)
	return m, store, clans
}

func castleOf(t *testing.T, m *Manager, id int) *Castle {
	t.Helper()
	c, ok := m.Get(id)
	if !ok {
		t.Fatalf("castle %d not loaded", id)
	}
	return c
}

// TestRiseTaxRevenueSplitsTheTax pins the tax split of an amount collected
// at Gludio (system rate 40%, tribute 25% to Aden). The expected values
// follow the stored formula: amount -= (sysget/100.0)*amount as a long,
// then tribute = (int)((tribute/100.0)*amount), which goes to the parent
// untaxed (only when it has an owner) and is taken off either way.
func TestRiseTaxRevenueSplitsTheTax(t *testing.T) {
	for _, tt := range []struct {
		name                 string
		amount               int64
		parentOwned          bool
		wantGludio, wantAden int64
	}{
		{"round amount", 1000, true, 450, 150},
		{"truncated system cut", 999, true, 450, 149}, // 999-399.6 -> 599; 149.75 -> 149
		{"tribute below one", 2, true, 1, 0},          // 2-0.8 -> 1; 0.25 -> 0
		{"system cut to zero", 1, true, 0, 0},         // 1-0.4 -> 0
		{"unowned parent takes nothing", 1000, false, 450, 0},
		{"large amount", 1_000_000_000, true, 450_000_000, 150_000_000},
	} {
		t.Run(tt.name, func(t *testing.T) {
			parent := int32(0)
			if tt.parentOwned {
				parent = rivals
			}
			m, _, _ := newTestManager(t, lords, parent)
			gludio, aden := castleOf(t, m, 1), castleOf(t, m, 5)
			gludio.RiseTaxRevenue(tt.amount)
			if got := gludio.TaxRevenue(); got != tt.wantGludio {
				t.Fatalf("Gludio revenue = %d, want %d", got, tt.wantGludio)
			}
			if got := aden.TaxRevenue(); got != tt.wantAden {
				t.Fatalf("Aden revenue = %d, want %d", got, tt.wantAden)
			}
		})
	}
}

// TestRiseTaxRevenueStore pins the writes: the parent's tribute is stored
// before the castle's own share, a free castle collects and stores
// nothing, and revenue caps at the int32 maximum, after which it is no
// longer stored. Seed income follows the same split and cap.
func TestRiseTaxRevenueStore(t *testing.T) {
	m, store, _ := newTestManager(t, lords, rivals)
	gludio, aden := castleOf(t, m, 1), castleOf(t, m, 5)
	gludio.RiseTaxRevenue(1000)
	if got, want := store.take(), []string{"taxRevenue 5=150", "taxRevenue 1=450"}; !slices.Equal(got, want) {
		t.Fatalf("writes = %q, want %q", got, want)
	}
	gludio.RiseSeedIncome(1000)
	if got, want := store.take(), []string{"taxRevenue 5=300", "seedIncome 1=450"}; !slices.Equal(got, want) {
		t.Fatalf("seed income writes = %q, want %q", got, want)
	}

	aden.RiseTaxRevenue(math.MaxInt64 / 2)
	if got := aden.TaxRevenue(); got != math.MaxInt32 {
		t.Fatalf("capped revenue = %d, want %d", got, math.MaxInt32)
	}
	store.take()
	aden.RiseTaxRevenue(1000)
	if got := store.take(); len(got) != 0 {
		t.Fatalf("writes past the cap = %q, want none", got)
	}

	free, _, _ := newTestManager(t, 0, 0)
	castleOf(t, free, 1).RiseTaxRevenue(1000)
	if got := castleOf(t, free, 1).TaxRevenue(); got != 0 {
		t.Fatalf("free castle revenue = %d, want 0", got)
	}
}

// TestEditTreasury pins the treasury edits: a deposit caps at the int32
// maximum, a withdrawal past the treasury, a zero amount and a free castle
// are refused, and only save stores the result.
func TestEditTreasury(t *testing.T) {
	m, store, _ := newTestManager(t, lords, 0)
	gludio := castleOf(t, m, 1)
	steps := []struct {
		amount int64
		save   bool
		ok     bool
		want   int64
	}{
		{0, true, false, 0},
		{-1, true, false, 0},
		{500, true, true, 500},
		{-200, false, true, 300},
		{-301, true, false, 300},
		{math.MaxInt32, true, true, math.MaxInt32},
		{-300, true, true, math.MaxInt32 - 300},
	}
	for i, s := range steps {
		if ok := gludio.EditTreasury(s.amount, s.save); ok != s.ok || gludio.Treasury() != s.want {
			t.Fatalf("step %d: EditTreasury(%d) = %v, treasury %d; want %v, %d", i, s.amount, ok, gludio.Treasury(), s.ok, s.want)
		}
	}
	want := []string{"treasury 1=500", fmt.Sprintf("treasury 1=%d", math.MaxInt32), fmt.Sprintf("treasury 1=%d", math.MaxInt32-300)}
	if got := store.take(); !slices.Equal(got, want) {
		t.Fatalf("writes = %q, want %q", got, want)
	}
	free, _, _ := newTestManager(t, 0, 0)
	if castleOf(t, free, 1).EditTreasury(100, true) {
		t.Fatal("a free castle took a deposit")
	}
}

// TestUpdateTaxes pins the tax period's close: an owned castle moves its
// revenue and income into the treasury and puts its next rate in force; a
// free one clears its money and stores the default rates while its rate
// in force stays.
func TestUpdateTaxes(t *testing.T) {
	m, store, _ := newTestManager(t, lords, 0)
	gludio := castleOf(t, m, 1)
	gludio.restore(Row{ID: 1, CurrentTaxPercent: 10, NextTaxPercent: 20, Treasury: 100, TaxRevenue: 30, SeedIncome: 7})
	gludio.UpdateTaxes()
	if gludio.Treasury() != 137 || gludio.TaxRevenue() != 0 || gludio.SeedIncome() != 0 || gludio.CurrentTaxPercent() != 20 || gludio.TaxRate() != 0.2 {
		t.Fatalf("owned castle after update: treasury %d revenue %d income %d current %d rate %v",
			gludio.Treasury(), gludio.TaxRevenue(), gludio.SeedIncome(), gludio.CurrentTaxPercent(), gludio.TaxRate())
	}
	if got, want := store.take(), []string{"finances 1=137/0/0/20/20"}; !slices.Equal(got, want) {
		t.Fatalf("owned writes = %q, want %q", got, want)
	}

	aden := castleOf(t, m, 5)
	aden.restore(Row{ID: 5, CurrentTaxPercent: 10, NextTaxPercent: 20, Treasury: 100, TaxRevenue: 30, SeedIncome: 7})
	aden.UpdateTaxes()
	if aden.Treasury() != 0 || aden.TaxRevenue() != 0 || aden.SeedIncome() != 0 || aden.CurrentTaxPercent() != 10 || aden.NextTaxPercent() != 20 {
		t.Fatalf("free castle after update: treasury %d revenue %d income %d current %d next %d",
			aden.Treasury(), aden.TaxRevenue(), aden.SeedIncome(), aden.CurrentTaxPercent(), aden.NextTaxPercent())
	}
	if got, want := store.take(), []string{"finances 5=0/0/0/15/15"}; !slices.Equal(got, want) {
		t.Fatalf("free writes = %q, want %q", got, want)
	}
}

// TestTaxSettersStoreOnlyChanges pins the rate and certificate setters: a
// rate already set changes and stores nothing, save chooses whether a
// change is stored, and the rate in force sets the tax fraction.
func TestTaxSettersStoreOnlyChanges(t *testing.T) {
	m, store, _ := newTestManager(t, lords, 0)
	gludio := castleOf(t, m, 1)
	gludio.SetCurrentTaxPercent(15, true)
	gludio.SetNextTaxPercent(15, true)
	gludio.SetCurrentTaxPercent(25, true)
	gludio.SetNextTaxPercent(5, false)
	gludio.SetLeftCertificates(120, true)
	if gludio.TaxRate() != 0.25 || gludio.NextTaxPercent() != 5 || gludio.LeftCertificates() != 120 {
		t.Fatalf("rate %v next %d certificates %d", gludio.TaxRate(), gludio.NextTaxPercent(), gludio.LeftCertificates())
	}
	if got, want := store.take(), []string{"currentTax 1=25", "certificates 1=120"}; !slices.Equal(got, want) {
		t.Fatalf("writes = %q, want %q", got, want)
	}
}

// TestSetAndRemoveOwner pins a change of owner: the clan taking a castle
// gets its id and the clan losing it is returned with none; a clan already
// owning a castle is refused; removing the owner resets the castle's
// finances and stores them before the owner.
func TestSetAndRemoveOwner(t *testing.T) {
	m, store, clans := newTestManager(t, lords, kings)
	gludio := castleOf(t, m, 1)
	gludio.restore(Row{ID: 1, CurrentTaxPercent: 10, NextTaxPercent: 20, Treasury: 100, TaxRevenue: 30, SeedIncome: 7})
	get := func(id int32) *clan.Clan { cl, _ := clans.Get(id); return cl }

	if _, ok := m.SetOwner(gludio, get(kings)); ok {
		t.Fatal("a clan owning Aden took Gludio")
	}
	former, ok := m.SetOwner(gludio, get(rivals))
	if !ok || former != get(lords) || get(lords).CastleID() != 0 || get(rivals).CastleID() != 1 || gludio.OwnerID() != rivals {
		t.Fatalf("SetOwner: ok %v former %v, Lords castle %d, Rivals castle %d, owner %#x", ok, former, get(lords).CastleID(), get(rivals).CastleID(), gludio.OwnerID())
	}
	if c, ok := m.ByOwner(rivals); !ok || c != gludio {
		t.Fatal("ByOwner(Rivals) is not Gludio")
	}
	if got, want := store.take(), []string{fmt.Sprintf("owner 1=%d", rivals)}; !slices.Equal(got, want) {
		t.Fatalf("set writes = %q, want %q", got, want)
	}
	if gludio.Treasury() != 100 || gludio.CurrentTaxPercent() != 10 {
		t.Fatal("a transfer touched the finances")
	}

	cl, ok := m.RemoveOwner(gludio)
	if !ok || cl != get(rivals) || cl.CastleID() != 0 || !gludio.IsFree() {
		t.Fatalf("RemoveOwner: ok %v clan %v castle %d free %v", ok, cl, get(rivals).CastleID(), gludio.IsFree())
	}
	if gludio.Treasury() != 0 || gludio.TaxRevenue() != 0 || gludio.SeedIncome() != 0 || gludio.CurrentTaxPercent() != 15 || gludio.NextTaxPercent() != 15 || gludio.TaxRate() != 0.15 {
		t.Fatal("finances not reset to the defaults")
	}
	if got, want := store.take(), []string{"finances 1=0/0/0/15/15", "owner 1=0"}; !slices.Equal(got, want) {
		t.Fatalf("remove writes = %q, want %q", got, want)
	}
	if _, ok := m.RemoveOwner(gludio); ok {
		t.Fatal("a free castle lost an owner")
	}

	m.UnequipCirclets(gludio, 42)
	if got, want := store.take(), []string{"circlets 6838 of 42"}; !slices.Equal(got, want) {
		t.Fatalf("circlet writes = %q, want %q", got, want)
	}
}

// TestRestoreSkipsUnknownOwners pins the boot restore: an owner row naming
// a clan that is not loaded, or a castle that is not, is skipped; of two
// rows naming one castle the later wins.
func TestRestoreSkipsUnknownOwners(t *testing.T) {
	m := NewManager(testCastles(t), testClans(), nil, nil, zerolog.Nop())
	m.Restore([]Row{{ID: 1, CurrentTaxPercent: 12, NextTaxPercent: 13, Treasury: 9, SiegeDate: 1234, RegTimeOver: true, LeftCertificates: 77}, {ID: 9}},
		[]Owner{{ClanID: lords, CastleID: 1}, {ClanID: rivals, CastleID: 1}, {ClanID: 0x10000099, CastleID: 5}, {ClanID: kings, CastleID: 7}})
	gludio, aden := castleOf(t, m, 1), castleOf(t, m, 5)
	if gludio.OwnerID() != rivals || !aden.IsFree() {
		t.Fatalf("owners: Gludio %#x, Aden %#x", gludio.OwnerID(), aden.OwnerID())
	}
	if gludio.CurrentTaxPercent() != 12 || gludio.TaxRate() != 0.12 || gludio.NextTaxPercent() != 13 || gludio.Treasury() != 9 ||
		gludio.SiegeDate() != 1234 || !gludio.IsTimeRegistrationOver() || gludio.LeftCertificates() != 77 {
		t.Fatal("Gludio row not restored")
	}
	if c, ok := m.ByAlias("GLUDIO_castle"); !ok || c != gludio {
		t.Fatal("ByAlias is not case-insensitive")
	}
}
