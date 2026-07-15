package enchant

import "sync"

// State owns per-player active enchant scroll selection. mu guards active.
type State struct {
	mu     sync.Mutex
	active map[int32]int32
}

// NewState returns an empty enchant selection state.
func NewState() *State {
	return &State{active: make(map[int32]int32)}
}

// Select records the selected enchant scroll and reports whether this is a new selection.
func (s *State) Select(playerID, scrollObjectID int32) bool {
	if s == nil {
		return false
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	first := s.active[playerID] == 0
	s.active[playerID] = scrollObjectID
	return first
}

// Active returns the currently selected enchant scroll object id.
func (s *State) Active(playerID int32) int32 {
	if s == nil {
		return 0
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.active[playerID]
}

// Clear removes the selected enchant scroll and reports whether one was present.
func (s *State) Clear(playerID int32) bool {
	if s == nil {
		return false
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.active[playerID] == 0 {
		return false
	}
	delete(s.active, playerID)
	return true
}
