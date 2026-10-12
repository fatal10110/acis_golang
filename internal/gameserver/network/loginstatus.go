package network

import (
	"sync"

	"github.com/fatal10110/acis_golang/internal/link"
)

// LoginStatus is what the game server tells the login server about itself
// beyond its registration: the status it is listed under, whether it is
// reserved for game masters, and its player cap. Every change is pushed to
// the login server at once; a later registration advertises the reserved
// flag and the cap as they then are. Its fields are guarded by mu.
type LoginStatus struct {
	send func(link.ServerStatus)

	mu         sync.Mutex
	typ        link.ServerType
	gmOnly     bool
	maxPlayers int32
	serverName string
}

// NewLoginStatus returns the status of a server booted reserved for game
// masters when gmOnly, capped at maxPlayers. send pushes a status change to
// the login server, dropping it while unlinked. The listed status starts
// as Auto even on a reserved server; registering advertises the
// reservation on its own.
func NewLoginStatus(gmOnly bool, maxPlayers int32, send func(link.ServerStatus)) *LoginStatus {
	return &LoginStatus{send: send, typ: link.ServerTypeAuto, gmOnly: gmOnly, maxPlayers: maxPlayers}
}

// Type is the status the server last listed itself under.
func (s *LoginStatus) Type() link.ServerType {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.typ
}

// SetType lists the server under t.
func (s *LoginStatus) SetType(t link.ServerType) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.setType(t)
}

func (s *LoginStatus) setType(t link.ServerType) {
	s.send(link.ServerStatus{Status: &t})
	s.typ = t
}

// GMOnly reports whether the server is reserved for game masters.
func (s *LoginStatus) GMOnly() bool {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.gmOnly
}

// SetGMOnly reserves the server for game masters, or opens it to everyone,
// and lists it so.
func (s *LoginStatus) SetGMOnly(on bool) {
	s.mu.Lock()
	defer s.mu.Unlock()
	if on {
		s.setType(link.ServerTypeGMOnly)
	} else {
		s.setType(link.ServerTypeAuto)
	}
	s.gmOnly = on
}

// MaxPlayers is the server's advertised player cap.
func (s *LoginStatus) MaxPlayers() int32 {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.maxPlayers
}

// SetMaxPlayers advertises n as the server's player cap.
func (s *LoginStatus) SetMaxPlayers(n int32) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.send(link.ServerStatus{MaxPlayers: &n})
	s.maxPlayers = n
}

// ServerName is the name the login server last registered the server
// under, empty before the first registration.
func (s *LoginStatus) ServerName() string {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.serverName
}

// Registered records the name the login server registered the server
// under.
func (s *LoginStatus) Registered(name string) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.serverName = name
}

// Auth returns base advertising the current player cap and reservation, as
// a new registration does. The listed status a registration sends is the
// reservation's, not Type: a server listed down before it lost its link is
// listed open again once it registers anew.
func (s *LoginStatus) Auth(base LoginServerAuth) LoginServerAuth {
	s.mu.Lock()
	defer s.mu.Unlock()
	base.MaxPlayers = s.maxPlayers
	t := link.ServerTypeAuto
	if s.gmOnly {
		t = link.ServerTypeGMOnly
	}
	base.InitialStatus.Status = &t
	return base
}
