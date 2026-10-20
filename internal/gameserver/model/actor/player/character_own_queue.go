package player

// RunOnOwnQueue runs fn, which the caller runs from a task of the
// character's own queue, so that OnOwnQueue reports true inside fn on the
// calling goroutine. Nowhere else, not even on a goroutine fn hands work
// to, does it report true.
func (c *Character) RunOnOwnQueue(fn func()) { c.ownQueue.Run(fn) }

// OnOwnQueue reports whether the caller runs inside RunOnOwnQueue of the
// character: it may then act on the character's queue-owned state inside
// the call, as TeleportOnQueue does.
func (c *Character) OnOwnQueue() bool { return c.ownQueue.Held() }
