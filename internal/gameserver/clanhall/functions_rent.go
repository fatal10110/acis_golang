package clanhall

import "github.com/fatal10110/acis_golang/internal/commons"

// RentOutcome is how a function rental ended.
type RentOutcome int

const (
	// Rented means the function was rented or changed.
	Rented RentOutcome = iota
	// RentCancelled means the function was cancelled.
	RentCancelled
	// RentUnchanged means the function is already at the level asked for;
	// nothing changed.
	RentUnchanged
	// RentUnpriced means the deco table prices no such level: it would
	// rent for free, for a term of no length, and is refused.
	RentUnpriced
	// RentShortOfAdena means the renter could not pay the first fee.
	RentShortOfAdena
)

// Rent rents, changes or, at level 0, cancels hall hallID's function of
// type funcType at level, for the fee and term clanHallDeco.xml gives
// decoLevel. A function already at level is left as is. pay takes the
// first fee from the renter and returns the write that lands its adena
// row, which Update runs ahead of the function's, reporting false when the
// renter holds less.
func (f *Functions) Rent(hallID int32, funcType, decoLevel, level int, pay func(fee int) (Landing, bool)) RentOutcome {
	if fn, ok := f.Get(hallID, funcType); ok && fn.Level == level {
		return RentUnchanged
	}
	decos := f.Decos()
	term := int64(decos.Days(funcType, decoLevel)) * commons.DayMs
	if level == 0 {
		f.Update(hallID, funcType, 0, 0, term, nil)
		return RentCancelled
	}
	if _, priced := decos.Get(funcType, decoLevel); !priced {
		return RentUnpriced
	}
	fee := decos.Fee(funcType, decoLevel)
	var paid Landing
	if fee > 0 {
		landing, ok := pay(fee)
		if !ok {
			return RentShortOfAdena
		}
		paid = landing
	}
	f.Update(hallID, funcType, level, fee, term, paid)
	return Rented
}
