package manager

import (
	"sync"

	"github.com/fatal10110/acis_golang/internal/link"
)

// SessionStore tracks the session key issued to each currently
// authenticated account. An entry lives until the game server claims it
// (PlayerAuthRequest) or its login connection is torn down; there is no
// time-based expiry.
//
// mu guards sessions.
type SessionStore struct {
	mu       sync.Mutex
	sessions map[string]link.SessionKey
}

// NewSessionStore returns an empty SessionStore.
func NewSessionStore() *SessionStore {
	return &SessionStore{sessions: make(map[string]link.SessionKey)}
}

// Put records key as account's current session.
func (s *SessionStore) Put(account string, key link.SessionKey) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.sessions[account] = key
}

// Get returns account's current session key, if any.
func (s *SessionStore) Get(account string) (link.SessionKey, bool) {
	s.mu.Lock()
	defer s.mu.Unlock()
	key, ok := s.sessions[account]
	return key, ok
}

// Delete removes account's session, if any.
func (s *SessionStore) Delete(account string) {
	s.mu.Lock()
	defer s.mu.Unlock()
	delete(s.sessions, account)
}
