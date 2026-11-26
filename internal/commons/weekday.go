package commons

import "time"

// DayMs and WeekMs are a day and a week in epoch milliseconds, the unit
// the residence calendars store their instants in.
const (
	DayMs  = int64(24 * time.Hour / time.Millisecond)
	WeekMs = 7 * DayMs
)

// WeekdayAt returns hour:00 on weekday wd of the Monday-to-Sunday week
// holding the date days after t's, in t's location, keeping t's
// milliseconds. Weeks start on Monday (#5345), so for a Sunday date the
// result lies in the six days before it.
func WeekdayAt(t time.Time, days int, wd time.Weekday, hour int) time.Time {
	y, m, d := t.Date()
	day := time.Date(y, m, d+days, 12, 0, 0, 0, t.Location())
	ms := t.Nanosecond() / int(time.Millisecond) * int(time.Millisecond)
	return time.Date(y, m, d+days+mondayIndex(wd)-mondayIndex(day.Weekday()), hour, 0, 0, ms, t.Location())
}

// mondayIndex numbers wd from Monday (0) to Sunday (6).
func mondayIndex(wd time.Weekday) int {
	return (int(wd) + 6) % 7
}
