package network

import "time"

// Reuse-gate identifiers, one per client action that carries a per-client
// reuse delay. Only the gates the current milestone wires exist (character selection
// family, server bypass, crafting, multisell, dice, chat, seed purchase,
// emotes, item drops and subclass actions), so the set grows as later
// systems port their own gated actions. Each lives on the connection, so a
// restart or a new character selection does not reset it.
const (
	floodProtectorCharacterSelect = iota
	floodProtectorServerBypass
	floodProtectorManufacture
	floodProtectorMultisell
	floodProtectorRollDice
	floodProtectorGlobalChat
	floodProtectorTradeChat
	floodProtectorHeroVoice
	floodProtectorManor
	floodProtectorSocial
	floodProtectorDropItem
	floodProtectorSubclass
	numFloodProtectors
)

// performFloodProtected reports whether the action identified by protector
// may run at now, extending its reuse deadline when allowed. A delay of 0
// (or less) means the action is never rate-limited. While the deadline set
// by a previous allowed call still holds, the action is refused without
// touching it.
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
