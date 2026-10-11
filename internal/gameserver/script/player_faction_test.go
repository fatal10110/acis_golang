package script

import "testing"

// TestPlayerVarkaKetraAllianceReadsStoredStanding pins
// Player.getAllianceWithVarkaKetra: the handle returns the character's
// stored standing unchanged, 0 for neither side, positive for Ketra and
// negative for Varka, with no clamp.
func TestPlayerVarkaKetraAllianceReadsStoredStanding(t *testing.T) {
	for _, standing := range []int{0, 3, -5} {
		p := playerAt(humanFighter, 1)
		p.character().SetVarkaKetraAlliance(standing)
		if got := p.VarkaKetraAlliance(); got != int32(standing) {
			t.Errorf("standing %d: VarkaKetraAlliance() = %d", standing, got)
		}
	}
}
