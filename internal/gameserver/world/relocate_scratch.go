package world

// RelocateScratch is reusable notification scratch for a Player's region
// crossings, letting relocate reuse the same grown slice across calls
// instead of allocating a fresh one on every crossing. Embed it (by
// value, addressed through a pointer receiver) in a Player type; the zero
// value is ready to use. Non-player Tracked types (NPCs, ground items) must
// not embed it — relocate only uses it for players, so every other Presence
// would carry a buffer it never fills.
type RelocateScratch struct {
	notifications []visibilityNotification
}

// relocateScratch exposes the embedded buffer to relocate, mirroring how
// Presence.presence() exposes Presence to State.
func (s *RelocateScratch) relocateScratch() *RelocateScratch { return s }

// relocateScratchOwner is implemented by Player types that embed
// RelocateScratch. relocate type-asserts for it and, when present, reuses
// its buffer instead of making a fresh one sized for the crossing.
type relocateScratchOwner interface {
	relocateScratch() *RelocateScratch
}
