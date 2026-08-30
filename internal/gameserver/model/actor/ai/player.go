package ai

import (
	"errors"
	"sync"

	"github.com/rs/zerolog"

	"github.com/fatal10110/acis_golang/internal/gameserver/model/actor/attackable"
)

// PlayerAttackActor is the actor state used by the player physical-attack
// intention loop.
type PlayerAttackActor interface {
	attackable.Combatant
	AttackDisabled() bool
	CastingNow() bool
	Knows(attackable.Combatant) bool
	PhysicalAttackRange() int
	Standing() bool
}

// PlayerAttack drives one player's physical-attack intention: closing
// distance on a target and re-attacking it until it dies, is lost, or the
// player cancels.
//
// mu serializes the whole decision in thinkLocked, not just the target
// field: Start runs on the packet-handling goroutine while Think can also
// run concurrently from a movement-arrived or attack-finished hook on a
// timer goroutine. Locking only the target read would let two goroutines
// both observe AttackingNow()==false and both reach DoAttack — a logic race
// on the compound decision that -race can't see, since each individual
// field access would still be individually synchronized.
type PlayerAttack struct {
	actor  PlayerAttackActor
	move   MoveController
	attack AttackController
	log    zerolog.Logger

	mu       sync.Mutex
	target   attackable.Combatant
	deferred bool
}

// NewPlayerAttack builds an idle player attack intention loop.
func NewPlayerAttack(actor PlayerAttackActor, move MoveController, attack AttackController) *PlayerAttack {
	return &PlayerAttack{actor: actor, move: move, attack: attack}
}

// SetLogger records where a broadcast error surfaced from a movement-arrived
// or attack-finished hook (with no caller left to return it to) is logged.
// The zero value discards it.
func (p *PlayerAttack) SetLogger(log zerolog.Logger) {
	p.mu.Lock()
	defer p.mu.Unlock()
	p.log = log
}

// Start sets target as the attack intention and evaluates it once. It
// reports false when the caller should report the action as failed
// (the actor is disabled, sitting, the target is lost, the actor is still
// mid-swing, or the attack was otherwise rejected) and true when the attack
// was accepted — either a swing just started, or the actor has begun
// closing distance and will attack once it arrives.
func (p *PlayerAttack) Start(target attackable.Combatant) bool {
	p.mu.Lock()
	defer p.mu.Unlock()
	p.target = target
	if p.actor.CastingNow() {
		p.deferred = true
		return false
	}
	accepted, err := p.thinkLocked()
	if err != nil {
		p.log.Warn().Err(err).Msg("ai: player attack broadcast")
	}
	return accepted
}

// ResumeAfterCast runs an attack intention that was requested while casting.
// It reports whether such an intention was waiting.
func (p *PlayerAttack) ResumeAfterCast() bool {
	p.mu.Lock()
	defer p.mu.Unlock()
	if !p.deferred {
		return false
	}
	p.deferred = false
	if _, err := p.thinkLocked(); err != nil {
		p.log.Warn().Err(err).Msg("ai: player attack broadcast")
	}
	return true
}

// Stop clears the attack intention and stops any movement toward it.
func (p *PlayerAttack) Stop() {
	p.mu.Lock()
	defer p.mu.Unlock()
	p.stopLocked()
}

// Target returns the current attack target, or nil if idle.
func (p *PlayerAttack) Target() attackable.Combatant {
	p.mu.Lock()
	defer p.mu.Unlock()
	return p.target
}

// Think re-evaluates the current attack intention once. Safe to call from
// a movement-arrived or attack-finished hook as well as from Start. Any
// broadcast error is logged through SetLogger — Think's own callers are
// void hooks with no return path of their own.
func (p *PlayerAttack) Think() {
	p.mu.Lock()
	defer p.mu.Unlock()
	if _, err := p.thinkLocked(); err != nil {
		p.log.Warn().Err(err).Msg("ai: player attack broadcast")
	}
}

// thinkLocked runs the full attack-intention decision. Callers hold mu for
// its entire body so a concurrent Start/Think can't interleave with it and
// reach DoAttack twice for the same swing.
func (p *PlayerAttack) thinkLocked() (bool, error) {
	if p.target == nil {
		return false, nil
	}
	if p.actor.CastingNow() {
		return false, nil
	}

	if p.actor.AttackDisabled() || !p.actor.Standing() || p.targetLost(p.target) {
		p.stopLocked()
		return false, nil
	}

	following, err := p.move.MaybeStartOffensiveFollow(p.target, p.actor.PhysicalAttackRange())
	if following {
		return true, err
	}

	if p.attack.BowCoolingDown() || p.attack.AttackingNow() {
		return false, nil
	}

	if !p.attack.CanAttack(p.target) {
		p.stopLocked()
		return false, nil
	}

	stopErr := p.move.Stop()
	attackErr := p.attack.DoAttack(p.target)
	return true, errors.Join(stopErr, attackErr)
}

func (p *PlayerAttack) stopLocked() {
	p.target = nil
	p.deferred = false
	if err := p.move.Stop(); err != nil {
		p.log.Warn().Err(err).Msg("ai: player attack broadcast")
	}
}

func (p *PlayerAttack) targetLost(target attackable.Combatant) bool {
	if target == nil {
		return true
	}
	if target.AlikeDead() {
		return true
	}
	return !p.actor.Knows(target)
}
