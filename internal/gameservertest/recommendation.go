package gameservertest

import (
	"context"
	"sync"
	"testing"
)

// RefreshDailyRecommendations runs the daily recommendation refresh the
// scheduled job runs, then waits until every online player's queue has
// applied its part.
func (s *Server) RefreshDailyRecommendations(tb testing.TB) {
	tb.Helper()
	if err := s.refreshRecommendations(context.Background()); err != nil {
		tb.Fatalf("refresh daily recommendations: %v", err)
	}
	s.Settle(tb)
}

// StartDailyRecommendationRefresh starts the daily recommendation refresh
// on its own goroutine, for a suite that holds a persistence lane the
// refresh has to wait for. The returned wait blocks until the refresh
// returns, fails the test on its error, and then settles like
// RefreshDailyRecommendations.
func (s *Server) StartDailyRecommendationRefresh(tb testing.TB) (wait func()) {
	tb.Helper()
	resume := s.holdEveryLane()
	done := make(chan error, 1)
	go func() {
		err := s.refreshRecommendations(context.Background())
		resume()
		done <- err
	}()
	return func() {
		tb.Helper()
		if err := <-done; err != nil {
			tb.Fatalf("refresh daily recommendations: %v", err)
		}
		s.Settle(tb)
	}
}

// ExpectEveryLanePaused is for a suite that keeps a scheduled start's
// offloaded work waiting (on a held lane or a locked table) while it pauses
// every persistence lane (the daily recommendation refresh): until resume is
// called (also on cleanup), the catch-up before a client read flushes no
// lane, since none can drain, and the read does not wait out
// catchUpTimeout. Call resume once that work has returned, which Settle
// waits for.
func (s *Server) ExpectEveryLanePaused(tb testing.TB) (resume func()) {
	tb.Helper()
	resume = s.holdEveryLane()
	cleanup(tb, resume)
	return resume
}

// holdEveryLane counts every persistence lane as held until the returned
// release is called: a refresh pausing every lane until a held one reaches
// the pause leaves no lane free for a catch-up to flush, and without this
// every client read would wait out catchUpTimeout.
func (s *Server) holdEveryLane() (release func()) {
	for i := range s.heldLanes {
		s.heldLanes[i].Add(1)
	}
	var once sync.Once
	return func() {
		once.Do(func() {
			for i := range s.heldLanes {
				s.heldLanes[i].Add(-1)
			}
		})
	}
}
