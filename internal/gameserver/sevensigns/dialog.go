package sevensigns

// PlayerContribScore returns the contribution score objectID's stones have
// earned this cycle; signed is false, and the score 0, when objectID has no
// sign-up row.
func (s *State) PlayerContribScore(objectID int32) (score int, signed bool) {
	s.mu.Lock()
	defer s.mu.Unlock()
	p, ok := s.players[objectID]
	if !ok {
		return 0, false
	}
	return p.ContributionScore, true
}

// WinningCabal returns the cabal ahead on total score, NoCabal on a tie.
func (s *State) WinningCabal() Cabal {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.winningCabalLocked()
}

// SealOwners returns each seal's owner, in Seals order.
func (s *State) SealOwners() [3]Cabal {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.row.SealOwners
}

// GnosisFollower reports whether objectID travels for the base ancient
// adena price: during seal validation, signed up for the cabal owning the
// Seal of Gnosis and having chosen that seal
// (TeleportLocation.getCalculatedPriceCount).
func (s *State) GnosisFollower(objectID int32) bool {
	s.mu.Lock()
	defer s.mu.Unlock()
	p, ok := s.players[objectID]
	gnosis, _ := sealIndex(Gnosis)
	return ok && s.row.Period == SealValidation && p.Cabal == s.row.SealOwners[gnosis] && p.Seal == Gnosis
}
