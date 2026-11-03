package clientpackets

// Olympiad spectating client packet sub-opcodes; neither carries a body.
const (
	// OpcodeRequestOlympiadObserverEnd leaves the stadium the player
	// watches.
	OpcodeRequestOlympiadObserverEnd uint16 = 0x0012
	// OpcodeRequestOlympiadMatchList asks a spectator for the stadiums'
	// list.
	OpcodeRequestOlympiadMatchList uint16 = 0x0013
)
