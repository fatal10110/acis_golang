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

// PlayerMoveController is the movement a player's attack intention drives.
type PlayerMoveController interface {
	MoveController
	// HoldOffensiveFollow reports whether target is out of attackRange plus
	// both footprints, without walking toward it.
	HoldOffensiveFollow(target attackable.Combatant, attackRange int) bool
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
	move   PlayerMoveController
	attack AttackController
	log    zerolog.Logger

	mu     sync.Mutex
	target attackable.Combatant
	// shift is the intention's shift modifier: an attack held with shift
	// never walks, and goes idle once target is out of reach.
	shift bool
	// deferred marks target as the next intention behind the cast in flight:
	// it was requested mid-cast, is not thought until the cast ends, and
	// ResumeAfterCast runs it then.
	deferred bool
	// queued marks the last think finding the actor busy with a swing, a bow
	// reuse or a cast: the attack stays current and is also the next
	// intention, which the end of a bow shot re-thinks.
	queued bool
}

// NewPlayerAttack builds an idle player attack intention loop.
func NewPlayerAttack(actor PlayerAttackActor, move PlayerMoveController, attack AttackController) *PlayerAttack {
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

// Start sets target as the attack intention, held with shift or not, and
// evaluates it once. It reports false when the caller should report the action
// as failed (the actor is disabled, sitting, the target is lost, the actor is
// still mid-swing or mid-cast, or the attack was otherwise rejected) and true
// when the attack was accepted — either a swing just started, or the actor has
// begun closing distance and will attack once it arrives. A shift-held attack
// on a target out of reach goes idle instead of closing distance, and reports
// false.
//
// A target the playable attack gate refuses is reported to the player and
// leaves the current intention untouched. The gate runs only when the
// request is neither denied nor deferred behind a swing or cast, the order
// the deny and busy checks take ahead of it. A request made mid-cast or
// mid-swing is waited out without being thought: no gate, no approach, until
// the cast or swing ends.
func (p *PlayerAttack) Start(target attackable.Combatant, shift bool) bool {
	return p.start(target, shift, true)
}

// StartIntention runs an attack already accepted as the next intention.
// The playable target gate belongs to the original request, before queuing.
func (p *PlayerAttack) StartIntention(target attackable.Combatant, shift bool) bool {
	return p.start(target, shift, false)
}

func (p *PlayerAttack) start(target attackable.Combatant, shift, request bool) bool {
	p.mu.Lock()
	defer p.mu.Unlock()
	if request && p.gateRefusesLocked(target) {
		p.actor.RefuseAttackTarget()
		return false
	}
	p.target, p.shift = target, shift
	p.deferred, p.queued = false, false
	if p.actor.CastingNow() {
		p.deferred = true
		return false
	}
	if p.attack.AttackingNow() {
		p.queued = true
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
// is answered with ActionFailed.
func (p *PlayerAttack) ResumeAfterCast() (resumed, actionFailed bool) {
	p.mu.Lock()
	defer p.mu.Unlock()
	if !p.deferred {
		return false, false
	}
	p.deferred = false
	_, actionFailed, err := p.thinkLocked()
	if err != nil {
		p.log.Warn().Err(err).Msg("ai: player attack broadcast")
	}
	return true, actionFailed
}

// ReplaceWithCast records a CAST intention taking the attack's place, current
// or queued behind a cast: a skill request that starts now, or one queued
// behind the swing or cast in flight. The attack is dropped: a finished
// nextActionAttack skill attacks its own final target (AttackAfterCast), not
// the one it replaced. Movement is left alone: the cast owns it.
func (p *PlayerAttack) ReplaceWithCast() {
	p.Replace()
}

// AttackAfterCast makes target the attack intention once a nextActionAttack
// skill cast on it ends, or is refused at its cost and condition checks, held
// with the cast's own shift modifier, and thinks it once, reporting whether
// that is answered with ActionFailed. The caller has already checked that
// target may be attacked without force. Like any intention the AI sets itself,
// it skips the playable attack gate a player's own attack request runs.
func (p *PlayerAttack) AttackAfterCast(target attackable.Combatant, shift bool) (actionFailed bool) {
	p.mu.Lock()
	defer p.mu.Unlock()
	p.target, p.shift = target, shift
	p.deferred, p.queued = false, false
	_, actionFailed, err := p.thinkLocked()
	if err != nil {
		p.log.Warn().Err(err).Msg("ai: player attack broadcast")
	}
	return actionFailed
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
// a movement-arrived, attack-finished or cast-finished hook as well as from
// Start. It reports whether the think is answered with ActionFailed: the
// intention went idle (the actor can't act, is sitting, lost the target, or
// can't attack it once in range), or the actor is still busy with a swing, a
// bow reuse or a cast and the attack waits for it. An attack waiting on a
// cast is not current and is not thought. Any broadcast error is logged
// through SetLogger — Think's own callers are void hooks with no return path
// of their own.
func (p *PlayerAttack) Think() (actionFailed bool) {
	p.mu.Lock()
	defer p.mu.Unlock()
	if p.deferred {
		return false
	}
	_, actionFailed, err := p.thinkLocked()
	if err != nil {
		p.log.Warn().Err(err).Msg("ai: player attack broadcast")
	}
	return actionFailed
}

// FinishedAttack re-thinks the attack once a swing ends and reports whether
// that is answered with ActionFailed. An attack queued behind the swing (a
// request made mid-swing, or a think that found the swing in flight) runs
// as the next intention. With nothing queued, the attack goes on only
// against a target the player can keep attacking; against any other it goes
// idle silently, without an ActionFailed. An attack waiting on a cast is not
// current and is not thought.
func (p *PlayerAttack) FinishedAttack() (actionFailed bool) {
	p.mu.Lock()
	defer p.mu.Unlock()
	if p.deferred || p.target == nil {
		return false
	}
	if !p.queued && !canKeepAttacking(p.actor, p.target) {
		p.stopLocked()
		return false
	}
	_, actionFailed, err := p.thinkLocked()
	if err != nil {
		p.log.Warn().Err(err).Msg("ai: player attack broadcast")
	}
	return actionFailed
}

// Replace drops the attack intention for another one that takes its place,
// leaving any walk under way to the new intention.
func (p *PlayerAttack) Replace() {
	p.mu.Lock()
	defer p.mu.Unlock()
	p.target, p.shift = nil, false
	p.deferred, p.queued = false, false
}

// ThinkQueued re-evaluates the attack only when the last think queued it
// behind the actor's swing, bow reuse or cast, as the end of a bow shot
// does; otherwise nothing happens. It reports whether the think is answered
// with ActionFailed.
func (p *PlayerAttack) ThinkQueued() (actionFailed bool) {
	p.mu.Lock()
	defer p.mu.Unlock()
	if !p.queued || p.deferred {
		return false
	}
	_, actionFailed, err := p.thinkLocked()
	if err != nil {
		p.log.Warn().Err(err).Msg("ai: player attack broadcast")
	}
	return actionFailed
}

// thinkLocked runs the full attack-intention decision and reports whether a
// swing started or the approach began (accepted) and whether the think is
// answered with ActionFailed. Callers hold mu for its entire body so a
// concurrent Start/Think can't interleave with it and reach DoAttack twice
// for the same swing.
//
// The first gate is the AI-action one, not the attack one: a flying or
// fake-dead actor still closes distance first and only fails the attack
// once in range, through CanAttack. Only once in range and stopped does a
// swing, bow reuse or cast in flight hold the attack: it stays current,
// becomes the next intention too, and is answered with ActionFailed. A
// shift-held attack out of reach goes idle with ActionFailed, not walking.
func (p *PlayerAttack) thinkLocked() (accepted, actionFailed bool, err error) {
	if p.target == nil {
		return false, false, nil
	}
	p.queued = false

	if p.actor.DenyAIAction() || !p.actor.Standing() || p.targetLost(p.target) {
		p.stopLocked()
		return false, true, nil
	}

	if p.shift {
		// A shift-held attack never walks: out of reach, it goes idle.
		if p.move.HoldOffensiveFollow(p.target, p.actor.PhysicalAttackRange()) {
			p.stopLocked()
			return false, true, nil
		}
	} else {
		following, err := p.move.MaybeStartOffensiveFollow(p.target, p.actor.PhysicalAttackRange())
		if following {
			return true, false, err
		}
	}

	p.move.Stop()

	if p.attack.BowCoolingDown() || p.attack.AttackingNow() || p.actor.CastingNow() {
		p.queued = true
		return false, true, nil
	}

	if !p.attack.CanAttack(p.target) {
		p.stopLocked()
		return false, true, nil
	}

	p.attack.DoAttack(p.target)
	return true, false, nil
}

func (p *PlayerAttack) stopLocked() {
	p.target, p.shift = nil, false
	p.deferred, p.queued = false, false
	p.move.Stop()
}

func (p *PlayerAttack) targetLost(target attackable.Combatant) bool {
	if target == nil {
		return true
	}
	// A fake-dead target is still there to attack; only a dead one is lost.
	if target.Dead() {
		return true
	}
	return !p.actor.Knows(target)
}
