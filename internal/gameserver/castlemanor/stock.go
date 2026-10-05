package castlemanor

import "sync/atomic"

// Production is one seed a castle sells over a manor period: StartAmount
// seeds at Price each, of which Amount are left. It is shared by pointer:
// the period lists hand the same Production around, and a sale takes its
// seeds with DecreaseAmount from any goroutine.
type Production struct {
	// ID is the seed's item id.
	ID          int32
	Price       int32
	StartAmount int32

	amount atomic.Int32
}

// NewProduction returns a seed production of seedID with amount seeds
// left.
func NewProduction(seedID, amount, price, startAmount int32) *Production {
	p := &Production{ID: seedID, Price: price, StartAmount: startAmount}
	p.amount.Store(amount)
	return p
}

// Amount is how many seeds, or crops, are left.
func (p *Production) Amount() int32 { return p.amount.Load() }

// SetAmount sets how many are left.
func (p *Production) SetAmount(n int32) { p.amount.Store(n) }

// DecreaseAmount takes n of what is left, reporting false, taking none,
// when fewer than n are left.
func (p *Production) DecreaseAmount(n int32) bool {
	for {
		cur := p.amount.Load()
		next := cur - n
		if next < 0 {
			return false
		}
		if p.amount.CompareAndSwap(cur, next) {
			return true
		}
	}
}

// Procure is one crop a castle buys over a manor period: StartAmount crops
// at Price each, of which Amount are still wanted, paid for with reward
// item Reward (1 or 2) of the crop's seed. Its ID is the crop's item id.
type Procure struct {
	Production
	Reward int32
}

// NewProcure returns a crop procure of cropID with amount crops still
// wanted.
func NewProcure(cropID, amount, price, startAmount, reward int32) *Procure {
	c := &Procure{Production: Production{ID: cropID, Price: price, StartAmount: startAmount}, Reward: reward}
	c.amount.Store(amount)
	return c
}
