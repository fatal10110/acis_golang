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
