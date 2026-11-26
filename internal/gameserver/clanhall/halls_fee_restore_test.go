package clanhall

import (
	"context"
	"maps"
	"testing"
	"time"

	"github.com/rs/zerolog"

	"github.com/fatal10110/acis_golang/internal/commons"
	"github.com/fatal10110/acis_golang/internal/gameserver/sim"
)

// restoreProbeBank is crashBank, recording for each warehouse restore
// whether the halls' lock was free while it ran and whether it came before
// the fee was taken.
type restoreProbeBank struct {
	*crashBank
	hs       *Halls
	restored []int32
	lockFree []bool
	taken    bool
	takenOK  bool
	// failing is how many restores, from the first, fail.
	failing int
}

func (b *restoreProbeBank) RestoreWarehouse(clanID int32) bool {
	free := b.hs.mu.TryLock()
	if free {
		b.hs.mu.Unlock()
	}
	b.restored = append(b.restored, clanID)
	b.lockFree = append(b.lockFree, free)
	return len(b.restored) > b.failing
}

func (b *restoreProbeBank) TakeAdena(clanID int32, adena int) (Landing, bool) {
	b.taken, b.takenOK = true, len(b.restored) > b.failing
	return b.crashBank.TakeAdena(clanID, adena)
}

// The weekly lease reads the owner's warehouse before it takes the halls'
// lock, so a slow read holds up no other call on the halls; the fee is
// then taken as before.
func TestLeaseRestoresWarehouseOutsideTheLock(t *testing.T) {
	start := time.Date(2026, 10, 5, 12, 0, 0, 0, time.UTC)
	base := crashBase(map[int32]int{crashSellers: 1200})
	base.halls[crashHall] = HallRow{ID: crashHall, OwnerID: crashSellers, PaidUntil: start.Add(time.Minute).UnixMilli(), Paid: true}

	log := &crashLog{}
	hs := NewHalls(crashHallData(t), crashClans(true), nil, &crashStore{base: base, log: log}, &fifoLanes{}, zerolog.Nop())
	if err := hs.Restore(context.Background()); err != nil {
		t.Fatal(err)
	}
	bank := &restoreProbeBank{crashBank: &crashBank{log: log, held: maps.Clone(base.adena)}, hs: hs}
	loop := sim.NewInline(start)
	hs.Start(loop.NewQueue("halls"), bank, nil, nil)
	loop.Advance(2 * time.Minute)

	if len(bank.restored) != 1 || bank.restored[0] != crashSellers || !bank.lockFree[0] {
		t.Fatalf("restores %v with the lock free %v, want one of clan %d outside the lock", bank.restored, bank.lockFree, crashSellers)
	}
	if !bank.taken || !bank.takenOK {
		t.Fatalf("fee taken %v after the restore %v, want taken after it", bank.taken, bank.takenOK)
	}
	hs.mu.Lock()
	h := hs.byID[crashHall]
	paidUntil, paid := h.paidUntil, h.paid
	hs.mu.Unlock()
	if want := base.halls[crashHall].PaidUntil + commons.WeekMs; paidUntil != want || !paid {
		t.Fatalf("term paid until %d (paid %v), want %d and paid", paidUntil, paid, want)
	}
}

// A lease whose read of the owner's warehouse fails takes nothing, so it
// reads nothing under the halls' lock, and keeps the owner's hall and term:
// it tries again a minute later and is paid then.
func TestLeaseRetriesWhenWarehouseUnread(t *testing.T) {
	start := time.Date(2026, 10, 5, 12, 0, 0, 0, time.UTC)
	base := crashBase(map[int32]int{crashSellers: 1200})
	base.halls[crashHall] = HallRow{ID: crashHall, OwnerID: crashSellers, PaidUntil: start.Add(time.Minute).UnixMilli(), Paid: true}

	log := &crashLog{}
	hs := NewHalls(crashHallData(t), crashClans(true), nil, &crashStore{base: base, log: log}, &fifoLanes{}, zerolog.Nop())
	if err := hs.Restore(context.Background()); err != nil {
		t.Fatal(err)
	}
	bank := &restoreProbeBank{crashBank: &crashBank{log: log, held: maps.Clone(base.adena)}, hs: hs, failing: 1}
	loop := sim.NewInline(start)
	hs.Start(loop.NewQueue("halls"), bank, nil, nil)
	loop.Advance(90 * time.Second)

	term := func() (int32, int64, bool) {
		hs.mu.Lock()
		defer hs.mu.Unlock()
		h := hs.byID[crashHall]
		return h.ownerID, h.paidUntil, h.paid
	}
	if len(bank.restored) != 1 || bank.taken {
		t.Fatalf("restores %v, fee taken %v, want one failed restore and no fee", bank.restored, bank.taken)
	}
	if owner, paidUntil, paid := term(); owner != crashSellers || paidUntil != base.halls[crashHall].PaidUntil || !paid {
		t.Fatalf("after a failed restore: owner %d, paid until %d (paid %v), want %d's term unchanged", owner, paidUntil, paid, crashSellers)
	}

	loop.Advance(feeRetryMs * time.Millisecond)
	if len(bank.restored) != 2 || !bank.lockFree[1] || !bank.taken || !bank.takenOK {
		t.Fatalf("restores %v with the lock free %v, fee taken %v after one %v, want the retry to restore then take it", bank.restored, bank.lockFree, bank.taken, bank.takenOK)
	}
	if owner, paidUntil, paid := term(); owner != crashSellers || paidUntil != base.halls[crashHall].PaidUntil+commons.WeekMs || !paid {
		t.Fatalf("after the retry: owner %d, paid until %d (paid %v), want the term paid a week on", owner, paidUntil, paid)
	}
}
