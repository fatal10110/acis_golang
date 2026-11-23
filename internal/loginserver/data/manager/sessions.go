package manager

import (
	"sync"

	"github.com/fatal10110/acis_golang/internal/link"
)

// NoServer is the server id of a session whose client has not chosen a game
// server yet; no linked server can claim such a session.
const NoServer = 0

// SessionStore tracks the session key issued to each currently
// authenticated account, with the game server the client chose to enter.
// An entry lives until that game server claims it (PlayerAuthRequest), its
// login connection gives it up, or the login link expires it at the login
// timeout. Removals that may race a newer login for the same account go
// through Take or Claim, which only remove the exact key the caller holds.
//
// mu guards sessions.
type SessionStore struct {
	mu       sync.Mutex
	sessions map[string]session
}

type session struct {
	key      link.SessionKey
	serverID int
}

// NewSessionStore returns an empty SessionStore.
func NewSessionStore() *SessionStore {
	return &SessionStore{sessions: make(map[string]session)}
}

// Put records key as account's current session, bound to serverID (NoServer
// while the client has not chosen a game server).
func (s *SessionStore) Put(account string, key link.SessionKey, serverID int) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.sessions[account] = session{key: key, serverID: serverID}
}

// Bind records serverID as the game server account's session key was handed
// over for, if account's session is still key, and reports whether it was.
// A later choice replaces an earlier one.
func (s *SessionStore) Bind(account string, key link.SessionKey, serverID int) bool {
	s.mu.Lock()
	defer s.mu.Unlock()
	cur, ok := s.sessions[account]
	if !ok || cur.key != key {
		return false
	}
	cur.serverID = serverID
	s.sessions[account] = cur
	return true
}

// Get returns account's current session key, if any.
func (s *SessionStore) Get(account string) (link.SessionKey, bool) {
	s.mu.Lock()
	defer s.mu.Unlock()
	cur, ok := s.sessions[account]
	return cur.key, ok
}

// Delete removes account's session, if any.
func (s *SessionStore) Delete(account string) {
	s.mu.Lock()
	defer s.mu.Unlock()
	delete(s.sessions, account)
}

// Take removes account's session if it is still key and reports whether it
// did, whatever server it is bound to. A newer session issued for the
// account in the meantime is left in place.
func (s *SessionStore) Take(account string, key link.SessionKey) bool {
	s.mu.Lock()
	defer s.mu.Unlock()
	if cur, ok := s.sessions[account]; !ok || cur.key != key {
		return false
	}
	delete(s.sessions, account)
	return true
}

// Claim removes account's session and reports true only if it is still key
// and bound to serverID, the game server presenting it: keys handed over for
// one server never validate on another (#4655). A refused claim leaves the
// session in place, and two servers holding the same keys cannot both claim
// it.
func (s *SessionStore) Claim(account string, key link.SessionKey, serverID int) bool {
	s.mu.Lock()
	defer s.mu.Unlock()
	cur, ok := s.sessions[account]
	if !ok || cur.key != key || cur.serverID == NoServer || cur.serverID != serverID {
		return false
	}
	delete(s.sessions, account)
	return true
}
