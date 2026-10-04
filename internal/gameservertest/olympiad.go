package gameservertest

import (
	"database/sql"
	"testing"
	"time"

	"github.com/fatal10110/acis_golang/internal/gameserver/olympiad"
)

// HeroMinMatches is the number of Olympiad matches a noble needs to be
// elected hero in a booted server, the reference default.
const HeroMinMatches = 5

// WithOlympiadSeed adjusts the database once the characters are seeded and
// before the Olympiad's records are restored from it: noble status in
// characters.nobless, records in olympiad_nobles, the cycle in server_memo,
// heroes in heroes. The fixture restores the records and the heroes only; it
// starts the Olympiad calendar only for WithOlympiadCompetition or
// WithOlympiadValidation, as its
// announcements would otherwise reach a test whenever the clock crosses a
// competition window.
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

// olympiadWindow is the Olympiad period a booted server starts in.
type olympiadWindow struct {
	competition bool
	remaining   time.Duration
}

// WithOlympiadCompetition starts the Olympiad calendar at boot, before any
// player is online, inside a competition window that closes between
// remaining minus a minute and remaining later. remaining stays under two
// hours.
func WithOlympiadCompetition(remaining time.Duration) Option {
	return func(o *options) { o.olympiadWindow = &olympiadWindow{competition: true, remaining: remaining} }
}

// WithOlympiadValidation starts the Olympiad calendar at boot in a
// validation period, the next competition window two hours away.
func WithOlympiadValidation() Option {
	return func(o *options) { o.olympiadWindow = &olympiadWindow{} }
}

// config is the Olympiad configuration putting now in w: the default one,
// its daily window moved to open two hours after now, for an hour, for a
// validation period, or to have opened so that a two-hour window closes
// w.remaining after now, to the minute, for a competition. A nil w keeps
// the default.
func (w *olympiadWindow) config(now time.Time) olympiad.Config {
	cfg := olympiad.DefaultConfig()
	if w == nil {
		return cfg
	}
	length := time.Hour
	start := now.Add(2 * time.Hour)
	if w.competition {
		length = 2 * time.Hour
		start = now.Add(w.remaining - length)
	}
	cfg.StartHour, cfg.StartMinute = start.Hour(), start.Minute()
	cfg.CompetitionMillis = length.Milliseconds()
	return cfg
}
