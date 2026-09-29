package event

// OwnerInfoChanged reports that a summon's owner-only info window is stale.
type OwnerInfoChanged struct{}

// Damaged reports direct damage a summon took from a named attacker.
type Damaged struct {
	AttackerName string
	Damage       int32
}

// ExpGained reports experience a pet earned.
type ExpGained struct{ Exp int64 }

// Unsummoning reports that a summon is about to leave the world for good.
// It is emitted once, while the summon and its owner are both still in world
// state, so whatever the summon's departure must settle -- a pet's row and
// inventory -- is settled before anyone is told it is gone.
type Unsummoning struct{}

// SummonRemoved reports that the owner's active summon slot was cleared,
// before the summon leaves the world.
type SummonRemoved struct{}

// PetCorpseDecayed reports that a dead pet's corpse decayed and has left
// the world. The pet is gone for good: its owner loses the collar and the
// pet's saved state with it.
type PetCorpseDecayed struct{}

func (OwnerInfoChanged) event() {}
func (Damaged) event()          {}
func (ExpGained) event()        {}
func (Unsummoning) event()      {}
func (SummonRemoved) event()    {}
func (PetCorpseDecayed) event() {}
