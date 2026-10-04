package task

import (
	"time"

	"github.com/rs/zerolog"

	"github.com/fatal10110/acis_golang/internal/commons/scheduler"
)

// DoorRegenPeriod is how often a damaged door regenerates: a hundred times
// the 3-second creature regeneration period.
const DoorRegenPeriod = 100 * NPCRegenTick

// DoorRegenEffects applies one door's regeneration tick that fell due at due.
type DoorRegenEffects interface {
	RegenDoor(id int, due time.Time)
}

type doorRegenDue struct {
	id  int
	due time.Time
}

// DoorRegen schedules each damaged door's fixed-rate regeneration: a door
// starts regenerating one period after its HP first drops below its
// maximum, then every period after that until it is full again or broken.
//
// All methods are safe for concurrent use.
type DoorRegen struct {
	now func() time.Time

	*deadlineRegistry[int, doorRegenDue]
}

// NewDoorRegen returns an empty door regeneration schedule timed on now.
func NewDoorRegen(now func() time.Time) *DoorRegen {
	if now == nil {
		now = time.Now
	}
	return &DoorRegen{now: now, deadlineRegistry: newDeadlineRegistry[int, doorRegenDue]()}
}

// Start launches the one-second sweep that runs every due door tick through
// effects.
func (r *DoorRegen) Start(effects DoorRegenEffects, log zerolog.Logger) *scheduler.Ticker {
	return scheduler.Start(DoorTick, func() { r.Tick(effects) }, log)
}

// Begin starts id's regeneration, first due one period from now, unless it
// is already regenerating.
func (r *DoorRegen) Begin(id int) {
	due := r.now().Add(DoorRegenPeriod)
	r.addIfAbsent(id, doorRegenDue{id: id, due: due}, due)
}

// Next schedules id's tick that follows the one due at due, keeping the
// fixed rate whenever the sweep ran it.
func (r *DoorRegen) Next(id int, due time.Time) {
	next := due.Add(DoorRegenPeriod)
	r.add(id, doorRegenDue{id: id, due: next}, next)
}

// Cancel stops id's regeneration and reports whether it was regenerating.
func (r *DoorRegen) Cancel(id int) bool {
	return r.remove(id)
}

// Tracked reports whether id has a regeneration tick pending.
func (r *DoorRegen) Tracked(id int) bool {
	return r.tracked(id)
}

// Due reports whether id's pending tick is still the one due at due, so an
// effect can drop a tick that Cancel and a fresh Begin replaced while it
// was in flight.
func (r *DoorRegen) Due(id int, due time.Time) bool {
	return r.hasDeadline(id, due)
}

// Tick hands every due door tick to effects. A tick stays scheduled while
// it runs, so a Begin for the same door meanwhile keeps it instead of
// replacing it; effects confirms it with Due and then either calls Next to
// keep the door regenerating or Cancel to stop it.
func (r *DoorRegen) Tick(effects DoorRegenEffects) {
	r.sweepDueConcurrent(r.now(), func(d doorRegenDue, due time.Time) { effects.RegenDoor(d.id, due) })
}
