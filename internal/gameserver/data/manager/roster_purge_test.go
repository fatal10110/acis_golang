package manager

import (
	"testing"
	"time"
)

// TestRosterPurgesOnDelete reports an immediate purge for a zero grace period
// (DeleteCharAfterDays = 0) and for a negative one, whose deletion time the
// reference stores already past, and a scheduled deletion otherwise.
func TestRosterPurgesOnDelete(t *testing.T) {
	for _, tc := range []struct {
		after time.Duration
		want  bool
	}{
		{DefaultDeleteAfter, false},
		{24 * time.Hour, false},
		{0, true},
		{-24 * time.Hour, true},
	} {
		r := NewRoster(nil, nil, nil, nil, nil, nil, nil, tc.after, nil)
		if got := r.PurgesOnDelete(); got != tc.want {
			t.Errorf("PurgesOnDelete() with %v = %v, want %v", tc.after, got, tc.want)
		}
	}
}
