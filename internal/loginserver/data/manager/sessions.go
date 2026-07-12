package manager

import (
	"sync"
	"time"

	"github.com/fatal10110/acis_golang/internal/link"
)

// SessionTTL is the grace window a login session may wait for a game server
// to claim it before a later login can replace the abandoned session.
const SessionTTL = time.Minute

// SessionStore tracks the session key issued to each currently
// authenticated account.
//
// mu guards sessions.
type SessionStore struct {
	mu       sync.RWMutex
	sessions map[string]sessionEntry
	ttl      time.Duration
	now      func() time.Time
}

type sessionEntry struct {
	key     link.SessionKey
	expires time.Time
}

// NewSessionStore returns an empty SessionStore.
func NewSessionStore() *SessionStore {
	return newSessionStore(SessionTTL, time.Now)
}

func newSessionStore(ttl time.Duration, now func() time.Time) *SessionStore {
	return &SessionStore{sessions: make(map[string]sessionEntry), ttl: ttl, now: now}
}

// Put records key as account's current session.
func (s *SessionStore) Put(account string, key link.SessionKey) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.sessions[account] = sessionEntry{key: key, expires: s.now().Add(s.ttl)}
}

// Get returns account's current session key, if any.
func (s *SessionStore) Get(account string) (link.SessionKey, bool) {
	s.mu.Lock()
	defer s.mu.Unlock()
	entry, ok := s.sessions[account]
	if !ok {
		return link.SessionKey{}, false
	}
	if !s.now().Before(entry.expires) {
		delete(s.sessions, account)
		return link.SessionKey{}, false
	}
	return entry.key, true
}

// Delete removes account's session, if any.
func (s *SessionStore) Delete(account string) {
	s.mu.Lock()
	defer s.mu.Unlock()
	delete(s.sessions, account)
}
