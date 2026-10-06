package gameservertest

import (
	"testing"
	"time"

	"github.com/fatal10110/acis_golang/internal/gameserver/network"
	"github.com/fatal10110/acis_golang/internal/gameserver/script"
	"github.com/fatal10110/acis_golang/internal/gameserver/sim"
)

// WithScheduledTasks runs the scheduled tasks of the WithScripts registry,
// as the production boot does once everything is restored, on an inline
// clock reading start that only Server.ScheduleClock moves; the clock's
// time zone is start's. Without it no task runs.
func WithScheduledTasks(start time.Time) Option {
	return func(o *options) { o.scheduleStart = start }
}

// startSchedule starts the runner WithScheduledTasks asks for against gcl,
// returning its clock; nil without it.
func startSchedule(t *testing.T, o *options, scripts *script.Registry, gcl *network.GameClientLink) *sim.Inline {
	t.Helper()
	if o.scheduleStart.IsZero() {
		return nil
	}
	clock := sim.NewInline(o.scheduleStart)
	sch := script.StartSchedule(scripts, clock.NewQueue("script-schedule"), gcl)
	t.Cleanup(sch.Stop)
	clock.Run()
	return clock
}
