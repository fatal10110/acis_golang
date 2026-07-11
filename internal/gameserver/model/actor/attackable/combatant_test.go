package attackable

// fakeCombatant is a minimal Combatant test double: no real creature type
// exists yet, so tests construct fixed identities and flags directly.
type fakeCombatant struct {
	id         int32
	siegeGuard bool
	alikeDead  bool
}

func (f *fakeCombatant) ObjectID() int32  { return f.id }
func (f *fakeCombatant) SiegeGuard() bool { return f.siegeGuard }
func (f *fakeCombatant) AlikeDead() bool  { return f.alikeDead }

func combatant(id int32) *fakeCombatant { return &fakeCombatant{id: id} }
