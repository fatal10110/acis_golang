package character

import (
	"testing"

	"github.com/fatal10110/acis_golang/internal/gameserver/castle"
	"github.com/fatal10110/acis_golang/internal/gameserver/model/residence"
	castledata "github.com/fatal10110/acis_golang/internal/gameserver/model/residence/castle"
	"github.com/fatal10110/acis_golang/internal/gameserver/sevensigns"
	"github.com/fatal10110/acis_golang/internal/gameservertest"
)

// ssqCastles holds castles 1 and 2 at the shipped default tax rate.
func ssqCastles(t *testing.T) gameservertest.Option {
	t.Helper()
	var all []*castledata.Castle
	for _, id := range []int{1, 2} {
		c, err := castledata.NewCastle(castledata.CastleAttrs{
			ID: id, Alias: []string{"", "gludio_castle", "dion_castle"}[id], Name: "Castle", Tax: residence.Tax{Rate: 15},
		}, nil, nil, nil, nil, nil)
		if err != nil {
			t.Fatal(err)
		}
		all = append(all, c)
	}
	table, err := castledata.NewTable(all)
	if err != nil {
		t.Fatal(err)
	}
	return gameservertest.WithCastles(table)
}

// storedCastle reads castle id's stored tax in force and certificates.
func storedCastle(t *testing.T, w *sevenSignsWorld, id int) (tax, certificates int) {
	t.Helper()
	w.srv.FlushPersistence(t)
	if err := w.srv.DB.QueryRow("SELECT currentTaxPercent, certificates FROM castle WHERE id = ?", id).Scan(&tax, &certificates); err != nil {
		t.Fatal(err)
	}
	return tax, certificates
}

// TestPeriodChangesSettleCastles pins the castles' part of
// SevenSignsPeriodChange: as the competition ends with Dusk taking the
// Seal of Strife, CastleManager.validateTaxes caps every castle's tax in
// force at 5 and stores it (castles already under it are left alone); as
// recruiting ends, CastleManager.resetCertificates gives every castle 300
// certificates again, stored.
func TestPeriodChangesSettleCastles(t *testing.T) {
	t.Parallel()
	w := bootSevenSigns(t, sevenSignsSetup{
		status: func(row *sevensigns.StatusRow) {
			row.Period = sevensigns.Competition
			row.DuskStoneScore = 100
		},
		cabal: sevensigns.Dusk, // Newbie, Dusk's one member, chose Strife: 100%
	}, ssqCastles(t))
	get := func(id int) *castle.Castle {
		c, ok := w.srv.Castles.Get(id)
		if !ok {
			t.Fatalf("castle %d not loaded", id)
		}
		return c
	}
	gludio, dion := get(1), get(2)
	gludio.SetCurrentTaxPercent(20, true)
	dion.SetCurrentTaxPercent(3, true)

	w.change() // competition -> results

	if got := w.srv.SevenSigns.Record(0).Seals[2].Owner; got != sevensigns.Dusk {
		t.Fatalf("Seal of Strife owner = %v, want DUSK", got)
	}
	if gludio.CurrentTaxPercent() != 5 || dion.CurrentTaxPercent() != 3 {
		t.Fatalf("tax in force = %d/%d, want 5/3", gludio.CurrentTaxPercent(), dion.CurrentTaxPercent())
	}
	if tax, _ := storedCastle(t, w, 1); tax != 5 {
		t.Fatalf("stored Gludio tax = %d, want 5", tax)
	}
	if tax, _ := storedCastle(t, w, 2); tax != 3 {
		t.Fatalf("stored Dion tax = %d, want 3", tax)
	}

	w.change() // results -> seal validation
	w.change() // seal validation -> recruiting
	gludio.SetLeftCertificates(120, true)
	dion.SetLeftCertificates(0, true)
	if gludio.CurrentTaxPercent() != 5 {
		t.Fatalf("tax in force after seal validation = %d, want 5 (only the competition's end caps)", gludio.CurrentTaxPercent())
	}

	w.change() // recruiting -> competition

	if gludio.LeftCertificates() != 300 || dion.LeftCertificates() != 300 {
		t.Fatalf("certificates = %d/%d, want 300/300", gludio.LeftCertificates(), dion.LeftCertificates())
	}
	for _, id := range []int{1, 2} {
		if _, certificates := storedCastle(t, w, id); certificates != 300 {
			t.Fatalf("stored castle %d certificates = %d, want 300", id, certificates)
		}
	}
}
