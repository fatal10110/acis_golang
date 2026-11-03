package summon

// OffBattlefield reports whether this summon is a siege summon its zones do
// not hold on an active siege battlefield, the only ground it may stand on.
func (a *Actor) OffBattlefield() bool { return a.SiegeSummon() && !a.InSiegeZone() }

// Vanish dismisses this summon and tells its owner its servitor has
// vanished. A dead summon stays where it lies, but its owner is told all
// the same.
func (a *Actor) Vanish() {
	a.Unsummon()
	if o := a.currentOwner(); o != nil {
		o.ServitorVanished()
	}
}
