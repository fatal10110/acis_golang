package manager

import (
	"sync"

	"github.com/fatal10110/acis_golang/internal/loginserver/link"
)

// ServerEntry is a game server's registration state: its auth key, whether
// it currently holds a live link connection, and the status it last
// reported.
type ServerEntry struct {
	ID         int
	HexID      []byte
	Authed     bool
	Host       string
	Port       uint16
	MaxPlayers int32
	Status     link.ServerType
	ShowClock  bool
	Brackets   bool
	TestServer bool
	Pvp        bool
	AgeLimit   int32
}

// ServerRegistry tracks every game server known to this login server —
// seeded from the database at boot, updated as game servers connect over
// the link — plus the accounts each currently reports online.
//
// mu guards servers and online.
type ServerRegistry struct {
	mu      sync.RWMutex
	servers map[int]ServerEntry
	online  map[int]map[string]struct{}
}

// NewServerRegistry returns an empty ServerRegistry.
func NewServerRegistry() *ServerRegistry {
	return &ServerRegistry{
		servers: make(map[int]ServerEntry),
		online:  make(map[int]map[string]struct{}),
	}
}

// Load seeds the registry with previously-registered servers (id -> hex
// auth key), as read from the database at boot. Entries start offline.
func (r *ServerRegistry) Load(known map[int][]byte) {
	r.mu.Lock()
	defer r.mu.Unlock()
	for id, hexID := range known {
		r.servers[id] = ServerEntry{ID: id, HexID: hexID, Status: link.ServerTypeDown}
	}
}

// Get returns a snapshot of the entry at id.
func (r *ServerRegistry) Get(id int) (ServerEntry, bool) {
	r.mu.RLock()
	defer r.mu.RUnlock()
	e, ok := r.servers[id]
	return e, ok
}

// Register creates an offline entry at id with hexID, failing if id is
// already registered.
func (r *ServerRegistry) Register(id int, hexID []byte) (ServerEntry, bool) {
	r.mu.Lock()
	defer r.mu.Unlock()
	if _, exists := r.servers[id]; exists {
		return ServerEntry{}, false
	}
	e := ServerEntry{ID: id, HexID: hexID, Status: link.ServerTypeDown}
	r.servers[id] = e
	return e, true
}

// RegisterFirst creates an offline entry with hexID at the first id in
// candidateIDs not already registered, failing if none is free.
func (r *ServerRegistry) RegisterFirst(candidateIDs []int, hexID []byte) (ServerEntry, bool) {
	r.mu.Lock()
	defer r.mu.Unlock()
	for _, id := range candidateIDs {
		if _, exists := r.servers[id]; !exists {
			e := ServerEntry{ID: id, HexID: hexID, Status: link.ServerTypeDown}
			r.servers[id] = e
			return e, true
		}
	}
	return ServerEntry{}, false
}

// MarkOnline marks the entry at id authed with the given connection
// details, failing if id is not registered.
func (r *ServerRegistry) MarkOnline(id int, host string, port uint16, maxPlayers int32) (ServerEntry, bool) {
	r.mu.Lock()
	defer r.mu.Unlock()
	e, ok := r.servers[id]
	if !ok {
		return ServerEntry{}, false
	}
	e.Authed = true
	e.Host = host
	e.Port = port
	e.MaxPlayers = maxPlayers
	r.servers[id] = e
	return e, true
}

// MarkOffline marks the entry at id as disconnected and clears its online
// account set, e.g. once its link connection drops.
func (r *ServerRegistry) MarkOffline(id int) {
	r.mu.Lock()
	defer r.mu.Unlock()
	e, ok := r.servers[id]
	if !ok {
		return
	}
	e.Authed = false
	e.Port = 0
	e.Status = link.ServerTypeDown
	r.servers[id] = e
	delete(r.online, id)
}

// ApplyStatus applies the attributes status carries (nil fields left
// unchanged) to the entry at id, failing if id is not registered.
func (r *ServerRegistry) ApplyStatus(id int, status link.ServerStatus) (ServerEntry, bool) {
	r.mu.Lock()
	defer r.mu.Unlock()
	e, ok := r.servers[id]
	if !ok {
		return ServerEntry{}, false
	}
	if status.Status != nil {
		e.Status = *status.Status
	}
	if status.ShowClock != nil {
		e.ShowClock = *status.ShowClock
	}
	if status.ShowBrackets != nil {
		e.Brackets = *status.ShowBrackets
	}
	if status.AgeLimit != nil {
		e.AgeLimit = *status.AgeLimit
	}
	if status.TestServer != nil {
		e.TestServer = *status.TestServer
	}
	if status.Pvp != nil {
		e.Pvp = *status.Pvp
	}
	if status.MaxPlayers != nil {
		e.MaxPlayers = *status.MaxPlayers
	}
	r.servers[id] = e
	return e, true
}

// AddOnlineAccount records account as online on server id.
func (r *ServerRegistry) AddOnlineAccount(id int, account string) {
	r.mu.Lock()
	defer r.mu.Unlock()
	set, ok := r.online[id]
	if !ok {
		set = make(map[string]struct{})
		r.online[id] = set
	}
	set[account] = struct{}{}
}

// RemoveOnlineAccount removes account from server id's online set.
func (r *ServerRegistry) RemoveOnlineAccount(id int, account string) {
	r.mu.Lock()
	defer r.mu.Unlock()
	delete(r.online[id], account)
}

// OnlineAccountCount returns how many accounts server id currently reports
// online.
func (r *ServerRegistry) OnlineAccountCount(id int) int {
	r.mu.RLock()
	defer r.mu.RUnlock()
	return len(r.online[id])
}
