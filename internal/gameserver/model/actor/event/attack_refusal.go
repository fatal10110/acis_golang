package event

// AttackRefusal is why the attack gate refuses a playable's attack think in a
// way its player is told.
type AttackRefusal uint8

const (
	// AttackRefusedFishingRod: a fishing rod never attacks.
	AttackRefusedFishingRod AttackRefusal = iota + 1
	// AttackRefusedNoArrows: a bow with no arrows to equip.
	AttackRefusedNoArrows
	// AttackRefusedNotEnoughMP: a bow whose MP cost exceeds the player's MP.
	AttackRefusedNotEnoughMP
	// AttackRefusedCantSeeTarget: the geodata hides the target.
	AttackRefusedCantSeeTarget
	// AttackRefusedInPeaceZone: the attacker stands in a peace zone and the
	// target is playable.
	AttackRefusedInPeaceZone
	// AttackRefusedTargetInPeaceZone: the playable target stands in a peace
	// zone.
	AttackRefusedTargetInPeaceZone
)

// AttackRefused reports a playable's attack think refused by the attack
// gate; its player (a summon's owner) is told why.
type AttackRefused struct{ Reason AttackRefusal }

func (AttackRefused) event() {}
