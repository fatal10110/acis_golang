package gameservertest

import (
	"testing"
	"time"
)

// AddPlayerLevel moves the live player's level by delta on its own queue,
// as a level change from play does, and waits for it.
func (s *Server) AddPlayerLevel(tb testing.TB, objID int32, delta int) {
	tb.Helper()
	c := s.onlineCharacter(tb, objID)
	done := make(chan struct{})
	if !c.Queue().Post(func() {
		defer close(done)
		c.AddLevel(s.levelTable, c.Template(), delta)
	}) {
		tb.Fatalf("player %d queue closed", objID)
	}
	select {
	case <-done:
	case <-time.After(shutdownDrainTimeout):
		tb.Fatalf("player %d level change did not run", objID)
	}
}
