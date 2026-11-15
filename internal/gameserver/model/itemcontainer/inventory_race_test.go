package itemcontainer

import (
	"runtime"
	"sync"
	"testing"

	"github.com/fatal10110/acis_golang/internal/gameserver/model/item"
)

func raceTemplates() *item.Table {
	return item.NewTable([]*item.Template{
		{ID: 1, Kind: item.KindEtcItem, Stackable: true, EtcItem: &item.EtcItemDetail{}},
	})
}

// TestInventory_DestroyItem_AfterWholeStackLeftIsNoop destroys from an
// instance its inventory no longer holds: a whole-stack exchange moved the
// very same instance to the receiver. The stale destroy must not take units
// from the receiver's stack.
func TestInventory_DestroyItem_AfterWholeStackLeftIsNoop(t *testing.T) {
	templates := raceTemplates()
	a := NewPlayerInventory(0x10000001, templates)
	b := NewPlayerInventory(0x10000002, templates)
	inst := a.AddNew(1, 10, 0x20000001)

	Exchange(a, b, func(ha, hb Held) {
		if _, ok := ha.Transfer(inst.ObjectID, 10, hb, 0); !ok {
			t.Fatal("whole-stack Transfer failed")
		}
	})
	if b.ItemByObjectID(inst.ObjectID) != inst {
		t.Fatal("the receiver should now hold the moved instance")
	}

	if got := a.DestroyItem(inst, 3); got != nil {
		t.Errorf("DestroyItem on a moved instance = %+v, want nil", got)
	}
	if got := a.DestroyItem(inst, 10); got != nil {
		t.Errorf("DestroyItem of every unit of a moved instance = %+v, want nil", got)
	}
	if got := inst.CountValue(); got != 10 {
		t.Errorf("receiver's stack Count = %d, want 10", got)
	}
	if b.ItemByObjectID(inst.ObjectID) != inst {
		t.Error("the receiver lost the moved instance")
	}
}

// TestExchange_ConsumeAndDropCannotInterleave races the holder consuming and
// dropping units of a stack, one at a time by object id, against an exchange
// that checks the stack's count and then moves the whole stack into the
// receiver's own stack. While the exchange holds both inventories nothing
// may change the count it checked, every unit must end up exactly once in
// one of the stacks, the consumed tally or the dropped tally, and no stack
// may be left listed at count zero.
func TestExchange_ConsumeAndDropCannotInterleave(t *testing.T) {
	const (
		iterations = 2000
		srcCount   = 1 << 20
		dstCount   = 5
		srcID      = int32(0x20000001)
		dstID      = int32(0x20000002)
	)
	templates := raceTemplates()

	var changedUnderLock, lost, zeroListed int
	for i := range iterations {
		a := NewPlayerInventory(0x10000001, templates)
		b := NewPlayerInventory(0x10000002, templates)
		a.AddNew(1, srcCount, srcID)
		b.AddNew(1, dstCount, dstID)

		var consumed, dropped int
		running, stop := make(chan struct{}), make(chan struct{})
		var wg sync.WaitGroup
		wg.Go(func() {
			// The holder keeps the stack it consumes from, as worn arrows
			// or shots are, and drops by object id.
			held := a.ItemByObjectID(srcID)
			for n := int32(0); ; n++ {
				if n == 1 {
					close(running)
				}
				select {
				case <-stop:
					return
				default:
				}
				if n%4 != 3 {
					if a.DestroyItem(held, 1) != nil {
						consumed++
					}
					continue
				}
				if d := a.DropItem(srcID, 1, 0x30000000+n); d != nil {
					dropped += d.CountValue()
				}
			}
		})

		// The exchange starts once the holder is consuming, so the two
		// overlap instead of the exchange finishing before the holder runs.
		<-running
		Exchange(a, b, func(ha, hb Held) {
			src := ha.ItemByObjectID(srcID)
			if src == nil {
				return
			}
			// Yield repeatedly so a holder able to slip in gets the chance.
			checked := src.Snapshot().Count
			for range 200 {
				runtime.Gosched()
				if src.Snapshot().Count != checked {
					changedUnderLock++
					break
				}
			}
			ha.Transfer(srcID, checked, hb, 0)
		})
		close(stop)
		wg.Wait()

		total := consumed + dropped
		for _, inv := range []*Inventory{a, b} {
			for _, inst := range inv.Items() {
				c := inst.CountValue()
				if c == 0 {
					zeroListed++
				}
				total += c
			}
		}
		if total != srcCount+dstCount {
			lost++
			if lost == 1 {
				t.Errorf("iteration %d: units accounted = %d, want %d", i, total, srcCount+dstCount)
			}
		}
	}
	if changedUnderLock > 0 {
		t.Errorf("stack count changed inside the exchange in %d of %d iterations", changedUnderLock, iterations)
	}
	if lost > 0 {
		t.Errorf("units not conserved in %d of %d iterations", lost, iterations)
	}
	if zeroListed > 0 {
		t.Errorf("%d zero-count stacks left listed", zeroListed)
	}
}
