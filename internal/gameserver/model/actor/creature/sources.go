package creature

import (
	"sync"
	"sync/atomic"
)

// DirectSource is the source SetInvul and SetImmobilized hold on behalf of
// an explicit command or script (a GM toggle, a GM login flag, a hall
// script). Effects and other mechanisms hold their own sources.
type DirectSource struct{}

// SourceSet is a status, such as invulnerability or a movement lock, held
// while any of its sources is active. Ending one source leaves the status
// held while another is still active; it ends only with the last one
// (#5084). The zero value holds nothing and is safe for concurrent use.
type SourceSet struct {
	mu      sync.Mutex
	sources map[any]struct{}
	// held mirrors len(sources) > 0 so readers skip the mutex.
	held atomic.Bool
}

// Set adds src when on and removes it otherwise, and reports whether the
// status itself changed. src must be comparable; setting a held source again
// or clearing one not held changes nothing.
func (s *SourceSet) Set(src any, on bool) bool {
	s.mu.Lock()
	defer s.mu.Unlock()
	if on {
		if s.sources == nil {
			s.sources = make(map[any]struct{}, 1)
		}
		s.sources[src] = struct{}{}
	} else {
		delete(s.sources, src)
	}
	held := len(s.sources) > 0
	return s.held.Swap(held) != held
}

// Held reports whether any source is active.
func (s *SourceSet) Held() bool { return s.held.Load() }
