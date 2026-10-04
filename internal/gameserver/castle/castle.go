// Package castle holds the castles' live state: the clan owning each, its
// tax rates, treasury, tax revenue and seed income, its left certificates
// and its stored siege date, restored from the castle table at boot and
// written back as they change.
package castle

import (
	"context"
	"math"
	"sync"

	castledata "github.com/fatal10110/acis_golang/internal/gameserver/model/residence/castle"
)

// Castle is one castle's live state over its castles.xml definition.
//
// mu guards every field below it. A change is written through the
// manager's writer on the castle's own lane, so the castle row's writes
// land in the order they were made.
type Castle struct {
	*castledata.Castle
	parent *Castle
	m      *Manager

	mu                sync.Mutex
	ownerID           int32
	currentTaxPercent int
	nextTaxPercent    int
	taxRate           float64
	treasury          int64
	taxRevenue        int64
	seedIncome        int64
	leftCertificates  int
	siegeDate         int64
	regTimeOver       bool
}

// Row is a castle table row.
type Row struct {
	ID                int32
	CurrentTaxPercent int
	NextTaxPercent    int
	Treasury          int64
	TaxRevenue        int64
	SeedIncome        int64
	// SiegeDate is the stored siege date, in Unix milliseconds.
	SiegeDate        int64
	RegTimeOver      bool
	LeftCertificates int
}

// OwnerID is the id of the clan owning the castle, 0 for none.
func (c *Castle) OwnerID() int32 {
	c.mu.Lock()
	defer c.mu.Unlock()
	return c.ownerID
}

// IsFree reports whether no clan owns the castle.
func (c *Castle) IsFree() bool { return c.OwnerID() == 0 }

// CurrentTaxPercent is the tax rate in force, in percent.
func (c *Castle) CurrentTaxPercent() int {
	c.mu.Lock()
	defer c.mu.Unlock()
	return c.currentTaxPercent
}

// NextTaxPercent is the tax rate the next tax update puts in force, in
// percent.
func (c *Castle) NextTaxPercent() int {
	c.mu.Lock()
	defer c.mu.Unlock()
	return c.nextTaxPercent
}

// TaxRate is the tax rate in force as a fraction: CurrentTaxPercent / 100.
func (c *Castle) TaxRate() float64 {
	c.mu.Lock()
	defer c.mu.Unlock()
	return c.taxRate
}

// Treasury is the adena the castle holds.
func (c *Castle) Treasury() int64 {
	c.mu.Lock()
	defer c.mu.Unlock()
	return c.treasury
}

// TaxRevenue is the tax collected since the last tax update.
func (c *Castle) TaxRevenue() int64 {
	c.mu.Lock()
	defer c.mu.Unlock()
	return c.taxRevenue
}

// SeedIncome is the seed sale income collected since the last tax update.
func (c *Castle) SeedIncome() int64 {
	c.mu.Lock()
	defer c.mu.Unlock()
	return c.seedIncome
}

// LeftCertificates is how many certificates the castle has left to give.
func (c *Castle) LeftCertificates() int {
	c.mu.Lock()
	defer c.mu.Unlock()
	return c.leftCertificates
}

// SiegeDate is the stored siege date, in Unix milliseconds.
func (c *Castle) SiegeDate() int64 {
	c.mu.Lock()
	defer c.mu.Unlock()
	return c.siegeDate
}

// IsTimeRegistrationOver reports whether the stored siege date is no
// longer open to change.
func (c *Castle) IsTimeRegistrationOver() bool {
	c.mu.Lock()
	defer c.mu.Unlock()
	return c.regTimeOver
}

// restore sets the castle's state from its stored row, at boot.
func (c *Castle) restore(r Row) {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.siegeDate, c.regTimeOver = r.SiegeDate, r.RegTimeOver
	c.setCurrentTaxLocked(r.CurrentTaxPercent)
	c.nextTaxPercent = r.NextTaxPercent
	c.treasury, c.taxRevenue, c.seedIncome = r.Treasury, r.TaxRevenue, r.SeedIncome
	c.leftCertificates = r.LeftCertificates
}

// setCurrentTaxLocked puts value in force; c.mu is held.
func (c *Castle) setCurrentTaxLocked(value int) {
	c.currentTaxPercent = value
	c.taxRate = float64(value) / 100.0
}

// SetCurrentTaxPercent puts the tax rate value, in percent, in force, and
// stores it when save is set. Setting the rate already in force does
// nothing.
func (c *Castle) SetCurrentTaxPercent(value int, save bool) {
	c.mu.Lock()
	defer c.mu.Unlock()
	if c.currentTaxPercent == value {
		return
	}
	c.setCurrentTaxLocked(value)
	if save {
		c.write("update current tax", func(ctx context.Context, st Store) error {
			return st.UpdateCurrentTax(ctx, c.id(), value)
		})
	}
}

// SetNextTaxPercent sets the tax rate, in percent, the next tax update puts
// in force, and stores it when save is set. Setting the rate already set
// does nothing.
func (c *Castle) SetNextTaxPercent(value int, save bool) {
	c.mu.Lock()
	defer c.mu.Unlock()
	if c.nextTaxPercent == value {
		return
	}
	c.nextTaxPercent = value
	if save {
		c.write("update next tax", func(ctx context.Context, st Store) error {
			return st.UpdateNextTax(ctx, c.id(), value)
		})
	}
}

// SetLeftCertificates sets how many certificates the castle has left, and
// stores it when save is set.
func (c *Castle) SetLeftCertificates(n int, save bool) {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.leftCertificates = n
	if save {
		c.write("update certificates", func(ctx context.Context, st Store) error {
			return st.UpdateCertificates(ctx, c.id(), n)
		})
	}
}

// EditTreasury adds amount, negative to take some out, to the treasury of
// an owned castle, storing the new treasury when save is set. It reports
// false, changing nothing, for a free castle, a zero amount, or a
// withdrawal of more than the treasury holds. A deposit caps the treasury
// at the int32 maximum.
func (c *Castle) EditTreasury(amount int64, save bool) bool {
	c.mu.Lock()
	defer c.mu.Unlock()
	if !c.editTreasuryLocked(amount) {
		return false
	}
	if save {
		treasury := c.treasury
		c.write("update treasury", func(ctx context.Context, st Store) error {
			return st.UpdateTreasury(ctx, c.id(), treasury)
		})
	}
	return true
}

func (c *Castle) editTreasuryLocked(amount int64) bool {
	if c.ownerID <= 0 || amount == 0 {
		return false
	}
	switch {
	case amount < 0:
		if c.treasury < -amount {
			return false
		}
		c.treasury += amount
	case c.treasury+amount > math.MaxInt32:
		c.treasury = math.MaxInt32
	default:
		c.treasury += amount
	}
	return true
}

// RiseTaxRevenue adds the share of amount, the tax collected on a sale,
// that stays with an owned castle once taxed (see tax), and stores the new
// revenue. Revenue already at the int32 maximum stays there unstored;
// below it, the sum is capped at it.
func (c *Castle) RiseTaxRevenue(amount int64) { c.riseTaxRevenue(amount, false) }

func (c *Castle) riseTaxRevenue(amount int64, bypassTax bool) {
	if c.OwnerID() <= 0 || amount <= 0 {
		return
	}
	if !bypassTax {
		amount = c.tax(amount)
	}
	c.mu.Lock()
	defer c.mu.Unlock()
	if c.taxRevenue >= math.MaxInt32 {
		return
	}
	c.taxRevenue = min(c.taxRevenue+amount, math.MaxInt32)
	revenue := c.taxRevenue
	c.write("update tax revenue", func(ctx context.Context, st Store) error {
		return st.UpdateTaxRevenue(ctx, c.id(), revenue)
	})
}

// RiseSeedIncome adds the share of amount, the adena of a seed sale, that
// stays with an owned castle once taxed (see tax), and stores the new
// income, capped as RiseTaxRevenue caps the revenue.
func (c *Castle) RiseSeedIncome(amount int64) {
	if c.OwnerID() <= 0 || amount <= 0 {
		return
	}
	amount = c.tax(amount)
	c.mu.Lock()
	defer c.mu.Unlock()
	if c.seedIncome >= math.MaxInt32 {
		return
	}
	c.seedIncome = min(c.seedIncome+amount, math.MaxInt32)
	income := c.seedIncome
	c.write("update seed income", func(ctx context.Context, st Store) error {
		return st.UpdateSeedIncome(ctx, c.id(), income)
	})
}

// tax returns what is left of a positive amount once taxed: the system
// rate takes its percent first; then, when the castle has a parent castle
// and a tribute rate, the tribute's percent of the rest, truncated, goes
// to the parent's tax revenue untaxed (an unowned parent takes nothing)
// and leaves the amount all the same. A free castle or a non-positive
// amount keeps nothing.
func (c *Castle) tax(amount int64) int64 {
	if c.OwnerID() <= 0 || amount <= 0 {
		return 0
	}
	if sysget := c.Tax.SysgetRate; sysget > 0 {
		amount = int64(float64(amount) - float64(sysget)/100.0*float64(amount))
	}
	if tribute := c.Tax.TributeRate; c.ParentID > 0 && tribute > 0 && c.parent != nil {
		if share := javaInt(float64(tribute) / 100.0 * float64(amount)); share > 0 {
			c.parent.riseTaxRevenue(share, true)
			amount -= share
		}
	}
	return amount
}

// UpdateTaxes closes the castle's tax period. An owned castle moves its tax
// revenue and seed income into the treasury (capped as EditTreasury caps
// it), clears both and puts the next tax rate in force. A free castle
// clears its treasury, revenue and income; its stored rates both go back
// to the default rate while the rate in force is left as it is. Either
// way the castle row is stored in one write.
func (c *Castle) UpdateTaxes() {
	c.mu.Lock()
	defer c.mu.Unlock()
	f := Finances{CurrentTaxPercent: c.Tax.Rate, NextTaxPercent: c.Tax.Rate}
	if c.ownerID == 0 {
		c.treasury, c.taxRevenue, c.seedIncome = 0, 0, 0
	} else {
		c.editTreasuryLocked(c.taxRevenue + c.seedIncome)
		c.taxRevenue, c.seedIncome = 0, 0
		c.setCurrentTaxLocked(c.nextTaxPercent)
		f = Finances{Treasury: c.treasury, CurrentTaxPercent: c.currentTaxPercent, NextTaxPercent: c.nextTaxPercent}
	}
	c.write("update taxes", func(ctx context.Context, st Store) error {
		return st.UpdateFinances(ctx, c.id(), f)
	})
}

// resetFinancesLocked clears a castle losing its owner: treasury, revenue
// and income go to 0 and both rates back to the default, and the row is
// stored; c.mu is held.
func (c *Castle) resetFinancesLocked() {
	c.treasury, c.taxRevenue, c.seedIncome = 0, 0, 0
	c.setCurrentTaxLocked(c.Tax.Rate)
	c.nextTaxPercent = c.Tax.Rate
	f := Finances{CurrentTaxPercent: c.Tax.Rate, NextTaxPercent: c.Tax.Rate}
	c.write("reset finances", func(ctx context.Context, st Store) error {
		return st.UpdateFinances(ctx, c.id(), f)
	})
}

// javaInt narrows v to an int32 as a Java (int) cast does: toward zero,
// saturating at the int32 range.
func javaInt(v float64) int64 {
	switch {
	case math.IsNaN(v):
		return 0
	case v >= math.MaxInt32:
		return math.MaxInt32
	case v <= math.MinInt32:
		return math.MinInt32
	}
	return int64(int32(v))
}

func (c *Castle) id() int32 { return int32(c.ID) }

// write queues one castle row write on the castle's lane.
func (c *Castle) write(what string, fn func(context.Context, Store) error) {
	c.m.write(c.id(), what, fn)
}
