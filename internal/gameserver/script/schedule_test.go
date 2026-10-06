package script

import (
	"bytes"
	"context"
	"strconv"
	"strings"
	"testing"
	"time"
	_ "time/tzdata" // the calendar golden's zones, wherever the test runs

	"github.com/fatal10110/acis_golang/internal/gameserver/sim"
	"github.com/fatal10110/acis_golang/internal/testsupport/scriptcontract"
	"github.com/rs/zerolog"
)

// localeWeeks are the week rules of the locales the calendar golden uses.
var localeWeeks = map[string]weekRules{
	"en-US": usWeek,
	"de-DE": {first: time.Monday, minDays: 4},
}

// TestScheduleCalendar replays the reference's schedule calendar: for each
// row, the first start set at now, then the three starts each firing moves
// on to, for the four schedule kinds in live use across zones, clock
// changes, month and year ends and both week rules.
func TestScheduleCalendar(t *testing.T) {
	scriptcontract.Run(t, "schedule.calendar", func(t *testing.T, r scriptcontract.Row) {
		loc, err := time.LoadLocation(r.Str(t, "zone"))
		if err != nil {
			t.Fatal(err)
		}
		wr, ok := localeWeeks[r.Str(t, "locale")]
		if !ok {
			t.Fatalf("no week rules for locale %s", r.Str(t, "locale"))
		}
		now, err := time.ParseInLocation("2006-01-02T15:04:05", r.Str(t, "now"), loc)
		if err != nil {
			t.Fatal(err)
		}
		sc, unknownDay, err := parseSchedule(r.Str(t, "type"), r.Str(t, "start"))
		if ok := err == nil && unknownDay == ""; ok != r.Bool(t, "ok") {
			t.Fatalf("parse ok = %v (err %v, day %q), reference %v", ok, err, unknownDay, r.Bool(t, "ok"))
		}
		st := sc.first(now, wr)
		for i, key := range []string{"next0", "next1", "next2", "next3"} {
			if i > 0 {
				st = sc.next(st)
			}
			if got, want := st.Format(time.RFC3339), r.Str(t, key); got != want {
				t.Errorf("%s = %s, reference %s", key, got, want)
			}
		}
	})
}

// TestScheduleRescan replays the reference's look-ahead: a start is armed
// only when it falls strictly within the coming period, after the delay
// left until it, at once when it is past.
func TestScheduleRescan(t *testing.T) {
	scriptcontract.Run(t, "schedule.rescan", func(t *testing.T, r scriptcontract.Row) {
		at := time.UnixMilli(r.Int(t, "at"))
		next := time.UnixMilli(r.Int(t, "next"))
		d, ok := dueWithin(at, next)
		got := "none"
		if ok {
			got = strconv.FormatInt(d.Milliseconds(), 10)
		}
		if want := r.Str(t, "delay"); got != want {
			t.Errorf("delay = %s, reference %s", got, want)
		}
	})
}

// TestParseScheduleRefusals: kinds the runner does not build and stamps
// that do not fit their kind are errors; an unknown day reads as Monday.
func TestParseScheduleRefusals(t *testing.T) {
	for _, tc := range []struct{ kind, start, errPart string }{
		{"MONTHLY_DAY", "1 6:20:10", "not supported"},
		{"YEARLY_DAY", "23-02 6:20:10", "not supported"},
		{"YEARLY_WEEK", "MON-1 6:20:10", "not supported"},
		{"EVERY_MINUTE", "00", "unknown schedule"},
		{"DAILY", "16:20", "does not fit"},
		{"WEEKLY", "TUE", "does not fit"},
		{"HOURLY", "x:00", "does not fit"},
	} {
		if _, _, err := parseSchedule(tc.kind, tc.start); err == nil || !strings.Contains(err.Error(), tc.errPart) {
			t.Errorf("parseSchedule(%s, %q) error = %v, want one saying %q", tc.kind, tc.start, err, tc.errPart)
		}
	}
	sc, day, err := parseSchedule("WEEKLY", "TUESDAY 16:55:00")
	if err != nil || day != "TUESDAY" || sc.weekday != time.Monday {
		t.Fatalf("unknown day: %+v %q %v, want Monday and the day reported", sc, day, err)
	}
}

// recordingServer is a Server the start hooks of the tests below never
// call; they record their own firings.
type recordingServer struct{ Server }

// firings returns a catalog entry whose start hook appends the clock's
// time to *at, panicking on its first firing when panicFirst is set.
func firings(in *sim.Inline, at *[]time.Time, panicFirst bool) func() Script {
	return func() Script {
		return Script{Hooks: Hooks{OnStart: func(_ *Script, e Start) {
			if e.Server == nil || e.Ctx == nil {
				panic("start without server or context")
			}
			*at = append(*at, in.Now())
			if panicFirst && len(*at) == 1 {
				panic("first start fails")
			}
		}}}
	}
}

func buildScheduled(t *testing.T, list []Listing, catalog Catalog) (*Registry, *bytes.Buffer) {
	t.Helper()
	var logs bytes.Buffer
	r := Build(list, catalog, Config{KindOf: allTemplates, Log: zerolog.New(&logs)})
	return r, &logs
}

// TestScheduleRunsTasksAtTheirStarts runs a daily task on a driven clock: it
// fires at exactly each start, a start already past at boot moves to the
// next day, and a start hook that panics does not stop the next start.
func TestScheduleRunsTasksAtTheirStarts(t *testing.T) {
	boot := time.Date(2026, 10, 5, 23, 50, 0, 0, time.UTC)
	in := sim.NewInline(boot)
	var at []time.Time
	r, _ := buildScheduled(t,
		[]Listing{{Path: "task.Daily", Schedule: "DAILY", Start: "00:00:00"}},
		Catalog{"task.Daily": firings(in, &at, true)})
	sch := StartSchedule(r, in.NewQueue("schedule"), recordingServer{})
	defer sch.Stop()

	in.Advance(48 * time.Hour)
	want := []time.Time{
		time.Date(2026, 10, 6, 0, 0, 0, 0, time.UTC),
		time.Date(2026, 10, 7, 0, 0, 0, 0, time.UTC),
	}
	if !equalTimes(at, want) {
		t.Fatalf("starts at %v, want %v", at, want)
	}
}

// TestScheduleRescanArmsTasksAhead: a start more than one period away is
// armed by the look that finds it within the coming period, and fires on
// time; until then nothing fires.
func TestScheduleRescanArmsTasksAhead(t *testing.T) {
	boot := time.Date(2026, 10, 5, 12, 53, 0, 0, time.UTC)
	in := sim.NewInline(boot)
	var at []time.Time
	r, _ := buildScheduled(t,
		[]Listing{{Path: "task.Hourly", Schedule: "HOURLY", Start: "00:00"}},
		Catalog{"task.Hourly": firings(in, &at, false)})
	sch := StartSchedule(r, in.NewQueue("schedule"), recordingServer{})

	// 13:00 is 7 minutes away: the look at boot leaves it, the one at
	// 12:58 arms it.
	in.Advance(5*time.Minute - time.Second)
	if next, ok := in.NextTimer(); !ok || !next.Equal(boot.Add(rescanPeriod)) {
		t.Fatalf("before the second look the next timer is %v (%v), want only the look at %v", next, ok, boot.Add(rescanPeriod))
	}
	in.Advance(3 * time.Hour)
	want := []time.Time{
		time.Date(2026, 10, 5, 13, 0, 0, 0, time.UTC),
		time.Date(2026, 10, 5, 14, 0, 0, 0, time.UTC),
		time.Date(2026, 10, 5, 15, 0, 0, 0, time.UTC),
	}
	if !equalTimes(at, want) {
		t.Fatalf("starts at %v, want %v", at, want)
	}

	sch.Stop()
	in.Advance(2 * time.Hour)
	if len(at) != len(want) {
		t.Fatalf("a stopped schedule fired again: %v", at)
	}
}

// TestScheduleOnlySchedulesTasks: a schedule on a script with no start hook
// is ignored, as on the reference's plain quests; a task entry with a
// schedule the runner does not build, a bad stamp, no start or an end of
// its own is logged and not scheduled; a task entry without a schedule is
// registered and never scheduled.
func TestScheduleOnlySchedulesTasks(t *testing.T) {
	in := sim.NewInline(time.Date(2026, 10, 5, 23, 59, 0, 0, time.UTC))
	var at []time.Time
	task := firings(in, &at, false)
	quest := func() Script {
		return Script{QuestID: 620, Hooks: Hooks{OnTalk: func(*Script, Talk) string { return "" }}}
	}
	list := []Listing{
		{Path: "quest.Q620_FourGoblets", Schedule: "DAILY", Start: "00:00:00", End: "06:00:00"},
		{Path: "task.Monthly", Schedule: "MONTHLY_DAY", Start: "1 00:00:00"},
		{Path: "task.Unknown", Schedule: "MINUTELY", Start: "00"},
		{Path: "task.BadStamp", Schedule: "DAILY", Start: "00:00"},
		{Path: "task.NoStart", Schedule: "DAILY"},
		{Path: "task.WithEnd", Schedule: "DAILY", Start: "00:00:00", End: "01:00:00"},
		{Path: "task.SameEnd", Schedule: "DAILY", Start: "00:00:00", End: "00:00:00"},
		{Path: "task.Unscheduled"},
	}
	catalog := Catalog{"quest.Q620_FourGoblets": quest}
	for _, l := range list[1:] {
		catalog[l.Path] = task
	}
	r, logs := buildScheduled(t, list, catalog)

	var dump strings.Builder
	if err := r.Dump(&dump); err != nil {
		t.Fatal(err)
	}
	if got := strings.Count(dump.String(), "  scheduled\n"); got != 1 {
		t.Errorf("dump schedules %d entries, want only task.SameEnd:\n%s", got, dump.String())
	}
	if !strings.Contains(dump.String(), "script task.SameEnd\n  kind scheduled\n  hooks onStart\n  scheduled\n") {
		t.Errorf("task.SameEnd dump:\n%s", dump.String())
	}
	if !strings.Contains(dump.String(), "script quest.Q620_FourGoblets\n  kind quest\n  hooks onTalk\nscript") {
		t.Errorf("the quest's schedule is not ignored:\n%s", dump.String())
	}
	for _, path := range []string{"task.Monthly", "task.Unknown", "task.BadStamp", "task.NoStart", "task.WithEnd"} {
		if !strings.Contains(logs.String(), `"script":"`+path+`"`) {
			t.Errorf("%s is not logged; logs:\n%s", path, logs.String())
		}
	}
	for _, path := range []string{"quest.Q620_FourGoblets", "task.Unscheduled", "task.SameEnd"} {
		if strings.Contains(logs.String(), `"script":"`+path+`"`) {
			t.Errorf("%s is logged; logs:\n%s", path, logs.String())
		}
	}

	sch := StartSchedule(r, in.NewQueue("schedule"), recordingServer{})
	defer sch.Stop()
	in.Advance(time.Hour)
	if want := []time.Time{time.Date(2026, 10, 6, 0, 0, 0, 0, time.UTC)}; !equalTimes(at, want) {
		t.Fatalf("starts at %v, want only task.SameEnd's %v", at, want)
	}
}

// TestScheduleStopCancelsStartContext: a start hook still running when the
// schedule stops sees its context cancelled.
func TestScheduleStopCancelsStartContext(t *testing.T) {
	in := sim.NewInline(time.Date(2026, 10, 5, 23, 59, 0, 0, time.UTC))
	var sch *Schedule
	var ctxErr error
	r, _ := buildScheduled(t,
		[]Listing{{Path: "task.Daily", Schedule: "DAILY", Start: "00:00:00"}},
		Catalog{"task.Daily": func() Script {
			return Script{Hooks: Hooks{OnStart: func(_ *Script, e Start) {
				sch.Stop()
				ctxErr = e.Ctx.Err()
			}}}
		}})
	sch = StartSchedule(r, in.NewQueue("schedule"), recordingServer{})
	in.Advance(time.Hour)
	if ctxErr != context.Canceled {
		t.Fatalf("start context after Stop: %v, want canceled", ctxErr)
	}
}

func equalTimes(a, b []time.Time) bool {
	if len(a) != len(b) {
		return false
	}
	for i := range a {
		if !a[i].Equal(b[i]) {
			return false
		}
	}
	return true
}
