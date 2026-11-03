package zone

import (
	"sync/atomic"
	"time"
)

// Trap ties a zone to a castle defense device. A castle-linked trap is
// dormant unless it has been armed and its castle's siege is running; a
// trap without a castle link is always live.
type Trap struct {
	// CastleID links the trap to a castle; 0 means unlinked (always live).
	CastleID int
	// EventID is the client map effect the trap shows while armed; 0
	// means none. A player whose surroundings stop including a region the
	// trap touches has the effect turned off, armed or not; one whose
	// surroundings come to include it has it turned on only while armed.
	EventID int
	// Armed reports whether the castle owner armed the trap. Nothing arms
	// a castle trap: a flame tower never arms its zones, whatever the
	// castle's trap upgrades, so a castle-linked trap stays dormant.
	// Unsynchronized: dormant() may run on any actor goroutine that
	// crosses the zone, and on the damage pulse's queue; a writer added
	// later must switch this to atomic.Bool (or equivalent) first.
	Armed bool
	// SiegeActive reports whether the linked castle's siege is running;
	// nil reads as not running. The game server wires it at boot.
	SiegeActive func() bool
}

// TrapOf returns k's castle trap, nil when k is no trap zone.
func TrapOf(k Kind) *Trap {
	switch z := k.(type) {
	case *Damage:
		return &z.Trap
	case *Swamp:
		return &z.Trap
	}
	return nil
}

// dormant reports whether the trap's effect is currently suppressed.
func (t *Trap) dormant() bool {
	if t.CastleID <= 0 {
		return false
	}
	return !t.Armed || t.SiegeActive == nil || !t.SiegeActive()
}

// Damage is a trap that periodically hurts the playables standing inside
// it, and marks players as being in danger.
type Damage struct {
	Zone
	Trap
	// HPDamage is the raw damage per pulse.
	HPDamage int
	// InitialDelay and ReuseDelay time the damage pulse.
	InitialDelay time.Duration
	ReuseDelay   time.Duration

	// StartPulse begins the periodic damage task; nil leaves the zone
	// harmless. The zone fires it at most once until PulseStopped resets
	// the latch. Hook implementations must tolerate overlapping calls when
	// a task resets the latch before its previous StartPulse invocation
	// returns.
	StartPulse func()

	pulsing atomic.Bool
}

// NewDamage builds an unlinked damage trap dealing 200 HP every 5 seconds
// after a 1 second delay; the loader overrides these from the zone's data
// settings before publishing it.
func NewDamage(id int, form Form) *Damage {
	return &Damage{
		Zone:         newZone(id, form),
		HPDamage:     200,
		InitialDelay: time.Second,
		ReuseDelay:   5 * time.Second,
	}
}

// Core exposes the shared zone state.
func (z *Damage) Core() *Zone { return &z.Zone }

func (z *Damage) affects(a Actor) bool { return a.Class().Playable() }

func (z *Damage) enter(a Actor) {
	if z.HPDamage > 0 {
		// A dormant castle trap does nothing at all - not even the danger
		// marker below.
		if z.dormant() {
			return
		}
		if z.pulsing.CompareAndSwap(false, true) && z.StartPulse != nil {
			z.StartPulse()
		}
	}
	enterDanger(a)
}

func (z *Damage) exit(a Actor) { exitDanger(a) }

// Endangered is a player that shows whether it stands in a damage or
// effect zone. Each occupant carries its own reaction, so no zone holds
// per-server state.
type Endangered interface {
	Actor
	// DangerStateChanged runs after a danger zone raised, or released the
	// last, hold on the actor's FlagDanger, to refresh its status display.
	DangerStateChanged()
}

// enterDanger marks a player as in danger; every entry refreshes its
// display, even when an overlapping danger zone already holds it.
func enterDanger(a Actor) {
	if a.Class() != ClassPlayer {
		return
	}
	a.ZoneFlags().Set(FlagDanger, true)
	if e, ok := a.(Endangered); ok {
		e.DangerStateChanged()
	}
}

// exitDanger releases one danger hold on a player, refreshing its display
// only once the last overlapping danger zone let go.
func exitDanger(a Actor) {
	if a.Class() != ClassPlayer {
		return
	}
	a.ZoneFlags().Set(FlagDanger, false)
	if a.ZoneFlags().Has(FlagDanger) {
		return
	}
	if e, ok := a.(Endangered); ok {
		e.DangerStateChanged()
	}
}

// Live reports whether the damage pulse keeps running: the zone deals
// damage and is not a dormant castle trap. The damage task stops once this
// is false.
func (z *Damage) Live() bool { return z.HPDamage > 0 && !z.dormant() }

// PulseStopped resets the pulse latch; the damage task calls it when it
// shuts itself down, so the next entry can start a fresh pulse. It reports
// true, holding the latch again, when a live zone still has occupants: one
// entered after the task's last look, and its entry found the latch still
// held, so the caller must start a fresh pulse for it.
func (z *Damage) PulseStopped() bool {
	z.pulsing.Store(false)
	return z.Live() && len(z.Occupants()) > 0 && z.pulsing.CompareAndSwap(false, true)
}

// Swamp is a trap that slows down everyone wading through it.
type Swamp struct {
	Zone
	Trap
	// MoveBonus is the movement speed adjustment applied inside, in
	// percent (negative slows).
	MoveBonus int
}

// Wader is an actor that reacts to entering or leaving a swamp.
type Wader interface {
	Actor
	// SwampStateChanged runs after the actor's swamp flag changed at z,
	// to apply or lift the slow and rebroadcast the actor's appearance.
	SwampStateChanged(z *Swamp)
}

// NewSwamp builds an unlinked swamp trap slowing movement by 50 percent;
// the loader overrides these from the zone's data settings before
// publishing it.
func NewSwamp(id int, form Form) *Swamp {
	return &Swamp{Zone: newZone(id, form), MoveBonus: -50}
}

// Core exposes the shared zone state.
func (z *Swamp) Core() *Zone { return &z.Zone }

func (z *Swamp) affects(Actor) bool { return true }

func (z *Swamp) enter(a Actor) {
	if z.dormant() {
		return
	}
	a.ZoneFlags().Set(FlagSwamp, true)
	if w, ok := a.(Wader); ok {
		w.SwampStateChanged(z)
	}
}

func (z *Swamp) exit(a Actor) {
	a.ZoneFlags().Set(FlagSwamp, false)
	if w, ok := a.(Wader); ok {
		w.SwampStateChanged(z)
	}
}
