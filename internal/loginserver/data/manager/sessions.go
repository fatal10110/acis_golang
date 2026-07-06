package manager

import "sync"

// SessionKey is the pair of session-key halves the login server issues an
// account at login (delivered via LoginOk) and again for the game server it
// chose to play on (delivered via PlayOk). A game server presents both
// pairs back over the link so the login server can confirm a connecting
// client's session is genuine.
type SessionKey struct {
	PlayKey1  int32
	PlayKey2  int32
	LoginKey1 int32
	LoginKey2 int32
}

// SessionStore tracks the session key issued to each currently
// authenticated account.
//
// mu guards sessions.
type SessionStore struct {
	mu       sync.RWMutex
	sessions map[string]SessionKey
}

// NewSessionStore returns an empty SessionStore.
func NewSessionStore() *SessionStore {
	return &SessionStore{sessions: make(map[string]SessionKey)}
}

// Put records key as account's current session.
func (s *SessionStore) Put(account string, key SessionKey) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.sessions[account] = key
}

// Get returns account's current session key, if any.
func (s *SessionStore) Get(account string) (SessionKey, bool) {
	s.mu.RLock()
	defer s.mu.RUnlock()
	key, ok := s.sessions[account]
	return key, ok
}

// Delete removes account's session, if any.
func (s *SessionStore) Delete(account string) {
	s.mu.Lock()
	defer s.mu.Unlock()
	delete(s.sessions, account)
}
