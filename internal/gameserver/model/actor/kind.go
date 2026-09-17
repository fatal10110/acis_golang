package actor

// Kind identifies the concrete sort of a world object. It is the one
// sanctioned discriminator for the rare branch that genuinely depends on
// what an object is rather than on what it can do.
type Kind uint8

const (
	// KindPlayer is a player character.
	KindPlayer Kind = iota + 1
	// KindNPC is any template-backed NPC: monsters, folk, decorations and
	// signet effect points.
	KindNPC
	// KindSummon is a pet or servitor.
	KindSummon
	// KindDoor is a door.
	KindDoor
	// KindStatic is a static world object.
	KindStatic
	// KindItem is an item lying on the ground.
	KindItem
)

// Playable reports whether k is a player-controlled creature: a player or
// its summon.
func (k Kind) Playable() bool { return k == KindPlayer || k == KindSummon }
