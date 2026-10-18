package clanhall

import (
	"context"
	"errors"
	"testing"
	"time"

	hallmodel "github.com/fatal10110/acis_golang/internal/gameserver/model/residence/clanhall"
	"github.com/fatal10110/acis_golang/internal/gameserver/sim"
)

// The tests below crash the server after every write a function rent or
// change makes, restart it from what landed, and check the database never
// holds both the term the renter paid for and the renter's adena (#3712).
// feeDB.adena stands for the renter's inventory adena here.

// renterPayment is the renter's adena row as a rent or change left it: it
// is logged only when the landing runs, the latest the item persistence
// could write it. A held write never reaches the log.
func renterPayment(log *feeLog, fee int) Landing {
	return func(context.Context) error {
		log.add(func(d *feeDB) { d.adena -= fee })
		return nil
	}
}

func TestFunctionRentAndChangeSurviveACrashAnywhere(t *testing.T) {
	start := time.Date(2026, 10, 12, 12, 0, 0, 0, time.UTC)
	const held, day = 100_000, int64(24 * time.Hour / time.Millisecond)
	rentedHP := Function{Type: hallmodel.FuncRestoreHP, Level: 80, Lease: 1000, Rate: day, EndTime: start.Add(time.Hour).UnixMilli()}
	for _, tc := range []struct {
		name  string
		base  map[int]Function
		level int
		fee   int
	}{
		{"rent", map[int]Function{}, 80, 1000},
		{"change", map[int]Function{hallmodel.FuncRestoreHP: rentedHP}, 140, 1750},
	} {
		t.Run(tc.name, func(t *testing.T) {
			base := feeDB{adena: held, rows: tc.base}
			log := &feeLog{}
			lanes := &fifoLanes{}
			f := crashFunctions(t, &feeCrashStore{base: base, log: log}, lanes)
			f.Start(sim.NewInline(start).NewQueue("clanhall-functions"), nil)
			f.Update(feeHall, hallmodel.FuncRestoreHP, tc.level, tc.fee, day, renterPayment(log, tc.fee))
			lanes.drain()

			n := len(log.writes)
			end := log.replay(base, n)
			if row := end.rows[hallmodel.FuncRestoreHP]; row.Level != tc.level || end.adena != held-tc.fee {
				t.Fatalf("after the %s: row %+v with %d adena, want level %d and %d", tc.name, row, end.adena, tc.level, held-tc.fee)
			}
			for i := range n + 1 {
				db := log.replay(base, i)
				// The restart reads only what landed.
				restarted := crashFunctions(t, &feeCrashStore{base: db, log: &feeLog{}}, nil)
				fn, rented := restarted.Get(feeHall, hallmodel.FuncRestoreHP)
				if rented && fn.Level == tc.level && db.adena == held {
					t.Errorf("crash after write %d of %d: the restart rents level %d and the renter keeps all %d adena", i, n, fn.Level, db.adena)
				}
			}
		})
	}
}

func TestFunctionRentWhoseAdenaFailsToLandStoresNoTerm(t *testing.T) {
	start := time.Date(2026, 10, 12, 12, 0, 0, 0, time.UTC)
	const day = int64(24 * time.Hour / time.Millisecond)
	base := feeDB{adena: 100_000, rows: map[int]Function{}}
	log := &feeLog{}
	lanes := &fifoLanes{}
	f := crashFunctions(t, &feeCrashStore{base: base, log: log}, lanes)
	f.Start(sim.NewInline(start).NewQueue("clanhall-functions"), nil)

	f.Update(feeHall, hallmodel.FuncRestoreHP, 80, 1000, day, func(context.Context) error {
		return errors.New("inventory write failed")
	})
	if len(lanes.jobs) == 0 {
		t.Fatal("the rent queued no write")
	}
	lanes.drain()
	if n := len(log.writes); n != 0 {
		t.Fatalf("the renter's adena failed to land, yet %d writes reached the database", n)
	}
}
