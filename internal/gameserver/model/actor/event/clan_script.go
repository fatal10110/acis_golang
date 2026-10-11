package event

// ClanReputationAdded reports a script adding Points to the reputation of
// the character's clan.
type ClanReputationAdded struct{ Points int }

func (ClanReputationAdded) event() {}

// ClanInfoBroadcast reports a script showing every online member of the
// character's clan the clan's header.
type ClanInfoBroadcast struct{}

func (ClanInfoBroadcast) event() {}
