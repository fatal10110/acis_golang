package creature

import (
	"sync"

	"github.com/fatal10110/acis_golang/internal/gameserver/model/actor/move"
	"github.com/fatal10110/acis_golang/internal/gameserver/model/location"
	"github.com/fatal10110/acis_golang/internal/gameserver/skill/effect"
)

// Live owns per-creature runtime state shared by live actor wrappers: its
// lifetime movement state, its active buffs/debuffs, and the crowd-control
// status flags both the effect list and transient action locks feed into.
type Live struct {
	movement move.CreatureMove
	effects  *effect.List

	// stateMu guards paralyzed and immobilized.
	stateMu     sync.RWMutex
	paralyzed   bool
	immobilized bool
}

// NewLive creates runtime state at origin with speed and geodata-bound
// movement validation. owner receives stat-function callbacks from this
// creature's active effects; a nil owner (e.g. an actor whose stats aren't
// driven by a calculator yet) leaves those callbacks unapplied.
func NewLive(origin location.Location, speed float64, geo move.Geo, owner effect.StatOwner) (*Live, error) {
	live := &Live{effects: effect.NewList(owner)}
	if err := live.movement.Init(origin, speed, geo); err != nil {
		return nil, err
	}
	return live, nil
}

// Move returns this live creature's lifetime movement state.
func (l *Live) Move() *move.CreatureMove {
	return &l.movement
}

// EffectList returns this creature's active buffs and debuffs.
func (l *Live) EffectList() *effect.List {
	if l == nil {
		return nil
	}
	return l.effects
}

// Stunned reports whether an active effect currently stuns this creature.
func (l *Live) Stunned() bool {
	if l == nil {
		return false
	}
	return l.effects.IsAffected(effect.FlagStunned)
}

// Rooted reports whether an active effect currently roots this creature in
// place.
func (l *Live) Rooted() bool {
	if l == nil {
		return false
	}
	return l.effects.IsAffected(effect.FlagRooted)
}

// Sleeping reports whether an active effect currently puts this creature to
// sleep.
func (l *Live) Sleeping() bool {
	if l == nil {
		return false
	}
	return l.effects.IsAffected(effect.FlagSleep)
}

// SilentMoving reports whether an active effect currently lets this
// creature move without alerting nearby AI.
func (l *Live) SilentMoving() bool {
	if l == nil {
		return false
	}
	return l.effects.IsAffected(effect.FlagSilentMove)
}

// Confused reports whether an active effect currently confuses this
// creature into attacking indiscriminately.
func (l *Live) Confused() bool {
	if l == nil {
		return false
	}
	return l.effects.IsAffected(effect.FlagConfused)
}

// Afraid reports whether an active effect currently fears this creature.
func (l *Live) Afraid() bool {
	if l == nil {
		return false
	}
	return l.effects.IsAffected(effect.FlagFear)
}

// Paralyzed reports whether this creature is paralyzed, either by a
// transient action lock (SetParalyzed) or by an active effect that carries
// the paralyze flag.
func (l *Live) Paralyzed() bool {
	if l == nil {
		return false
	}
	l.stateMu.RLock()
	manual := l.paralyzed
	l.stateMu.RUnlock()
	return manual || l.effects.IsAffected(effect.FlagParalyzed)
}

// SetParalyzed sets or clears this creature's transient paralysis lock and
// reports whether it changed. It does not touch any active paralyze effect
// — Paralyzed() unions both sources.
func (l *Live) SetParalyzed(v bool) bool {
	if l == nil {
		return false
	}
	l.stateMu.Lock()
	defer l.stateMu.Unlock()
	if l.paralyzed == v {
		return false
	}
	l.paralyzed = v
	return true
}

// Invul reports whether this creature is currently invulnerable.
func (l *Live) Invul() bool {
	return false
}

// Immobilized reports whether this creature's movement-lock flag is set.
func (l *Live) Immobilized() bool {
	if l == nil {
		return false
	}
	l.stateMu.RLock()
	defer l.stateMu.RUnlock()
	return l.immobilized
}

// SetImmobilized sets or clears this creature's movement-lock flag and
// reports whether it changed.
func (l *Live) SetImmobilized(v bool) bool {
	if l == nil {
		return false
	}
	l.stateMu.Lock()
	defer l.stateMu.Unlock()
	if l.immobilized == v {
		return false
	}
	l.immobilized = v
	return true
}
