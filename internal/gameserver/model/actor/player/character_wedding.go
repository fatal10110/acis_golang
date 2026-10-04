package player

import "sync/atomic"

// marriageRequest is the marriage request a character sent or was asked
// to answer. The wedding manager's lock orders every change to it, made
// from either player's queue; any goroutine reads it.
type marriageRequest struct {
	pending     atomic.Bool
	requesterID atomic.Int32
}

// UnderMarryRequest reports a marriage request c sent or received that is
// not answered yet.
func (c *Character) UnderMarryRequest() bool { return c.marriage.pending.Load() }

// SetUnderMarryRequest marks or clears c's pending marriage request.
func (c *Character) SetUnderMarryRequest(pending bool) { c.marriage.pending.Store(pending) }

// MarryRequesterID is the player whose marriage request c was asked to
// answer, 0 when none waits on c.
func (c *Character) MarryRequesterID() int32 { return c.marriage.requesterID.Load() }

// SetMarryRequesterID records the player whose marriage request c is
// asked to answer.
func (c *Character) SetMarryRequesterID(id int32) { c.marriage.requesterID.Store(id) }
