package npc

import "errors"

// ErrNoWorld is returned by an EffectPoint broadcast when SetWorld has not
// been called yet — the actor has no known-observer list to broadcast to.
var ErrNoWorld = errors.New("npc: SetWorld not called")
