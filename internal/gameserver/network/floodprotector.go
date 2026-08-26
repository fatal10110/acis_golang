package network

import "time"

// Reuse-gate identifiers, one per client action that carries a per-client
// reuse delay. The reference numbers these through a global enum; this port
// only needs the gates the current milestone wires (character selection
// family and server bypass), so the set grows as later systems port their
// own gated actions.
const (
	floodProtectorCharacterSelect = iota
	floodProtectorServerBypass
	numFloodProtectors
)

// performFloodProtected reports whether the action identified by protector
// may run at now, extending its reuse deadline when allowed. A delay of 0
// (or less) means the action is never rate-limited. While the deadline set
// by a previous allowed call still holds, the action is refused without
// touching it. Mirrors GameClient.performAction.
func (c *Client) performFloodProtected(protector int, delay time.Duration, now time.Time) bool {
	if delay <= 0 {
		return true
	}
	if c.floodProtectors[protector].After(now) {
		return false
	}
	c.floodProtectors[protector] = now.Add(delay)
	return true
}
