package clanhall

import (
	"testing"
	"time"
)

// Reference: SiegableHall.updateNextSiege takes now, adds
// scheduleConfig's days, then months, then years (Calendar.add, pinning the
// day to the end of a shorter month), sets the hour of day and the minute,
// second 0, and keeps now's millisecond.
func TestNextSiegeDate(t *testing.T) {
	utc := time.UTC
	for _, tc := range []struct {
		name     string
		now      time.Time
		schedule []int
		want     time.Time
	}{
		{
			name:     "shipped schedule",
			now:      time.Date(2026, 10, 19, 15, 4, 5, 123_456_789, utc),
			schedule: []int{14, 0, 0, 12, 0},
			want:     time.Date(2026, 11, 2, 12, 0, 0, 123_000_000, utc),
		},
		{
			name:     "month pinned to its last day",
			now:      time.Date(2027, 1, 31, 8, 0, 0, 0, utc),
			schedule: []int{0, 1, 0, 20, 30},
			want:     time.Date(2027, 2, 28, 20, 30, 0, 0, utc),
		},
		{
			name:     "days then months",
			now:      time.Date(2027, 1, 20, 8, 0, 0, 0, utc),
			schedule: []int{14, 1, 0, 12, 0},
			want:     time.Date(2027, 3, 3, 12, 0, 0, 0, utc),
		},
		{
			name:     "leap day a year on",
			now:      time.Date(2028, 2, 29, 8, 0, 0, 0, utc),
			schedule: []int{0, 0, 1, 12, 0},
			want:     time.Date(2029, 2, 28, 12, 0, 0, 0, utc),
		},
		{
			name:     "missing values count as 0",
			now:      time.Date(2026, 10, 19, 15, 4, 5, 0, utc),
			schedule: []int{1},
			want:     time.Date(2026, 10, 20, 0, 0, 0, 0, utc),
		},
	} {
		t.Run(tc.name, func(t *testing.T) {
			if got := NextSiegeDate(tc.now, tc.schedule); !got.Equal(tc.want) {
				t.Fatalf("NextSiegeDate(%v, %v) = %v, want %v", tc.now, tc.schedule, got, tc.want)
			}
		})
	}
}
