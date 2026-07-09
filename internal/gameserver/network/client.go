package network

import (
	"sync"

	"github.com/fatal10110/acis_golang/internal/link"
)

// Client is one connected game client: its framed, encrypted session plus
// its position in the connect-to-in-world state machine described by
// State. state, accountName, and sessionKey are guarded by mu, since a
// login-server reply or a scheduled callback can reach them from a
// goroutine other than the one reading the connection.
type Client struct {
	Session *Session

	mu          sync.RWMutex
	state       State
	accountName string
	sessionKey  link.SessionKey
}

// NewClient returns a Client wrapping session, starting in StateConnected.
func NewClient(session *Session) *Client {
	return &Client{Session: session, state: StateConnected}
}

// State returns the client's current state.
func (c *Client) State() State {
	c.mu.RLock()
	defer c.mu.RUnlock()
	return c.state
}

// SetState moves the client to s.
func (c *Client) SetState(s State) {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.state = s
}

// AccountName returns the account name recorded by a successful
// SetAuthenticated call, or "" before that.
func (c *Client) AccountName() string {
	c.mu.RLock()
	defer c.mu.RUnlock()
	return c.accountName
}

// SessionKey returns the session key recorded by a successful
// SetAuthenticated call, or the zero value before that.
func (c *Client) SessionKey() link.SessionKey {
	c.mu.RLock()
	defer c.mu.RUnlock()
	return c.sessionKey
}

// SetAuthenticated records accountName and key as the result of a
// successful login-server validation and advances the client to
// StateAuthed. The caller (SessionValidator.Validate) is responsible for
// having already confirmed the session before calling this.
func (c *Client) SetAuthenticated(accountName string, key link.SessionKey) {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.accountName = accountName
	c.sessionKey = key
	c.state = StateAuthed
}

// Accept reports whether opcode is valid for the client's current state. A
// packet reader calls this before decoding a packet body and drops the
// packet (or disconnects, per the caller's abuse policy) when it returns
// false.
func (c *Client) Accept(opcode byte) bool {
	return Allowed(c.State(), opcode)
}
