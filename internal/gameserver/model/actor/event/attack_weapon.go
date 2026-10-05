package event

// AttackWeaponRefusal is why a player's active weapon refuses an attack.
type AttackWeaponRefusal uint8

const (
	// AttackRefusedFishingRod: a fishing rod never attacks.
	AttackRefusedFishingRod AttackWeaponRefusal = iota + 1
	// AttackRefusedNoArrows: a bow with no arrows to equip.
	AttackRefusedNoArrows
	// AttackRefusedNotEnoughMP: a bow whose MP cost exceeds the player's MP.
	AttackRefusedNotEnoughMP
)

// AttackWeaponRefused reports a player's attack think refused by the active
// weapon; the player is told why.
type AttackWeaponRefused struct{ Reason AttackWeaponRefusal }

func (AttackWeaponRefused) event() {}
