package event

// ClanKill reports that this player, a clan member, died outside an arena
// to the player KillerID (or its summon), a member of the clan KillerClanID.
// The clans settle the reputation a kill between clans at war moves.
type ClanKill struct {
	KillerID     int32
	KillerClanID int32
}

func (ClanKill) event() {}

// SiegeKill reports that this player died to a player of the opposite
// siege side, both in a PvP zone: the clan KillerClanID scores a siege
// kill and the clan VictimClanID a siege death. A side without a clan
// (0) scores nothing.
type SiegeKill struct {
	KillerClanID int32
	VictimClanID int32
}

func (SiegeKill) event() {}
