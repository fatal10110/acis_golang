package clan

// TakeReputation takes points from cl's reputation score. It reports the
// change, or false when there is none: a clan below level 5 neither gains
// nor loses reputation.
func (s *Service) TakeReputation(cl *Clan, points int) (ReputationChanged, bool) {
	cl.mu.Lock()
	defer cl.mu.Unlock()
	return s.addReputationLocked(cl, -points)
}

// AddReputation adds points to cl's reputation score. It reports the
// change, or false when there is none: a clan below level 5 neither gains
// nor loses reputation.
func (s *Service) AddReputation(cl *Clan, points int) (ReputationChanged, bool) {
	cl.mu.Lock()
	defer cl.mu.Unlock()
	return s.addReputationLocked(cl, points)
}
