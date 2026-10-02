package player

// PetMountRefusal is why a character may not ride its pet, or
// PetMountAllowed.
type PetMountRefusal uint8

const (
	PetMountAllowed PetMountRefusal = iota
	// PetMountRiderDead: the rider is dead.
	PetMountRiderDead
	// PetMountPetDead: the pet is dead.
	PetMountPetDead
	// PetMountPetInBattle: the pet is in combat or rooted.
	PetMountPetInBattle
	// PetMountRiderInBattle: the rider is in combat or holds a cursed
	// weapon.
	PetMountRiderInBattle
	// PetMountSitting: the rider sits.
	PetMountSitting
	// PetMountFishing: the rider is fishing.
	PetMountFishing
	// PetMountTooFar: the pet is out of reach.
	PetMountTooFar
	// PetMountHungry: the pet is fed below its hungry limit.
	PetMountHungry
)

// RideablePet is the state of the pet a character asks to ride that the
// mount checks read.
type RideablePet struct {
	Dead     bool
	InCombat bool
	Rooted   bool
	// InReach is whether the pet stands within the mount range of its
	// rider.
	InReach bool
	Hungry  bool
}

// PetMountRefusal runs the checks for c riding pet, in their order, and
// returns the first that refuses.
func (c *Character) PetMountRefusal(pet RideablePet) PetMountRefusal {
	switch {
	case c.Dead():
		return PetMountRiderDead
	case pet.Dead:
		return PetMountPetDead
	case pet.InCombat || pet.Rooted:
		return PetMountPetInBattle
	case c.InCombat() || c.CursedWeaponEquipped():
		return PetMountRiderInBattle
	case !c.Standing():
		return PetMountSitting
	case c.Fishing():
		return PetMountFishing
	case !pet.InReach:
		return PetMountTooFar
	case pet.Hungry:
		return PetMountHungry
	}
	return PetMountAllowed
}
