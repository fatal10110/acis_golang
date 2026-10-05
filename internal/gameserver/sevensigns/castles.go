package sevensigns

// Castles is the castles as the period changes act on them. Its methods
// are called without the state's lock held.
type Castles interface {
	// ResetCertificates gives every castle its certificates back, as
	// recruiting ends.
	ResetCertificates()
	// ValidateTaxes brings every castle's tax rate in force above
	// maxPercent down to it, as the competition ends.
	ValidateTaxes(maxPercent int)
}

// SetCastles hands the period changes' effects on the castles to castles.
func (s *State) SetCastles(castles Castles) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.castles = castles
}

// castleTaxCap is the highest castle tax rate, in percent, the Seal of
// Strife's owner allows: 25 under the dawn, 5 under the dusk, 15 when no
// cabal owns it.
func castleTaxCap(strifeOwner Cabal) int {
	switch strifeOwner {
	case Dawn:
		return 25
	case Dusk:
		return 5
	default:
		return 15
	}
}
