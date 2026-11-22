package creature

import (
	"sync/atomic"
	"time"

	"github.com/fatal10110/acis_golang/internal/gameserver/model/actor/move"
	"github.com/fatal10110/acis_golang/internal/gameserver/model/location"
	"github.com/fatal10110/acis_golang/internal/gameserver/sim"
	"github.com/fatal10110/acis_golang/internal/gameserver/skill/effect"
)

// Live owns per-creature runtime state shared by live actor wrappers: its
// lifetime movement state, its active buffs/debuffs, and the crowd-control
// status flags both the effect list and transient action locks feed into.
type Live struct {
	movement move.CreatureMove
	// effects is built on its first use and published once by
	// compare-and-swap, whichever goroutine needs it first (a buff lands
	// from the caster's queue); nil until then, as a creature no effect
	// ever reached holds none. effectOwner and effectOpts build it.
	effects     atomic.Pointer[effect.List]
	effectOwner effect.StatOwner
	effectOpts  []effect.Option
	// queue runs this creature's work; set once before the creature is
	// published into the world.
	queue *sim.Queue

	// Status flags are independent of each other and settable from any
	// goroutine; no reader needs two of them to change together.
	paralyzed   atomic.Bool
	immobilized SourceSet
	invul       SourceSet
	teleporting atomic.Bool
}

// NewLive creates runtime state at origin with speed and geodata-bound
// movement validation, configured by opts. owner receives stat-function
// callbacks from this creature's active effects; a nil owner (e.g. an actor
// whose stats aren't driven by a calculator yet) leaves those callbacks
// unapplied. Movement arrivals run on the queue SetQueue installs.
func NewLive(origin location.Location, speed float64, geo move.Geo, owner effect.StatOwner, opts ...Option) (*Live, error) {
	live := &Live{effectOwner: owner}
	var moveOpts []move.Option
	for _, opt := range opts {
		if opt.effects != nil {
			live.effectOpts = opt.effects
		}
		if moveOpts == nil {
			moveOpts = opt.move
		} else {
			// Copy rather than append into a caller's slice.
			moveOpts = append(moveOpts[:len(moveOpts):len(moveOpts)], opt.move...)
		}
	}
	if err := live.movement.Init(origin, speed, geo, live, moveOpts...); err != nil {
		return nil, err
	}
	return live, nil
}

// Option configures a Live when NewLive builds it.
type Option struct {
	effects []effect.Option
	move    []move.Option
}

// WithEffects makes the live state build its effect list with opts on first
// use. The slice is kept, so callers spawning many creatures pass one shared
// slice.
func WithEffects(opts ...effect.Option) Option {
	return Option{effects: opts}
}

// WithMovement configures the creature's movement state with opts.
func WithMovement(opts ...move.Option) Option {
	return Option{move: opts}
}

// SetQueue makes q the queue this creature's work runs on, movement
// arrivals included. Call it once, before the creature is published into
// the world.
func (l *Live) SetQueue(q *sim.Queue) {
	l.queue = q
	if e := l.effects.Load(); e != nil {
		e.SetQueue(q)
	}
}

// Queue returns the queue this creature's work runs on.
func (l *Live) Queue() *sim.Queue {
	if l == nil {
		return nil
	}
	return l.queue
}

// Now reads the clock this creature's queue runs on. Reuse windows and other
// deadlines compared against its timers are stamped and checked with it.
func (l *Live) Now() time.Time { return l.Queue().Now() }

// Move returns this live creature's lifetime movement state.
func (l *Live) Move() *move.CreatureMove {
	return &l.movement
}

// MoveType reports how this creature travels from where it stands; a nil
// Live stands on the ground.
func (l *Live) MoveType() move.MoveType {
	if l == nil {
		return move.MoveGround
	}
	return l.movement.MoveType()
}

// MovingTo returns the target of this creature's leg in flight and whether
// it is walking at all; a nil Live never walks.
func (l *Live) MovingTo() (location.Location, bool) {
	if l == nil {
		return location.Location{}, false
	}
	return l.movement.MovingTo()
}

// ValidLocation resolves a destination against this creature's movement
// geodata without starting an ordinary move.
func (l *Live) ValidLocation(ox, oy, oz, tx, ty, tz int) location.Location {
	if l == nil {
		return location.Location{X: ox, Y: oy, Z: oz}
	}
	return l.movement.ValidLocation(ox, oy, oz, tx, ty, tz)
}

// EffectList returns this creature's active buffs and debuffs, building
// the list first when none exists yet.
func (l *Live) EffectList() *effect.List {
	if l == nil {
		return nil
	}
	if e := l.effects.Load(); e != nil {
		return e
	}
	e := effect.NewList(l.effectOwner, l.effectOpts...)
	e.SetQueue(l.queue)
	l.effects.CompareAndSwap(nil, e)
	return l.effects.Load()
}

// EffectListIfBuilt is EffectList without building one: nil, a list that
// holds nothing, until the first use. Every effect.List read and stop
// takes a nil list.
func (l *Live) EffectListIfBuilt() *effect.List {
	if l == nil {
		return nil
	}
	return l.effects.Load()
}

// Stunned reports whether an active effect currently stuns this creature.
func (l *Live) Stunned() bool {
	if l == nil {
		return false
	}
	return l.effects.Load().IsAffected(effect.FlagStunned)
}

// Rooted reports whether an active effect currently roots this creature in
// place.
func (l *Live) Rooted() bool {
	if l == nil {
		return false
	}
	return l.effects.Load().IsAffected(effect.FlagRooted)
}

// Sleeping reports whether an active effect currently puts this creature to
// sleep.
func (l *Live) Sleeping() bool {
	if l == nil {
		return false
	}
	return l.effects.Load().IsAffected(effect.FlagSleep)
}

// FakeDead reports whether an active effect currently makes this creature
// play dead.
func (l *Live) FakeDead() bool {
	if l == nil {
		return false
	}
	return l.effects.Load().IsAffected(effect.FlagFakeDeath)
}

// SilentMoving reports whether an active effect currently lets this
// creature move without alerting nearby AI.
func (l *Live) SilentMoving() bool {
	if l == nil {
		return false
	}
	return l.effects.Load().IsAffected(effect.FlagSilentMove)
}

// Confused reports whether an active effect currently confuses this
// creature into attacking indiscriminately.
func (l *Live) Confused() bool {
	if l == nil {
		return false
	}
	return l.effects.Load().IsAffected(effect.FlagConfused)
}

// Afraid reports whether an active effect currently fears this creature.
func (l *Live) Afraid() bool {
	if l == nil {
		return false
	}
	return l.effects.Load().IsAffected(effect.FlagFear)
}

// ImmobileUntilAttacked reports whether an active effect currently holds
// this creature in place until it is attacked.
func (l *Live) ImmobileUntilAttacked() bool {
	if l == nil {
		return false
	}
	return l.effects.Load().IsAffected(effect.FlagMeditating)
}

// Paralyzed reports whether this creature is paralyzed, either by a
// transient action lock (SetParalyzed) or by an active effect that carries
// the paralyze flag.
func (l *Live) Paralyzed() bool {
	if l == nil {
		return false
	}
	return l.paralyzed.Load() || l.effects.Load().IsAffected(effect.FlagParalyzed)
}

// SetParalyzed sets or clears this creature's transient paralysis lock and
// reports whether it changed. It does not touch any active paralyze effect
// — Paralyzed() unions both sources.
func (l *Live) SetParalyzed(v bool) bool {
	if l == nil {
		return false
	}
	return l.paralyzed.CompareAndSwap(!v, v)
}

// Invul reports whether this creature is currently invulnerable: any
// invulnerability source is active, or it is teleporting.
func (l *Live) Invul() bool {
	if l == nil {
		return false
	}
	return l.invul.Held() || l.teleporting.Load()
}

// SetInvul sets or clears the direct (command or script) invulnerability
// source and reports whether invulnerability changed. Other sources keep it
// held.
func (l *Live) SetInvul(v bool) bool {
	return l.SetInvulSource(DirectSource{}, v)
}

// SetInvulSource adds or removes src as a source of invulnerability and
// reports whether invulnerability changed: it holds while any source does.
func (l *Live) SetInvulSource(src any, on bool) bool {
	if l == nil {
		return false
	}
	return l.invul.Set(src, on)
}

// Immobilized reports whether any movement-lock source is active.
func (l *Live) Immobilized() bool {
	if l == nil {
		return false
	}
	return l.immobilized.Held()
}

// SetImmobilized sets or clears the direct movement-lock source and reports
// whether the lock changed.
func (l *Live) SetImmobilized(v bool) bool {
	return l.SetImmobilizedSource(DirectSource{}, v)
}

// SetImmobilizedSource adds or removes src as a source of the movement lock
// and reports whether the lock changed: it holds while any source does.
func (l *Live) SetImmobilizedSource(src any, on bool) bool {
	if l == nil {
		return false
	}
	return l.immobilized.Set(src, on)
}

// Teleporting reports whether this creature is in the client-visible
// teleport transition.
func (l *Live) Teleporting() bool {
	if l == nil {
		return false
	}
	return l.teleporting.Load()
}

// AIDeniedBeforeEffect reports whether this creature could not take AI
// actions before the effect whose on-start hook is running landed; see
// effect.List.StartedAffected.
func (l *Live) AIDeniedBeforeEffect() bool {
	if l == nil {
		return false
	}
	return l.paralyzed.Load() || l.teleporting.Load() || l.effects.Load().StartedAffected(effect.AIDenyFlags)
}

// SetTeleporting sets or clears this creature's teleport transition flag
// and reports whether it changed.
func (l *Live) SetTeleporting(v bool) bool {
	if l == nil {
		return false
	}
	return l.teleporting.CompareAndSwap(!v, v)
}
