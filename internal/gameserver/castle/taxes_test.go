package castle

import (
	"slices"
	"testing"
)

// TestManagerUpdateTaxes pins the all-castles tax refresh
// (CastleManager.updateTaxes): an owned castle banks its revenue and puts
// its next rate in force, a free one is cleared back to the default rates,
// each in one row write, in castle order.
func TestManagerUpdateTaxes(t *testing.T) {
	m, store, _ := newTestManager(t, lords, 0)
	gludio, aden := castleOf(t, m, 1), castleOf(t, m, 5)
	gludio.RiseTaxRevenue(1000) // 1000 - 40% = 600; Aden is free, so no tribute: 600 - 150 = 450.
	gludio.SetNextTaxPercent(20, false)
	aden.SetCurrentTaxPercent(10, false)
	store.take()

	m.UpdateTaxes()

	if got, want := store.take(), []string{"finances 1=450/0/0/20/20", "finances 5=0/0/0/15/15"}; !slices.Equal(got, want) {
		t.Fatalf("writes = %q, want %q", got, want)
	}
	if gludio.Treasury() != 450 || gludio.TaxRevenue() != 0 || gludio.CurrentTaxPercent() != 20 {
		t.Fatalf("Gludio treasury/revenue/tax = %d/%d/%d, want 450/0/20", gludio.Treasury(), gludio.TaxRevenue(), gludio.CurrentTaxPercent())
	}
	// A free castle's rate in force stays as it is; only the stored row
	// goes back to the default.
	if aden.CurrentTaxPercent() != 10 {
		t.Fatalf("Aden tax in force = %d, want 10", aden.CurrentTaxPercent())
	}
}

// TestManagerValidateTaxes pins CastleManager.validateTaxes: every castle
// whose rate in force is above the cap is brought down to it and stored;
// one at or under it is untouched, and the next rate is never capped.
func TestManagerValidateTaxes(t *testing.T) {
	for _, tt := range []struct {
		name          string
		max           int
		wantGludio    int
		wantAden      int
		wantWrites    []string
		wantRateFloat float64
	}{
		{"dusk cap", 5, 5, 5, []string{"currentTax 1=5", "currentTax 5=5"}, 0.05},
		{"no owner cap", 15, 15, 15, []string{"currentTax 1=15"}, 0.15},
		{"dawn cap", 25, 25, 15, nil, 0.25},
	} {
		t.Run(tt.name, func(t *testing.T) {
			m, store, _ := newTestManager(t, lords, rivals)
			gludio, aden := castleOf(t, m, 1), castleOf(t, m, 5)
			gludio.SetCurrentTaxPercent(25, false)
			gludio.SetNextTaxPercent(25, false)
			store.take()

			m.ValidateTaxes(tt.max)

			if got := store.take(); !slices.Equal(got, tt.wantWrites) {
				t.Fatalf("writes = %q, want %q", got, tt.wantWrites)
			}
			if gludio.CurrentTaxPercent() != tt.wantGludio || aden.CurrentTaxPercent() != tt.wantAden {
				t.Fatalf("tax in force = %d/%d, want %d/%d", gludio.CurrentTaxPercent(), aden.CurrentTaxPercent(), tt.wantGludio, tt.wantAden)
			}
			if got := gludio.TaxRate(); got != tt.wantRateFloat {
				t.Fatalf("Gludio TaxRate = %v, want %v", got, tt.wantRateFloat)
			}
			if got := gludio.NextTaxPercent(); got != 25 {
				t.Fatalf("Gludio next tax = %d, want 25 (never capped)", got)
			}
		})
	}
}

// TestManagerResetCertificates pins CastleManager.resetCertificates: every
// castle has 300 certificates again, stored.
func TestManagerResetCertificates(t *testing.T) {
	m, store, _ := newTestManager(t, lords, 0)
	gludio, aden := castleOf(t, m, 1), castleOf(t, m, 5)
	gludio.SetLeftCertificates(12, false)
	aden.SetLeftCertificates(0, false)

	m.ResetCertificates()

	if got, want := store.take(), []string{"certificates 1=300", "certificates 5=300"}; !slices.Equal(got, want) {
		t.Fatalf("writes = %q, want %q", got, want)
	}
	if gludio.LeftCertificates() != 300 || aden.LeftCertificates() != 300 {
		t.Fatalf("certificates = %d/%d, want 300/300", gludio.LeftCertificates(), aden.LeftCertificates())
	}
}

// TestNilManagerTaxes pins that a server without castle data runs the
// period hooks as no-ops.
func TestNilManagerTaxes(t *testing.T) {
	var m *Manager
	m.UpdateTaxes()
	m.ValidateTaxes(5)
	m.ResetCertificates()
}
