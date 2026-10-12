package clanhall

import (
	"context"
	"maps"
	"testing"
	"time"

	"github.com/rs/zerolog"

	"github.com/fatal10110/acis_golang/internal/gameserver/sim"
)

// The test below crashes the server after every write the weekly lease
// payment makes, restarts it from what landed, lets the lease fall due
// again if the term it paid for was lost, and checks the owner never keeps
// both the lease's adena and the week it paid for (#3454).

// settleLease restarts the halls from db at the clock at, lets an overdue
// lease be charged, and returns the database once every write landed.
func settleLease(t *testing.T, data crashDB, at time.Time) crashDB {
	t.Helper()
	log := &crashLog{}
	hs := NewHalls(crashHallData(t), crashClans(true), nil, &crashStore{base: data, log: log}, nil, zerolog.Nop())
	if err := hs.Restore(context.Background()); err != nil {
		t.Fatal(err)
	}
	loop := sim.NewInline(at)
	hs.Start(loop.NewQueue("halls"), &crashBank{log: log, held: maps.Clone(data.adena)}, nil, nil)
	loop.Advance(time.Second)
	return log.replay(data, len(log.writes))
}

func TestLeasePaymentSurvivesACrashAnywhere(t *testing.T) {
	start := time.Date(2026, 10, 5, 12, 0, 0, 0, time.UTC)
	base := crashBase(map[int32]int{crashSellers: 1200})
	base.halls[crashHall] = HallRow{ID: crashHall, OwnerID: crashSellers, PaidUntil: start.Add(time.Minute).UnixMilli(), Paid: true}

	log := &crashLog{}
	lanes := &fifoLanes{}
	hs := NewHalls(crashHallData(t), crashClans(true), nil, &crashStore{base: base, log: log}, lanes, zerolog.Nop())
	if err := hs.Restore(context.Background()); err != nil {
		t.Fatal(err)
	}
	loop := sim.NewInline(start)
	hs.Start(loop.NewQueue("halls"), &crashBank{log: log, held: maps.Clone(base.adena)}, nil, nil)
	loop.Advance(2 * time.Minute)
	lanes.drain()

	n := len(log.writes)
	end := log.replay(base, n)
	if got, want := end.halls[crashHall].PaidUntil, base.halls[crashHall].PaidUntil+weekMs; got != want || end.adena[crashSellers] != 700 {
		t.Fatalf("after the payment: paid until %d with %d adena, want %d with 700", got, end.adena[crashSellers], want)
	}
	restartAt := loop.Now().Add(time.Minute)
	limit := settleLease(t, end, restartAt).adena[crashSellers]
	for i := range n {
		db := log.replay(base, i)
		got := settleLease(t, db, restartAt)
		if row := got.halls[crashHall]; row.OwnerID != crashSellers || row.PaidUntil < restartAt.UnixMilli() {
			t.Fatalf("crash after write %d of %d: hall row %+v after the restart, want it owned and paid past the restart", i, n, row)
		}
		if a := got.adena[crashSellers]; a > limit {
			t.Errorf("crash after write %d of %d: owner keeps %d adena with the week paid, more than %d without the crash", i, n, a, limit)
		}
	}
}
