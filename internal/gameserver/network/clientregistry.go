package network

import "sync"

// ClientRegistry tracks, for each account name, the game client connection
// currently claiming it. A game server keeps exactly one such registry for
// its whole process lifetime, shared by every connection.
type ClientRegistry struct {
	mu      sync.Mutex
	clients map[string]*Client
}

// NewClientRegistry returns a ClientRegistry with no accounts claimed.
func NewClientRegistry() *ClientRegistry {
	return &ClientRegistry{clients: make(map[string]*Client)}
}

// Take claims accountName for client, replacing whatever connection held it
// before. It reports the previously registered connection, if any, so the
// caller can evict it: a second AuthLogin for an account in use takes the
// account over rather than being rejected, matching LoginServerThread.addClient
// (LoginServerThread.java:292-304).
func (r *ClientRegistry) Take(accountName string, client *Client) (evicted *Client, replaced bool) {
	r.mu.Lock()
	defer r.mu.Unlock()
	prev, ok := r.clients[accountName]
	r.clients[accountName] = client
	return prev, ok
}

// Release drops client's claim on accountName, but only while client is
// still the registered owner: a stale caller's cleanup must not undo a later
// Take that already replaced it.
func (r *ClientRegistry) Release(accountName string, client *Client) {
	r.mu.Lock()
	defer r.mu.Unlock()
	if r.clients[accountName] == client {
		delete(r.clients, accountName)
	}
}
