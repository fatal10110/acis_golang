package gameservertest

import (
	"database/sql"
	"testing"
)

// HeroMinMatches is the number of Olympiad matches a noble needs to be
// elected hero in a booted server, the reference default.
const HeroMinMatches = 5

// WithOlympiadSeed adjusts the database once the characters are seeded and
// before the Olympiad's records are restored from it: noble status in
// characters.nobless, records in olympiad_nobles, the cycle in server_memo,
// heroes in heroes. The fixture restores the records and the heroes only; it never starts the Olympiad
// calendar, whose announcements would otherwise reach a test whenever the
// clock crosses a competition window.
func WithOlympiadSeed(seed func(db *sql.DB)) Option {
	return func(o *options) { o.seedOlympiad = seed }
}

// SetPlayerOlympiadMode puts the online player objID into an Olympiad
// match on side 1 of stadium 0, or takes it out, the precondition of the
// Olympiad gates no packet reaches until the matches run.
func (s *Server) SetPlayerOlympiadMode(tb testing.TB, objID int32, on bool) {
	tb.Helper()
	c := s.onlineCharacter(tb, objID)
	if on {
		c.SetOlympiadGameID(0)
		c.SetOlympiadSide(1)
	} else {
		c.SetOlympiadGameID(-1)
		c.SetOlympiadSide(-1)
	}
	c.SetOlympiadMode(on)
}
