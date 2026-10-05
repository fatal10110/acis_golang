package clanhall

import (
	"context"
	"time"

	hallmodel "github.com/fatal10110/acis_golang/internal/gameserver/model/residence/clanhall"
)

// Get returns hall hallID's function of type funcType, false when the hall
// rents none.
func (f *Functions) Get(hallID int32, funcType int) (Function, bool) {
	if f == nil {
		return Function{}, false
	}
	f.mu.Lock()
	defer f.mu.Unlock()
	fn, ok := f.byHall[hallID][funcType]
	if !ok {
		return Function{}, false
	}
	return fn.Function, true
}

// Update rents, changes or cancels hall hallID's function of type
// funcType, the renter having paid lease already. A function the hall does
// not rent yet is rented at level for lease, its first term rate
// milliseconds long from now; a level and lease of 0 rent nothing. One it
// rents is removed when level and lease are both 0, and otherwise takes
// the new level and lease and starts a new term now, as long as its own
// first one, its next fee due when it ends. Each change is stored.
func (f *Functions) Update(hallID int32, funcType, level, lease int, rate int64) {
	if f == nil {
		return
	}
	// A free hall's functions charge no fee; the owner is read before mu,
	// which is never held across an Owners call.
	owned := f.owners != nil && f.owners.HallOwner(hallID) != 0
	f.mu.Lock()
	defer f.mu.Unlock()
	nowMs := f.nowLocked()
	fn, ok := f.byHall[hallID][funcType]
	switch {
	case !ok:
		if level == 0 && lease == 0 {
			return
		}
		fn = &rented{Function: Function{Type: funcType, Level: level, Lease: lease, Rate: rate, EndTime: nowMs + rate}}
		f.hallLocked(hallID)[funcType] = fn
		f.saveLocked(hallID, fn)
		if owned {
			f.scheduleLocked(hallID, fn, fn.EndTime-nowMs)
		}
	case level == 0 && lease == 0:
		f.stopLocked(fn)
		delete(f.byHall[hallID], funcType)
		f.write("remove clan hall function", hallID, func(ctx context.Context, st Store) error {
			return st.DeleteFunction(ctx, hallID, funcType)
		})
	default:
		f.stopLocked(fn)
		fn.Lease, fn.Level = lease, level
		fn.EndTime = nowMs + fn.Rate
		f.saveLocked(hallID, fn)
		f.scheduleLocked(hallID, fn, fn.Rate)
	}
}

// nowLocked is the time on the clock the fees run on, in Unix
// milliseconds: the wall clock before Start.
func (f *Functions) nowLocked() int64 {
	if f.queue == nil {
		return time.Now().UnixMilli()
	}
	return f.queue.Now().UnixMilli()
}

// stopLocked stops fn's fee timer. A fee already under way leaves fn as it
// is: one not yet at the warehouse charges nothing, and one there neither
// re-arms, re-stores nor removes fn once the warehouse answers.
func (f *Functions) stopLocked(fn *rented) {
	if fn.timer != nil {
		fn.timer.Stop()
		fn.timer = nil
	}
	fn.gen++
}

// saveLocked stores fn as it is now.
func (f *Functions) saveLocked(hallID int32, fn *rented) {
	saved := fn.Function
	f.write("store clan hall function", hallID, func(ctx context.Context, st Store) error {
		return st.SaveFunction(ctx, hallID, saved)
	})
}

// Decos is the decoration data the functions are priced by, nil when f is.
func (f *Functions) Decos() *hallmodel.DecoTable {
	if f == nil {
		return nil
	}
	return f.decos
}
