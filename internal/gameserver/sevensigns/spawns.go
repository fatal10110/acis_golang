package sevensigns

// Spawns is the NPC population whose Seven Signs groups follow the period.
// SevenSignsChanged is called without the state's lock held, after every
// period change has been saved and announced.
type Spawns interface {
	SevenSignsChanged()
}

// SetSpawns hands the period changes' effect on the Seven Signs NPC groups
// to spawns.
func (s *State) SetSpawns(spawns Spawns) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.spawns = spawns
}
