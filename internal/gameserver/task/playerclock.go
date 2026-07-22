package task

import (
	"errors"
	"sync"

	"github.com/fatal10110/acis_golang/internal/gameserver/world"
)

// shadowSenseSkillID is the racial Dark Elf passive whose side effect is
// re-applied (and announced) each time the in-game day/night boundary
// crosses.
const shadowSenseSkillID = 294

// shadowSenseLevel is the level applied on a boundary crossing. Shadow Sense
// is only ever acquired at level 1, so this is also the level the
// system-message parameter reports.
const shadowSenseLevel = 1

// activityReminderMinutes is how many in-game minutes pass between two
// "playing for a long time" reminders for the same player: two real hours
// of play is 720 in-game minutes (2h * 24 * 60 / 4).
const activityReminderMinutes = 720

// PlayerClockActor is the narrow player surface PlayerClock inspects and
// mutates when a day/night boundary crosses.
type PlayerClockActor interface {
	ObjectID() int32
	HasSkill(skillID int) bool
	SetSkillLevel(skillID, level int)
}

// PlayerClockEffects routes PlayerClock's per-player frames back through
// the connection's write path. Method names describe the event the player
// is told about, not the wire shape; the concrete SystemMessage ids and
// frame layouts live in the serverpackets package.
type PlayerClockEffects interface {
	// NotifyPlayingTooLong sends the periodic "playing for a long time"
	// reminder (once every two real hours of activity) to the player
	// identified by actorID. A missing or offline session is a no-op.
	NotifyPlayingTooLong(actorID int32)

	// NotifyDayNightSkillTransition sends the "skill S1 effect applies at
	// night" or "skill S1 effect disappears by day" SystemMessage, naming
	// skillID at level. night is true when night has just fallen. A
	// missing or offline session is a no-op.
	NotifyDayNightSkillTransition(actorID int32, night bool, skillID, level int32)
}

// PlayerClock drives the two player-facing periodic side effects of the
// in-game clock:
//
//   - On each day/night boundary cross, every online player holding the
//     Shadow Sense skill has it re-applied at its canonical level and is
//     told, by SystemMessage, that the skill's effect has appeared (night)
//     or disappeared (day).
//   - Once per game minute, every tracked player whose next activity
//     reminder has come due is sent PLAYING_FOR_LONG_TIME and rescheduled
//     for the next interval.
//
// The day/night reapply rides on GameClock.OnDayNight, so it fires on the
// same Tick that detected the boundary crossing; the activity reminder has
// its own per-minute clock listener, calling GameClock.Minutes for the
// monotonic counter that rescheduling is anchored to.
//
// mu guards activity.
type PlayerClock struct {
	clock   *GameClock
	players *world.State
	effects PlayerClockEffects

	mu       sync.Mutex
	activity map[int32]int // actor id -> absolute game-minute at which the next reminder falls due
}

// NewPlayerClock returns a PlayerClock wired to clock and state. clock is
// used both as the day/night source (NewPlayerClock registers an
// OnDayNight listener on it) and as the monotonic counter the activity
// reminder ticks against. state yields the player set walked on each
// boundary crossing. effects routes the resulting per-player frames.
func NewPlayerClock(clock *GameClock, state *world.State, effects PlayerClockEffects) (*PlayerClock, error) {
	if clock == nil {
		return nil, errors.New("task: game clock is nil")
	}
	if effects == nil {
		return nil, errors.New("task: player clock effects is nil")
	}
	pc := &PlayerClock{
		clock:    clock,
		players:  state,
		effects:  effects,
		activity: make(map[int32]int),
	}
	clock.OnDayNight(pc.onDayNight)
	clock.OnMinute(pc.Tick)
	return pc, nil
}

// Add registers actor for the activity reminder; the first reminder is
// scheduled activityReminderMinutes out from the current in-game minute.
// Add called again for an already-tracked actor resets its schedule.
func (p *PlayerClock) Add(actor PlayerClockActor) {
	if actor == nil {
		return
	}
	p.mu.Lock()
	defer p.mu.Unlock()
	p.activity[actor.ObjectID()] = p.clock.Minutes() + activityReminderMinutes
}

// Remove stops tracking actorID for the activity reminder; safe to call
// for an unknown id.
func (p *PlayerClock) Remove(actorID int32) {
	p.mu.Lock()
	defer p.mu.Unlock()
	delete(p.activity, actorID)
}

// Tick fires PLAYING_FOR_LONG_TIME for every tracked player whose reminder
// has come due and reschedules them for the next interval. Iteration is
// over the activity map alone; the day/night reapply (onDayNight) walks
// online world players instead.
func (p *PlayerClock) Tick() {
	current := p.clock.Minutes()
	p.mu.Lock()
	due := make([]int32, 0, len(p.activity))
	for id, dueAt := range p.activity {
		if current < dueAt {
			continue
		}
		due = append(due, id)
		p.activity[id] = current + activityReminderMinutes
	}
	p.mu.Unlock()

	for _, id := range due {
		p.effects.NotifyPlayingTooLong(id)
	}
}

// onDayNight re-applies the Shadow Sense skill to every online player who
// holds it, and announces the effect transition. It runs on the GameClock's
// Tick goroutine, so each effect send must be non-blocking; the per-player
// session write path satisfies that.
func (p *PlayerClock) onDayNight(night bool) {
	if p.players == nil {
		return
	}
	for _, obj := range p.players.Players() {
		actor, ok := obj.(PlayerClockActor)
		if !ok {
			continue
		}
		if !actor.HasSkill(shadowSenseSkillID) {
			continue
		}
		actor.SetSkillLevel(shadowSenseSkillID, shadowSenseLevel)
		p.effects.NotifyDayNightSkillTransition(actor.ObjectID(), night, shadowSenseSkillID, shadowSenseLevel)
	}
}
