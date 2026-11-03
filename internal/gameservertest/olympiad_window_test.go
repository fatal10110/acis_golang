package gameservertest

import (
	"testing"
	"time"
	_ "time/tzdata" // the sweep's zones, whatever the host has installed

	"github.com/fatal10110/acis_golang/internal/gameserver/olympiad"
)

// competitionEndAt is the competition window running at now under cfg, by
// the calendar rule: the day's window rebuilt from its start hour and
// minute on now's date, yesterday's taken as that minus a fixed day.
func competitionEndAt(now time.Time, cfg olympiad.Config) (time.Time, bool) {
	y, m, d := now.Date()
	today := time.Date(y, m, d, cfg.StartHour, cfg.StartMinute, 0, 0, now.Location())
	length := time.Duration(min(cfg.CompetitionMillis, 86390000)) * time.Millisecond
	for _, start := range []time.Time{today, today.Add(-24 * time.Hour)} {
		if !now.Before(start) && now.Before(start.Add(length)) {
			return start.Add(length), true
		}
	}
	return time.Time{}, false
}

// TestOlympiadCompetitionWindowClosesWhenAsked places the competition
// window at every minute around midnight and around the daylight saving
// changes of zones shifting by an hour and by half an hour, and checks the
// calendar finds it running and closing remaining after now.
func TestOlympiadCompetitionWindowClosesWhenAsked(t *testing.T) {
	sweeps := []struct {
		zone string
		from time.Time
	}{
		{"UTC", time.Date(2026, 10, 20, 21, 0, 0, 0, time.UTC)},
		{"Asia/Jerusalem", time.Date(2026, 10, 24, 18, 0, 0, 0, time.UTC)},
		{"Asia/Jerusalem", time.Date(2026, 3, 26, 18, 0, 0, 0, time.UTC)},
		{"America/New_York", time.Date(2026, 11, 1, 2, 0, 0, 0, time.UTC)},
		{"Australia/Lord_Howe", time.Date(2026, 4, 4, 10, 0, 0, 0, time.UTC)},
	}
	for _, s := range sweeps {
		loc, err := time.LoadLocation(s.zone)
		if err != nil {
			t.Fatalf("load %s: %v", s.zone, err)
		}
		for _, remaining := range []time.Duration{5 * time.Minute, time.Hour, 90 * time.Minute} {
			w := &olympiadWindow{competition: true, remaining: remaining}
			for at := s.from; at.Before(s.from.Add(10 * time.Hour)); at = at.Add(time.Minute + 7*time.Second) {
				now := at.In(loc)
				end, ok := competitionEndAt(now, w.config(now))
				if want := now.Add(remaining); !ok || want.Sub(end) < 0 || want.Sub(end) >= time.Millisecond {
					t.Fatalf("%s at %s, remaining %s: competition %v ending %s, want ending %s", s.zone, now, remaining, ok, end, want)
				}
			}
		}
	}
}
