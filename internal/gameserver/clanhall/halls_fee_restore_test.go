package clanhall

import (
	"context"
	"maps"
	"testing"
	"time"

	"github.com/rs/zerolog"

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
}

func (b *restoreProbeBank) RestoreWarehouse(clanID int32) {
	free := b.hs.mu.TryLock()
	if free {
		b.hs.mu.Unlock()
	}
	b.restored = append(b.restored, clanID)
	b.lockFree = append(b.lockFree, free)
}

func (b *restoreProbeBank) TakeAdena(clanID int32, adena int) (Landing, bool) {
	b.taken, b.takenOK = true, len(b.restored) > 0
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
	if want := base.halls[crashHall].PaidUntil + weekMs; paidUntil != want || !paid {
		t.Fatalf("term paid until %d (paid %v), want %d and paid", paidUntil, paid, want)
	}
}
