package network

import "sync"

// AuthFunc validates a login attempt for accountName, reporting whether the
// session is authorized to move a client from StateConnected to
// StateAuthed. The real check runs over the gameserver-to-login-server
// link (tracked separately); callers without that link wired up yet inject
// a stub.
type AuthFunc func(accountName string) bool

// Client is one connected game client: its framed, encrypted session plus
// its position in the connect-to-in-world state machine described by
// State. state and accountName are guarded by mu, since a login-server
// reply or a scheduled callback can reach them from a goroutine other than
// the one reading the connection.
type Client struct {
	Session *Session

	mu          sync.Mutex
	state       State
	accountName string
}

// NewClient returns a Client wrapping session, starting in StateConnected.
func NewClient(session *Session) *Client {
	return &Client{Session: session, state: StateConnected}
}

// State returns the client's current state.
func (c *Client) State() State {
	c.mu.Lock()
	defer c.mu.Unlock()
	return c.state
}

// SetState moves the client to s.
func (c *Client) SetState(s State) {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.state = s
}

// AccountName returns the account name recorded by a successful
// Authenticate call, or "" before that.
func (c *Client) AccountName() string {
	c.mu.Lock()
	defer c.mu.Unlock()
	return c.accountName
}

// Authenticate validates accountName with auth and, on success, records the
// account name and advances the client to StateAuthed. It reports whether
// authentication succeeded; on failure the client's state and account name
// are left unchanged.
func (c *Client) Authenticate(accountName string, auth AuthFunc) bool {
	if !auth(accountName) {
		return false
	}
	c.mu.Lock()
	defer c.mu.Unlock()
	c.accountName = accountName
	c.state = StateAuthed
	return true
}

// Accept reports whether opcode is valid for the client's current state. A
// packet reader calls this before decoding a packet body and drops the
// packet (or disconnects, per the caller's abuse policy) when it returns
// false.
func (c *Client) Accept(opcode byte) bool {
	return Allowed(c.State(), opcode)
}
