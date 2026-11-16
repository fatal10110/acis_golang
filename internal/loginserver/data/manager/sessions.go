package manager

import (
	"sync"

	"github.com/fatal10110/acis_golang/internal/link"
)

// SessionStore tracks the session key issued to each currently
// authenticated account. An entry lives until the game server claims it
// (PlayerAuthRequest), its login connection gives it up, or the login link
// expires it at the login timeout. Removals that may race a newer login for
// the same account go through Take, which only removes the exact key the
// caller holds.
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

// Take removes account's session if it is still key and reports whether it
// did. A newer session issued for the account in the meantime is left in
// place, so two callers holding the same key cannot both claim it.
func (s *SessionStore) Take(account string, key link.SessionKey) bool {
	s.mu.Lock()
	defer s.mu.Unlock()
	if cur, ok := s.sessions[account]; !ok || cur != key {
		return false
	}
	delete(s.sessions, account)
	return true
}
