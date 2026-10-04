package event

// CursedWeaponLost reports that this player died, at another's hand,
// holding a cursed weapon: the weapon drops or ends, and the death costs
// nothing else.
type CursedWeaponLost struct{}

func (CursedWeaponLost) event() {}

// CursedWeaponKill reports that this player killed another player while
// holding a cursed weapon. The kill feeds the weapon instead of counting as
// a PK or PvP kill.
type CursedWeaponKill struct{}

func (CursedWeaponKill) event() {}
