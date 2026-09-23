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

func (OwnerInfoChanged) event() {}
func (Damaged) event()          {}
func (ExpGained) event()        {}
func (Unsummoning) event()      {}
