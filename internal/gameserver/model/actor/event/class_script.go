package event

// ClassChangeRequested reports a script moving the character to ClassID as
// every occupation change does: the change's visual,
// the new class and its template, the transfer message, the party and clan
// rows, and the automatic skills.
type ClassChangeRequested struct{ ClassID int }

func (ClassChangeRequested) event() {}

// BaseClassChangeRequested reports a script making ClassID the character's
// base class.
type BaseClassChangeRequested struct{ ClassID int }

func (BaseClassChangeRequested) event() {}

// UserInfoBroadcast reports a script showing the character anew: its own
// full view to itself, its public view
// to every player around it.
type UserInfoBroadcast struct{}

func (UserInfoBroadcast) event() {}
