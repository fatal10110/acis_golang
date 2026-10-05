package castle

import "context"

// resetCertificates is how many certificates every castle has again once
// the Seven Signs recruitment ends.
const resetCertificates = 300

// UpdateTaxes closes the tax period of every castle (see Castle.UpdateTaxes):
// the body of the daily castle tax refresh.
func (m *Manager) UpdateTaxes() {
	for _, c := range m.All() {
		c.UpdateTaxes()
	}
}

// ValidateTaxes brings every castle's tax rate in force above maxPercent
// down to it, storing it. The rate the next tax update puts in force is
// left as it is.
func (m *Manager) ValidateTaxes(maxPercent int) {
	for _, c := range m.All() {
		c.capCurrentTax(maxPercent)
	}
}

// ResetCertificates gives every castle its full count of certificates
// back, storing each.
//
// The reference resets memory and stores every row with one table-wide
// update; here each castle's row is written on its own lane, so the reset
// lands after any certificate write that castle queued before it.
func (m *Manager) ResetCertificates() {
	for _, c := range m.All() {
		c.SetLeftCertificates(resetCertificates, true)
	}
}

// capCurrentTax brings the tax rate in force down to maxPercent, storing
// it, when it is higher.
func (c *Castle) capCurrentTax(maxPercent int) {
	c.mu.Lock()
	defer c.mu.Unlock()
	if c.currentTaxPercent <= maxPercent {
		return
	}
	c.setCurrentTaxLocked(maxPercent)
	c.write("update current tax", func(ctx context.Context, st Store) error {
		return st.UpdateCurrentTax(ctx, c.id(), maxPercent)
	})
}
