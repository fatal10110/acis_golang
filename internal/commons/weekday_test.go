package commons

import (
	"testing"
	"time"
)

func TestWeekdayAtUsesMondayFirstWeeks(t *testing.T) {
	loc := time.UTC
	at := func(y int, m time.Month, d, h int) time.Time { return time.Date(y, m, d, h, 0, 0, 0, loc) }
	cases := []struct {
		name string
		from time.Time
		days int
		wd   time.Weekday
		want time.Time
	}{
		{"saturday from sunday is the day before", at(2026, time.March, 8, 12), 0, time.Saturday, at(2026, time.March, 7, 19)},
		{"saturday from monday is later that week", at(2026, time.March, 9, 12), 0, time.Saturday, at(2026, time.March, 14, 19)},
		{"sunday from monday ends that week", at(2026, time.March, 9, 12), 0, time.Sunday, at(2026, time.March, 15, 19)},
		{"tuesday six days past monday is the week before", at(2026, time.March, 2, 12), 6, time.Tuesday, at(2026, time.March, 3, 19)},
		{"tuesday six days past tuesday is the next week", at(2026, time.March, 3, 12), 6, time.Tuesday, at(2026, time.March, 10, 19)},
		{"across the year", at(2027, time.January, 3, 12), 0, time.Monday, at(2026, time.December, 28, 19)},
	}
	for _, c := range cases {
		if got := WeekdayAt(c.from, c.days, c.wd, 19); !got.Equal(c.want) {
			t.Errorf("%s: WeekdayAt(%s, %d, %s) = %s, want %s", c.name, c.from, c.days, c.wd, got, c.want)
		}
	}
}

func TestWeekdayAtKeepsMilliseconds(t *testing.T) {
	from := time.Date(2026, time.March, 4, 12, 0, 0, 250_999_999, time.UTC)
	want := time.Date(2026, time.March, 7, 19, 0, 0, 250_000_000, time.UTC)
	if got := WeekdayAt(from, 0, time.Saturday, 19); !got.Equal(want) {
		t.Errorf("WeekdayAt = %s, want %s", got, want)
	}
}
