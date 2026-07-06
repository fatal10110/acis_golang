package network

// SessionKey is the pair of session-key halves a login server issues an
// account: one delivered at login, the other for the specific game server
// the account chose to play on. A game client presents both back to this
// game server, which confirms them with the login server before treating
// the connection as authenticated.
type SessionKey struct {
	LoginKey1 int32
	LoginKey2 int32
	PlayKey1  int32
	PlayKey2  int32
}
