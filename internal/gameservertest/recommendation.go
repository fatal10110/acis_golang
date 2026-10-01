package gameservertest

import (
	"context"
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
	done := make(chan error, 1)
	go func() { done <- s.refreshRecommendations(context.Background()) }()
	return func() {
		tb.Helper()
		if err := <-done; err != nil {
			tb.Fatalf("refresh daily recommendations: %v", err)
		}
		s.Settle(tb)
	}
}
