package event

// ClanGateOpened reports that a clan gate portal opened on the character:
// the other online members of its clan are told.
type ClanGateOpened struct{}

func (ClanGateOpened) event() {}
