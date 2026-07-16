package ai

import (
	"sync"

	"github.com/fatal10110/acis_golang/internal/gameserver/model/actor/attackable"
)

// PlayerAttackActor is the actor state used by the player physical-attack
// intention loop.
type PlayerAttackActor interface {
	attackable.Combatant
	AttackDisabled() bool
	Knows(attackable.Combatant) bool
	PhysicalAttackRange() int
}

// PlayerAttack drives one player's physical-attack intention: closing
// distance on a target and re-attacking it until it dies, is lost, or the
// player cancels.
//
// mu guards target: Start and Stop run on the packet-handling goroutine
// while Think can also run from a movement-arrived or attack-finished hook
// on a timer goroutine.
type PlayerAttack struct {
	actor  PlayerAttackActor
	move   MoveController
	attack AttackController

	mu     sync.Mutex
	target attackable.Combatant
}

// NewPlayerAttack builds an idle player attack intention loop.
func NewPlayerAttack(actor PlayerAttackActor, move MoveController, attack AttackController) *PlayerAttack {
	return &PlayerAttack{actor: actor, move: move, attack: attack}
}

// Start sets target as the attack intention and evaluates it once. It
// reports false when the caller should report the action as failed
// (the actor is disabled, the target is lost, the actor is still mid-swing,
// or the attack was otherwise rejected) and true when the attack was
// accepted — either a swing just started, or the actor has begun closing
// distance and will attack once it arrives.
func (p *PlayerAttack) Start(target attackable.Combatant) bool {
	p.mu.Lock()
	p.target = target
	p.mu.Unlock()
	return p.think()
}

// Stop clears the attack intention and stops any movement toward it.
func (p *PlayerAttack) Stop() {
	p.mu.Lock()
	p.target = nil
	p.mu.Unlock()
	p.move.Stop()
}

// Target returns the current attack target, or nil if idle.
func (p *PlayerAttack) Target() attackable.Combatant {
	p.mu.Lock()
	defer p.mu.Unlock()
	return p.target
}

// Think re-evaluates the current attack intention once. Safe to call from
// a movement-arrived or attack-finished hook as well as from Start.
func (p *PlayerAttack) Think() {
	p.think()
}

func (p *PlayerAttack) think() bool {
	p.mu.Lock()
	target := p.target
	p.mu.Unlock()
	if target == nil {
		return false
	}

	if p.actor.AttackDisabled() || p.targetLost(target) {
		p.Stop()
		return false
	}

	if p.move.MaybeStartOffensiveFollow(target, p.actor.PhysicalAttackRange()) {
		return true
	}

	if p.attack.BowCoolingDown() || p.attack.AttackingNow() {
		return false
	}

	if !p.attack.CanAttack(target) {
		p.Stop()
		return false
	}

	p.move.Stop()
	p.attack.DoAttack(target)
	return true
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
