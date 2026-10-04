package player

import "sync/atomic"

// olympiadStanding is the character's place in an Olympiad match. The
// match, the character's own queue and the queues of the checks reading it
// (attack, cast, trade, item use) all touch it, so every field is atomic.
// gameID and side hold the value plus one, so the zero value reads as
// "none" (-1).
type olympiadStanding struct {
	mode    atomic.Bool
	started atomic.Bool
	gameID  atomic.Int32
	side    atomic.Int32
}

// OlympiadMode reports whether the character is a competitor of an
// Olympiad match, from its arrival in the stadium until it is sent back.
func (c *Character) OlympiadMode() bool { return c.olympiad.mode.Load() }

// SetOlympiadMode marks the character a competitor of an Olympiad match,
// or clears it.
func (c *Character) SetOlympiadMode(on bool) { c.olympiad.mode.Store(on) }

// OlympiadStarted reports whether the character's Olympiad match is past
// its countdown and the competitors may fight.
func (c *Character) OlympiadStarted() bool { return c.olympiad.started.Load() }

// SetOlympiadStarted marks the character's match as fighting, or not.
func (c *Character) SetOlympiadStarted(on bool) { c.olympiad.started.Store(on) }

// OlympiadGameID is the stadium of the match the character competes in or
// watches, -1 when none.
func (c *Character) OlympiadGameID() int { return int(c.olympiad.gameID.Load()) - 1 }

// SetOlympiadGameID records the stadium of the character's match; -1
// clears it.
func (c *Character) SetOlympiadGameID(id int) { c.olympiad.gameID.Store(int32(id + 1)) }

// OlympiadSide is the side, 1 or 2, the character competes on, -1 when
// none.
func (c *Character) OlympiadSide() int { return int(c.olympiad.side.Load()) - 1 }

// SetOlympiadSide records the side the character competes on; -1 clears
// it.
func (c *Character) SetOlympiadSide(side int) { c.olympiad.side.Store(int32(side + 1)) }
