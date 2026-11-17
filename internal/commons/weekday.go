package commons

import "time"

// WeekdayAt returns hour:00 on weekday wd of the Sunday-to-Saturday week
// holding the date days after t's, in t's location, keeping t's
// milliseconds.
func WeekdayAt(t time.Time, days int, wd time.Weekday, hour int) time.Time {
	y, m, d := t.Date()
	day := time.Date(y, m, d+days, 12, 0, 0, 0, t.Location())
	ms := t.Nanosecond() / int(time.Millisecond) * int(time.Millisecond)
	return time.Date(y, m, d+days+int(wd-day.Weekday()), hour, 0, 0, ms, t.Location())
}
