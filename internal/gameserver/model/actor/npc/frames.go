package npc

import "errors"

// ErrNoWorld is returned by an EffectPoint broadcast when Attach installed
// no world — the actor has no known-observer list to broadcast to.
var ErrNoWorld = errors.New("npc: no world attached")
