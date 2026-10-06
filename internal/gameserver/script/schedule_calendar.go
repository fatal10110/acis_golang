package script

import (
	"fmt"
	"strconv"
	"strings"
	"time"
)

// scheduleKind is a scripts.xml schedule attribute the runner builds.
type scheduleKind uint8

// The schedule kinds in live use.
const (
	scheduleHourly scheduleKind = iota + 1
	scheduleDaily
	scheduleWeekly
	scheduleMonthlyWeek
)

// unbuiltKinds are the schedule attributes the reference also accepts but no
// listed task uses; they are refused like any unknown attribute, with their
// own message.
var unbuiltKinds = map[string]bool{"MONTHLY_DAY": true, "YEARLY_DAY": true, "YEARLY_WEEK": true}

// schedule is a parsed schedule and start attribute: the kind and the
// calendar fields its start stamp sets.
type schedule struct {
	kind scheduleKind
	// weekday is the WEEKLY and MONTHLY_WEEK day; week the MONTHLY_WEEK
	// week of the month.
	weekday time.Weekday
	week    int
	// hour is unused by HOURLY, whose stamp is minute:second.
	hour, minute, second int
}

// weekRules are the calendar locale's week rules: the first day of a week
// and the fewest days of a month's first week. WEEKLY and MONTHLY_WEEK
// resolve their day through them.
type weekRules struct {
	first   time.Weekday
	minDays int
}

// usWeek is the en-US locale's week, the one the runner keeps: weeks start
// on Sunday and a month's first week may hold a single day of it.
var usWeek = weekRules{first: time.Sunday, minDays: 1}

// parseSchedule parses an entry's schedule attribute kind and start stamp.
// Day names other than MON-SUN read as Monday, as the reference reads them;
// unknownDay reports such a name. Fields past the ones a kind reads are
// ignored.
func parseSchedule(kind, start string) (sc schedule, unknownDay string, err error) {
	misfit := fmt.Errorf("start %q does not fit schedule %s", start, kind)
	switch kind {
	case "HOURLY":
		// "mm:ss"
		ms, ok := stampInts(start, ":", 2)
		if !ok {
			return schedule{}, "", misfit
		}
		return schedule{kind: scheduleHourly, minute: ms[0], second: ms[1]}, "", nil
	case "DAILY":
		// "hh:mm:ss"
		sc = schedule{kind: scheduleDaily}
		if !sc.setTime(start) {
			return schedule{}, "", misfit
		}
		return sc, "", nil
	case "WEEKLY":
		// "DAY hh:mm:ss"
		params := strings.Split(start, " ")
		sc = schedule{kind: scheduleWeekly}
		if len(params) < 2 || !sc.setTime(params[1]) {
			return schedule{}, "", misfit
		}
		sc.weekday, unknownDay = weekday(params[0])
		return sc, unknownDay, nil
	case "MONTHLY_WEEK":
		// "DAY-n hh:mm:ss"
		params := strings.Split(start, " ")
		if len(params) < 2 {
			return schedule{}, "", misfit
		}
		date := strings.Split(params[0], "-")
		sc = schedule{kind: scheduleMonthlyWeek}
		if len(date) < 2 || !sc.setTime(params[1]) {
			return schedule{}, "", misfit
		}
		week, err := strconv.Atoi(date[1])
		if err != nil {
			return schedule{}, "", misfit
		}
		sc.weekday, unknownDay = weekday(date[0])
		sc.week = week
		return sc, unknownDay, nil
	}
	if unbuiltKinds[kind] {
		return schedule{}, "", fmt.Errorf("schedule %s is not supported: no listed task uses it", kind)
	}
	return schedule{}, "", fmt.Errorf("unknown schedule %q", kind)
}

// setTime reads an "hh:mm:ss" stamp into sc, reporting whether it fits.
func (sc *schedule) setTime(stamp string) bool {
	hms, ok := stampInts(stamp, ":", 3)
	if !ok {
		return false
	}
	sc.hour, sc.minute, sc.second = hms[0], hms[1], hms[2]
	return true
}

// stampInts splits s on sep and parses its first n fields as integers,
// reporting false when s has fewer fields or one of them is not a number.
func stampInts(s, sep string, n int) ([]int, bool) {
	fields := strings.Split(s, sep)
	if len(fields) < n {
		return nil, false
	}
	out := make([]int, n)
	for i := range out {
		v, err := strconv.Atoi(fields[i])
		if err != nil {
			return nil, false
		}
		out[i] = v
	}
	return out, true
}

var weekdays = map[string]time.Weekday{
	"SUN": time.Sunday, "MON": time.Monday, "TUE": time.Tuesday, "WED": time.Wednesday,
	"THU": time.Thursday, "FRI": time.Friday, "SAT": time.Saturday,
}

// weekday reads a day name; an unknown one is Monday, and is returned as
// unknown.
func weekday(name string) (time.Weekday, string) {
	if d, ok := weekdays[name]; ok {
		return d, ""
	}
	return time.Monday, name
}

// first returns the first start at or after now, on now's clock and in its
// time zone. The start stamp is laid on now's date (and, for HOURLY, its
// hour; for WEEKLY, its week; for MONTHLY_WEEK, its month), and moved on by
// one period when that is already before now. A start at exactly now
// stays.
func (sc schedule) first(now time.Time, wr weekRules) time.Time {
	loc := now.Location()
	y, mo, d := now.Date()
	var st time.Time
	switch sc.kind {
	case scheduleHourly:
		st = wallTime(y, mo, d, now.Hour(), sc.minute, sc.second, loc)
	case scheduleDaily:
		st = wallTime(y, mo, d, sc.hour, sc.minute, sc.second, loc)
	case scheduleWeekly:
		day := weekOfMonthDay(y, mo, weekOfMonth(y, mo, d, wr), sc.weekday, wr)
		st = wallTime(day.Year(), day.Month(), day.Day(), sc.hour, sc.minute, sc.second, loc)
	case scheduleMonthlyWeek:
		day := weekOfMonthDay(y, mo, sc.week, sc.weekday, wr)
		st = wallTime(day.Year(), day.Month(), day.Day(), sc.hour, sc.minute, sc.second, loc)
	}
	if st.UnixMilli() < now.UnixMilli() {
		st = sc.next(st)
	}
	return st
}

// next returns the start one period after st: an hour later on the clock;
// a day or a week later at the same wall time of day; a calendar month
// later on the same day of the month (the month's last day when it is
// shorter) at the same wall time of day. A MONTHLY_WEEK schedule therefore
// keeps the day of the month its first start fell on, rather than its
// weekday.
func (sc schedule) next(st time.Time) time.Time {
	switch sc.kind {
	case scheduleHourly:
		return st.Add(time.Hour)
	case scheduleDaily:
		return addDays(st, 1)
	case scheduleWeekly:
		return addDays(st, 7)
	default:
		return addMonth(st)
	}
}

// civil is the calendar date y-mo-d, as a UTC midnight.
func civil(y int, mo time.Month, d int) time.Time { return time.Date(y, mo, d, 0, 0, 0, 0, time.UTC) }

// weekOfMonth is the week of its month that y-mo-d falls in under wr: the
// month's first week holds its 1st when at least wr.minDays of that week
// are in the month, else it is week 0.
func weekOfMonth(y int, mo time.Month, d int, wr weekRules) int {
	days := int(civil(y, mo, d).Sub(firstWeekStart(y, mo, wr)) / (24 * time.Hour))
	if days >= 0 {
		return days/7 + 1
	}
	return (days-6)/7 + 1
}

// weekOfMonthDay is the date with weekday dow in week week of y-mo under
// wr. It may fall in the month before or after.
func weekOfMonthDay(y int, mo time.Month, week int, dow time.Weekday, wr weekRules) time.Time {
	start := firstWeekStart(y, mo, wr)
	return start.AddDate(0, 0, (int(dow)-int(wr.first)+7)%7+7*(week-1))
}

// firstWeekStart is the first day of week 1 of y-mo under wr.
func firstWeekStart(y int, mo time.Month, wr weekRules) time.Time {
	day1 := civil(y, mo, 1)
	start := day1.AddDate(0, 0, (int(wr.first)-int(day1.Weekday())+7)%7)
	if int(start.Sub(day1)/(24*time.Hour)) >= wr.minDays {
		start = start.AddDate(0, 0, -7)
	}
	return start
}

// wallTime is the instant the wall time y-mo-d h:mi:s reads in loc, the
// fields normalized when out of range. A wall time a clock change skips is
// read with the offset before the change, so it lands that much later; one
// a clock change repeats is read with the offset after it, the later of the
// two.
func wallTime(y int, mo time.Month, d, h, mi, s int, loc *time.Location) time.Time {
	wall := time.Date(y, mo, d, h, mi, s, 0, time.UTC).Unix()
	return time.Unix(wall-int64(wallOffset(wall, loc)), 0).In(loc)
}

// wallOffset is the UTC offset, in seconds, of the last zone period of loc
// that begins, on its own wall clock, at or before wall.
func wallOffset(wall int64, loc *time.Location) int {
	// Two days back is before any period that can start at wall.
	t := time.Unix(wall-2*24*3600, 0).In(loc)
	_, off := t.Zone()
	for {
		_, end := t.ZoneBounds()
		if end.IsZero() {
			return off
		}
		_, nextOff := end.Zone()
		if end.Unix()+int64(nextOff) > wall {
			return off
		}
		t, off = end, nextOff
	}
}

// addDays returns t n days later at the same wall time of day. Across a
// clock change the time of day is kept unless keeping it would move the
// date, which happens only when the target time is skipped; the time then
// lands after the change, and later days keep that later time of day.
func addDays(t time.Time, n int) time.Time {
	loc := t.Location()
	y, mo, d := t.Date()
	h, mi, s := t.Clock()
	target := civil(y, mo, d).AddDate(0, 0, n)
	_, oldOff := t.Zone()
	wall := time.Date(target.Year(), target.Month(), target.Day(), h, mi, s, 0, time.UTC).Unix()
	cand := time.Unix(wall-int64(oldOff), 0).In(loc)
	_, newOff := cand.Zone()
	if newOff == oldOff {
		return cand
	}
	moved := cand.Add(time.Duration(oldOff-newOff) * time.Second)
	if my, mm, md := moved.Date(); my == target.Year() && mm == target.Month() && md == target.Day() {
		return moved
	}
	return cand
}

// addMonth returns t one calendar month later on the same day of the
// month, or the new month's last day when it is shorter, at the same wall
// time of day.
func addMonth(t time.Time) time.Time {
	y, mo, d := t.Date()
	h, mi, s := t.Clock()
	ny, nmo, _ := civil(y, mo+1, 1).Date()
	d = min(d, civil(ny, nmo+1, 0).Day())
	return wallTime(ny, nmo, d, h, mi, s, t.Location())
}
