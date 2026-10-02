package gameservertest

import "database/sql"

// WithOlympiadSeed adjusts the database once the characters are seeded and
// before the Olympiad's records are restored from it: noble status in
// characters.nobless, records in olympiad_nobles, the cycle in server_memo.
// The fixture restores the records only; it never starts the Olympiad
// calendar, whose announcements would otherwise reach a test whenever the
// clock crosses a competition window.
func WithOlympiadSeed(seed func(db *sql.DB)) Option {
	return func(o *options) { o.seedOlympiad = seed }
}
