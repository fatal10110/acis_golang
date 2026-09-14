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

func (OwnerInfoChanged) event() {}
func (Damaged) event()          {}
func (ExpGained) event()        {}
