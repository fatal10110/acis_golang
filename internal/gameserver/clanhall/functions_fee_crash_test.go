package clanhall

import (
	"context"
	"maps"
	"sync"
	"testing"
	"time"

	"github.com/rs/zerolog"

	hallmodel "github.com/fatal10110/acis_golang/internal/gameserver/model/residence/clanhall"
	"github.com/fatal10110/acis_golang/internal/gameserver/sim"
)

// The test below crashes the server after every write a function's fee
// makes, restarts it from what landed, lets the fee fall due again if the
// term it paid for was lost, and checks the owning clan never keeps both
// the fee's adena and the term it paid for (#3454).

// feeDB is the database as a crash leaves it: feeHall's clanhall_functions
// rows by type and feeClan's warehouse adena.
type feeDB struct {
	rows  map[int]Function
	adena int
}

// feeLog is every write in the order it reached the database.
type feeLog struct {
	mu     sync.Mutex
	writes []func(*feeDB)
}

func (l *feeLog) add(w func(*feeDB)) {
	l.mu.Lock()
	defer l.mu.Unlock()
	l.writes = append(l.writes, w)
}

func (l *feeLog) replay(base feeDB, n int) feeDB {
	l.mu.Lock()
	defer l.mu.Unlock()
	db := feeDB{rows: maps.Clone(base.rows), adena: base.adena}
	for _, w := range l.writes[:n] {
		w(&db)
	}
	return db
}

// feeCrashStore loads base and logs every write.
type feeCrashStore struct {
	base feeDB
	log  *feeLog
}

func (s *feeCrashStore) LoadFunctions(context.Context) ([]StoredFunction, error) {
	var out []StoredFunction
	for _, f := range s.base.rows {
		out = append(out, StoredFunction{HallID: feeHall, Function: f})
	}
	return out, nil
}

func (s *feeCrashStore) SaveFunction(_ context.Context, _ int32, f Function) error {
	s.log.add(func(d *feeDB) { d.rows[f.Type] = f })
	return nil
}

func (s *feeCrashStore) DeleteFunction(_ context.Context, _ int32, funcType int) error {
	s.log.add(func(d *feeDB) { delete(d.rows, funcType) })
	return nil
}

func (s *feeCrashStore) DeleteFunctions(context.Context, int32) error {
	s.log.add(func(d *feeDB) { clear(d.rows) })
	return nil
}

// feeCrashTreasury holds feeClan's warehouse adena and logs a fee taken
// only when its landing runs, the latest the item persistence could write
// it.
type feeCrashTreasury struct {
	log  *feeLog
	mu   sync.Mutex
	held int
}

func (b *feeCrashTreasury) TakeAdena(clanID int32, adena int) (Landing, bool) {
	b.mu.Lock()
	defer b.mu.Unlock()
	if clanID != feeClan || b.held < adena {
		return nil, false
	}
	b.held -= adena
	return func(context.Context) error {
		b.log.add(func(d *feeDB) { d.adena -= adena })
		return nil
	}, true
}

// crashFunctions returns feeHall's functions restored from store, written
// through writes.
func crashFunctions(t *testing.T, store Store, writes Writer) *Functions {
	t.Helper()
	halls, err := hallmodel.NewTable([]*hallmodel.Hall{{ID: int(feeHall), Alias: "moonstone"}})
	if err != nil {
		t.Fatal(err)
	}
	decos, err := hallmodel.NewDecoTable(nil)
	if err != nil {
		t.Fatal(err)
	}
	f := New(halls, decos, feeOwners{}, store, writes, zerolog.Nop())
	if err := f.Restore(context.Background()); err != nil {
		t.Fatal(err)
	}
	return f
}

// settleFee restarts the functions from db at the clock at, lets an
// overdue fee be charged, and returns the database once every write
// landed.
func settleFee(t *testing.T, db feeDB, at time.Time) feeDB {
	t.Helper()
	log := &feeLog{}
	f := crashFunctions(t, &feeCrashStore{base: db, log: log}, nil)
	loop := sim.NewInline(at)
	f.Start(loop.NewQueue("clanhall-functions"), &feeCrashTreasury{log: log, held: db.adena})
	loop.Advance(time.Second)
	return log.replay(db, len(log.writes))
}

func TestFunctionFeeSurvivesACrashAnywhere(t *testing.T) {
	start := time.Date(2026, 10, 5, 12, 0, 0, 0, time.UTC)
	const lease, rate = 1000, int64(7 * 24 * time.Hour / time.Millisecond)
	base := feeDB{adena: 2500, rows: map[int]Function{hallmodel.FuncRestoreHP: {
		Type: hallmodel.FuncRestoreHP, Level: 80, Lease: lease, Rate: rate, EndTime: start.Add(time.Minute).UnixMilli(),
	}}}

	log := &feeLog{}
	lanes := &fifoLanes{}
	f := crashFunctions(t, &feeCrashStore{base: base, log: log}, lanes)
	loop := sim.NewInline(start)
	f.Start(loop.NewQueue("clanhall-functions"), &feeCrashTreasury{log: log, held: base.adena})
	loop.Advance(2 * time.Minute)
	lanes.drain()

	n := len(log.writes)
	end := log.replay(base, n)
	if row, ok := end.rows[hallmodel.FuncRestoreHP]; !ok || row.EndTime <= start.UnixMilli() || end.adena != 2500-lease {
		t.Fatalf("after the fee: row %+v (stored %v) with %d adena, want a new term and %d", row, ok, end.adena, 2500-lease)
	}
	restartAt := loop.Now().Add(time.Minute)
	limit := settleFee(t, end, restartAt).adena
	for i := range n {
		got := settleFee(t, log.replay(base, i), restartAt)
		if row, ok := got.rows[hallmodel.FuncRestoreHP]; !ok || row.EndTime < restartAt.UnixMilli() {
			t.Fatalf("crash after write %d of %d: row %+v (stored %v) after the restart, want a term past the restart", i, n, row, ok)
		}
		if got.adena > limit {
			t.Errorf("crash after write %d of %d: clan keeps %d adena with the term paid, more than %d without the crash", i, n, got.adena, limit)
		}
	}
}
