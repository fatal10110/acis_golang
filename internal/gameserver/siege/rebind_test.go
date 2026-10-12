package siege

import (
	"testing"
	"time"

	"github.com/fatal10110/acis_golang/internal/gameserver/model/zone"
)

// TestRebindMovesTheSiegeOntoTheReloadedBattlefield pins a zone reload's
// hand-over: a siege under way moves onto the reloaded battlefield, which is
// on at once and holds the siege's points from then on, and the siege's end
// turns that one off, leaving the replaced battlefield as the reload left
// it. A castle with no reloaded battlefield is left with none.
func TestRebindMovesTheSiegeOntoTheReloadedBattlefield(t *testing.T) {
	w := newWorld(t, DefaultConfig())
	gludio := w.siege(1)
	if _, refused := gludio.RegisterAttacker(w.clan(rivals)); refused {
		t.Fatal("rivals refused")
	}
	w.only(1)
	w.start(saturday.Add(-time.Second))
	w.advanceTo(saturday)
	if !gludio.InProgress() || !w.field.Active() {
		t.Fatal("siege not under way at its date")
	}

	form, err := zone.NewCuboid(4000, 6000, -1000, 1000, -1000, 1000)
	if err != nil {
		t.Fatal(err)
	}
	reloaded := zone.NewSiege(11, form)
	reloaded.ResidenceID = 1
	w.e.Rebind([]*zone.Siege{reloaded})
	if gludio.Battlefield() != reloaded || !reloaded.Active() {
		t.Fatalf("after the rebind the battlefield is %p (active %v), want the reloaded one on", gludio.Battlefield(), reloaded.Active())
	}
	if s, ok := w.e.ActiveAt(5000, 0, 0); !ok || s != gludio {
		t.Fatal("ActiveAt inside the reloaded battlefield found no siege")
	}
	if _, ok := w.e.ActiveAt(10, 10, 10); ok {
		t.Fatal("ActiveAt inside the replaced battlefield still found the siege")
	}
	if dion := w.siege(2); dion.Battlefield() != nil {
		t.Fatal("a castle with no reloaded battlefield got one")
	}

	w.advanceTo(saturday.Add(2 * time.Hour))
	if gludio.InProgress() || reloaded.Active() {
		t.Fatal("the siege's end left the reloaded battlefield on")
	}
	if !w.field.Active() {
		t.Fatal("the siege's end switched the replaced battlefield")
	}
}
