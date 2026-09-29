package ai

import (
	"sync"

	"github.com/rs/zerolog"

	"github.com/fatal10110/acis_golang/internal/gameserver/model/actor/attackable"
)

// PlayerAttackActor is the actor state used by the player physical-attack
// intention loop.
type PlayerAttackActor interface {
	attackable.Combatant
	CastingNow() bool
	DenyAIAction() bool
	Knows(attackable.Combatant) bool
	PhysicalAttackRange() int
	Standing() bool
	// RefuseAttackTarget tells the player an attack target was refused
	// (TARGET_IS_INCORRECT).
	RefuseAttackTarget()
}

// PlayerAttack drives one player's physical-attack intention: closing
// distance on a target and re-attacking it until it dies, is lost, or the
// player cancels.
//
// mu serializes the whole decision in thinkLocked, not just the target
// field. Start, Think and the movement/attack hooks run on the player's
// queue, but an effect another actor lands (AbortAll from a stun) stops this
// intention from that actor's queue (ActionsStopRequested → tryToIdle →
// Stop). Locking only the target read would let two goroutines both observe
// AttackingNow()==false and both reach DoAttack — a logic race on the
// compound decision that -race can't see, since each individual field
// access would still be individually synchronized.
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
//
// A target the playable attack gate refuses is reported to the player and
// leaves the current intention untouched. The gate runs only when the
// request is neither denied nor deferred behind a swing or cast, the order
// the deny and busy checks take ahead of it.
func (p *PlayerAttack) Start(target attackable.Combatant) bool {
	p.mu.Lock()
	defer p.mu.Unlock()
	if p.gateRefusesLocked(target) {
		p.actor.RefuseAttackTarget()
		return false
	}
	p.target = target
	if p.actor.CastingNow() {
		p.deferred = true
		return false
	}
	accepted, _, err := p.thinkLocked()
	if err != nil {
		p.log.Warn().Err(err).Msg("ai: player attack broadcast")
	}
	return accepted
}

// RefuseTarget runs the playable attack gate Start runs for target, without
// changing any intention. When the gate refuses, it tells the player and
// reports true: the caller answers ActionFailed and keeps every intention it
// holds, the way a refused attack leaves the current one in place. Callers
// that drop other intentions for a new attack check this first.
func (p *PlayerAttack) RefuseTarget(target attackable.Combatant) bool {
	p.mu.Lock()
	defer p.mu.Unlock()
	if !p.gateRefusesLocked(target) {
		return false
	}
	p.actor.RefuseAttackTarget()
	return true
}

// gateRefusesLocked reports whether the playable attack gate refuses target.
// A denied request, or one deferred behind a cast or swing, is not gated
// yet: the deny and busy checks come first.
func (p *PlayerAttack) gateRefusesLocked(target attackable.Combatant) bool {
	return !p.actor.DenyAIAction() && !p.actor.CastingNow() && !p.attack.AttackingNow() && refusesPlayableTarget(p.actor, target)
}

// ResumeAfterCast runs an attack intention that was requested while casting.
// It reports whether such an intention was waiting, and whether running it
// sent the intention idle, which the caller answers with ActionFailed.
func (p *PlayerAttack) ResumeAfterCast() (resumed, idled bool) {
	p.mu.Lock()
	defer p.mu.Unlock()
	if !p.deferred {
		return false, false
	}
	p.deferred = false
	_, idled, err := p.thinkLocked()
	if err != nil {
		p.log.Warn().Err(err).Msg("ai: player attack broadcast")
	}
	return true, idled
}

// DropResumeAfterCast forgets an attack requested while casting, without
// touching the attack intention itself: a later request queued behind the
// same cast has replaced it as the next intention.
func (p *PlayerAttack) DropResumeAfterCast() {
	p.mu.Lock()
	defer p.mu.Unlock()
	p.deferred = false
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
// a movement-arrived or attack-finished hook as well as from Start. It
// reports whether the intention went idle (the actor can't act, is sitting,
// lost the target, or can't attack it once in range), which the caller
// answers with ActionFailed; a busy actor keeps the intention silently. Any
// broadcast error is logged through SetLogger — Think's own callers are
// void hooks with no return path of their own.
func (p *PlayerAttack) Think() (idled bool) {
	p.mu.Lock()
	defer p.mu.Unlock()
	_, idled, err := p.thinkLocked()
	if err != nil {
		p.log.Warn().Err(err).Msg("ai: player attack broadcast")
	}
	return idled
}

// thinkLocked runs the full attack-intention decision and reports whether a
// swing started or the approach began (accepted) and whether the intention
// was dropped (idled). Callers hold mu for its entire body so a concurrent
// Start/Think can't interleave with it and reach DoAttack twice for the same
// swing.
//
// The first gate is the AI-action one, not the attack one: a flying or
// fake-dead actor still closes distance first and only fails the attack
// once in range, through CanAttack.
func (p *PlayerAttack) thinkLocked() (accepted, idled bool, err error) {
	if p.target == nil {
		return false, false, nil
	}
	if p.actor.CastingNow() {
		return false, false, nil
	}

	if p.actor.DenyAIAction() || !p.actor.Standing() || p.targetLost(p.target) {
		p.stopLocked()
		return false, true, nil
	}

	following, err := p.move.MaybeStartOffensiveFollow(p.target, p.actor.PhysicalAttackRange())
	if following {
		return true, false, err
	}

	if p.attack.BowCoolingDown() || p.attack.AttackingNow() {
		return false, false, nil
	}

	if !p.attack.CanAttack(p.target) {
		p.stopLocked()
		return false, true, nil
	}

	p.move.Stop()
	p.attack.DoAttack(p.target)
	return true, false, nil
}

func (p *PlayerAttack) stopLocked() {
	p.target = nil
	p.deferred = false
	p.move.Stop()
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
