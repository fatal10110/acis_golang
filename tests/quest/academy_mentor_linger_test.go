package quest

import (
	"testing"

	"github.com/fatal10110/acis_golang/internal/gameserver/script"
)

// TestScriptAcademyMentorsGoneOnDrop pins Quest.getSponsor and
// getApprentice (the Q118 and Q123 helpers) against a mentor whose client
// dropped in combat: it lingers in the world beside the player, but it is
// offline from the drop, so it does not count.
func TestScriptAcademyMentorsGoneOnDrop(t *testing.T) {
	t.Parallel()
	srv, objID := bootHelpers(t, academyOf(t))
	chief := enterChief(t, srv)
	run(t, srv, 900001, func(sc *script.Script, p *script.Player) {
		if got := sc.ApprenticeInRange(p); got == nil || got.ObjectID() != objID {
			t.Fatalf("ApprenticeInRange with the apprentice online beside the sponsor = %v, want %d", got, objID)
		}
	})

	srv.DropInCombat(t, objID, srv.Client)
	run(t, srv, 900001, func(sc *script.Script, p *script.Player) {
		if got := sc.ApprenticeInRange(p); got != nil {
			t.Errorf("ApprenticeInRange with the apprentice's client gone = %d, want nil", got.ObjectID())
		}
	})
	run(t, srv, objID, func(sc *script.Script, p *script.Player) {
		if !sc.SponsorInRange(p) {
			t.Error("SponsorInRange of the lingering apprentice, its sponsor online beside it = false")
		}
	})

	srv.DropInCombat(t, 900001, chief)
	run(t, srv, objID, func(sc *script.Script, p *script.Player) {
		if sc.SponsorInRange(p) {
			t.Error("SponsorInRange with the sponsor's client gone = true")
		}
	})
}
