package event

// ClanKill reports that this player, a clan member, died outside an arena
// to the player KillerID (or its summon), a member of the clan KillerClanID.
// The clans settle the reputation a kill between clans at war moves.
type ClanKill struct {
	KillerID     int32
	KillerClanID int32
}

func (ClanKill) event() {}
