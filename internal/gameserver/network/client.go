package network

import (
	"sync"
	"time"

	"github.com/fatal10110/acis_golang/internal/link"
)

// Disconnect thresholds for the sliding 60s abuse counters below, matching
// Config.CLIENT_PACKET_QUEUE_MAX_UNDERFLOWS_PER_MIN /
// _MAX_UNKNOWN_PER_MIN's hardcoded defaults (not file-configurable in the
// reference either).
const (
	maxUnderflowsPerMin = 1
	maxUnknownPerMin    = 5
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

	underflowCount       int
	underflowWindowStart time.Time
	unknownCount         int
	unknownWindowStart   time.Time

	// stats and floodProtectors are owned by the connection's read-loop
	// goroutine: both track per-frame progress along the read path, which
	// no other goroutine observes.
	stats           clientStats
	floodProtectors [numFloodProtectors]time.Time
}

// NewClient returns a Client wrapping session, starting in StateConnected.
func NewClient(session *Session) *Client {
	return &Client{Session: session, state: StateConnected, stats: newClientStats()}
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

// countUnderflow records one malformed-packet (buffer underflow) occurrence
// in the current 60s sliding window and reports whether the per-minute
// threshold has now been exceeded, mirroring ClientStats.countUnderflowException.
func (c *Client) countUnderflow() bool {
	return countInWindow(&c.mu, &c.underflowCount, &c.underflowWindowStart, maxUnderflowsPerMin)
}

// countUnknownPacket records one unknown/rejected-opcode occurrence in the
// current 60s sliding window and reports whether the per-minute threshold
// has now been exceeded, mirroring ClientStats.countUnknownPacket.
func (c *Client) countUnknownPacket() bool {
	return countInWindow(&c.mu, &c.unknownCount, &c.unknownWindowStart, maxUnknownPerMin)
}

// countInWindow implements the reference's sliding-60s-window counters: a
// tick more than 60s after windowStart resets the window to 1 and reports
// no threshold breach; otherwise it increments and reports whether count
// now exceeds max.
func countInWindow(mu *sync.RWMutex, count *int, windowStart *time.Time, max int) bool {
	mu.Lock()
	defer mu.Unlock()

	tick := time.Now()
	if tick.Sub(*windowStart) > 60*time.Second {
		*windowStart = tick
		*count = 1
		return false
	}

	*count++
	return *count > max
}
