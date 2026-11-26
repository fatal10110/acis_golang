package castle

import "math"

// The castle's purchases: its gate and trap upgrades, its certificate sales
// and its vault moves. Each is a check of the castle's state, a payment and
// the change it buys, made alone among the castle's purchases (purchase).
// pay takes adena from the buyer and reports false, taking nothing, when
// it holds less; it must not start another purchase.

// PurchaseOutcome is how a castle purchase ended.
type PurchaseOutcome int

const (
	// PurchaseDone means the price was paid and the change made.
	PurchaseDone PurchaseOutcome = iota
	// PurchaseUnknown means the gate or trap is none of the castle's;
	// nothing was paid.
	PurchaseUnknown
	// PurchaseAlreadyUpgraded means the gate or trap is already at the
	// level bought or above; nothing was paid.
	PurchaseAlreadyUpgraded
	// PurchaseNoneLeft means the castle has no certificates left; nothing
	// was paid.
	PurchaseNoneLeft
	// PurchaseShortOfAdena means the buyer could not pay.
	PurchaseShortOfAdena
)

// BuyGateUpgrade raises the castle's gate doorID to ratio for price,
// storing the upgrade. current is the gate's ratio before the purchase.
func (c *Castle) BuyGateUpgrade(doorID, ratio, price int, pay func(adena int) bool) (out PurchaseOutcome, current int) {
	c.purchase.Lock()
	defer c.purchase.Unlock()
	current, ok := c.GateUpgradeRatio(doorID)
	switch {
	case !ok:
		return PurchaseUnknown, 0
	case current >= ratio:
		return PurchaseAlreadyUpgraded, current
	case !pay(price):
		return PurchaseShortOfAdena, current
	}
	c.UpgradeDoor(doorID, ratio, true)
	return PurchaseDone, current
}

// BuyTrapUpgrade raises the trap of the castle's control tower at
// towerIndex to level for price, storing the upgrade. current is the
// trap's level before the purchase.
func (c *Castle) BuyTrapUpgrade(towerIndex, level, price int, pay func(adena int) bool) (out PurchaseOutcome, current int) {
	c.purchase.Lock()
	defer c.purchase.Unlock()
	current, ok := c.TrapUpgrade(towerIndex)
	switch {
	case !ok:
		return PurchaseUnknown, 0
	case current >= level:
		return PurchaseAlreadyUpgraded, current
	case !pay(price):
		return PurchaseShortOfAdena, current
	}
	c.SetTrapUpgrade(towerIndex, level, true)
	return PurchaseDone, current
}

// BuyCertificates sells n of the castle's certificates for price while it
// has any left, storing what is left, and returns the certificates left
// after the sale. The caller hands the bundle over on PurchaseDone.
func (c *Castle) BuyCertificates(n, price int, pay func(adena int) bool) (out PurchaseOutcome, left int) {
	c.purchase.Lock()
	defer c.purchase.Unlock()
	left = c.LeftCertificates()
	switch {
	case left == 0:
		return PurchaseNoneLeft, 0
	case !pay(price):
		return PurchaseShortOfAdena, left
	}
	return PurchaseDone, c.TakeCertificates(n)
}

// Deposit adds amount, paid through pay, to the castle's treasury and
// stores it, unless the treasury would reach the int32 maximum, which
// takes nothing.
func (c *Castle) Deposit(amount int, pay func(adena int) bool) {
	c.purchase.Lock()
	defer c.purchase.Unlock()
	if c.Treasury()+int64(amount) < math.MaxInt32 && pay(amount) {
		c.EditTreasury(int64(amount), true)
	}
}

// Withdraw takes amount out of the castle's treasury and stores it,
// reporting enough false when the treasury holds less and paid whether the
// amount was taken, for the caller to hand over.
func (c *Castle) Withdraw(amount int) (enough, paid bool) {
	c.purchase.Lock()
	defer c.purchase.Unlock()
	if c.Treasury() < int64(amount) {
		return false, false
	}
	return true, c.EditTreasury(-int64(amount), true)
}
