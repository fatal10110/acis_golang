package olympiad

import "time"

const (
	dayMillis = int64(24 * time.Hour / time.Millisecond)
	// maxCompetitionMillis caps the daily competition window just short of
	// a whole day.
	maxCompetitionMillis = int64(86390000)
	// registrationCloseMillis is how long before a competition window ends
	// its registration closes.
	registrationCloseMillis = int64(10 * time.Minute / time.Millisecond)
	// olympiadEndHour is the hour of the first day of the month an Olympiad
	// ends at.
	olympiadEndHour = 12
)

// nextOlympiadEnd returns when an Olympiad started at now ends: the first
// day of the following month at noon, local to now.
func nextOlympiadEnd(now time.Time) int64 {
	y, m, _ := now.Date()
	return time.Date(y, m+1, 1, olympiadEndHour, 0, 0, 0, now.Location()).UnixMilli()
}

// nextNoblePointsUpdate returns when the next weekly noble points grant
// falls, computed at now: on the weekday the current month began on, at the
// competition start time, skipping the first day of a month. The weekday of
// the first is taken from now's month, so the grants of one month keep its
// weekday and the next month's grants move to the next month's.
//
// A grant whose day is today but whose hour has passed moves a week later;
// so does any computation made during the competition start hour once its
// minute has passed, whatever the day.
func nextNoblePointsUpdate(now time.Time, cfg Config) int64 {
	y, m, d := now.Date()
	firstDay := time.Date(y, m, 1, olympiadEndHour, 0, 0, 0, now.Location()).Weekday()
	days := int(firstDay) - int(now.Weekday())
	hour, minute := now.Hour(), now.Minute()
	if days < 0 || (days == 0 && hour > cfg.StartHour) || (hour == cfg.StartHour && minute >= cfg.StartMinute) {
		days += 7
	}
	if d == 1 {
		days += 7
	}
	return time.Date(y, m, d+days, cfg.StartHour, cfg.StartMinute, 0, 0, now.Location()).UnixMilli()
}

// periodAt reports the period running at now and when it ends. The
// competition runs daily from the start time for the configured length
// (capped just short of a day), and the validation period fills the rest of
// the day up to the next competition start. A competition that began the
// day before and runs past midnight is still the running one. Neither
// period ends after olympiadEnd.
func periodAt(now time.Time, cfg Config, olympiadEnd int64) (period Period, end int64) {
	nowMs := now.UnixMilli()
	y, m, d := now.Date()
	startToday := time.Date(y, m, d, cfg.StartHour, cfg.StartMinute, 0, 0, now.Location())
	length := min(cfg.CompetitionMillis, maxCompetitionMillis)

	startTodayMs := startToday.UnixMilli()
	startYesterdayMs := startTodayMs - dayMillis
	switch {
	case nowMs >= startTodayMs && nowMs < startTodayMs+length:
		period, end = Competition, startTodayMs+length
	case nowMs >= startYesterdayMs && nowMs < startYesterdayMs+length:
		period, end = Competition, startYesterdayMs+length
	case startTodayMs > nowMs:
		period, end = Validation, startTodayMs
	default:
		period, end = Validation, startToday.AddDate(0, 0, 1).UnixMilli()
	}
	return period, min(end, olympiadEnd)
}
