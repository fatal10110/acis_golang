package gameservertest

import "testing"

// TickGameMinutes advances the in-game clock by n minutes, one tick at a
// time, so the per-minute and day/night listeners run as the production
// ticker would drive them. Boot does not start the game-minute ticker.
func (s *Server) TickGameMinutes(tb testing.TB, n int) {
	tb.Helper()
	for range n {
		s.gameClock.Tick()
	}
}
