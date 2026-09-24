package ai

import (
	"sync"
	"time"

	"github.com/rs/zerolog"

	"github.com/fatal10110/acis_golang/internal/gameserver/model/actor/attackable"
	"github.com/fatal10110/acis_golang/internal/gameserver/model/skill"
	"github.com/fatal10110/acis_golang/internal/gameserver/sim"
)

const summonFollowOffset = 70

// summonOffensiveFollowTick is CreatureMove.java's ATTACK_FOLLOW_INTERVAL
// (CreatureMove.java:41): a summon's offensive follow re-evaluates on its
// own 500 ms schedule, independent of the shared 1 s AI think tick that
// otherwise drives Think.
const summonOffensiveFollowTick = 500 * time.Millisecond

// SummonActor is the live summon state needed by the summon AI loop.
type SummonActor interface {
	attackable.Combatant
	DenyAIAction() bool
	Knows(attackable.Combatant) bool
	PhysicalAttackRange() int
	SetHeadingTo(attackable.Combatant)
	BroadcastMoveToPawn(attackable.Combatant)
}

// SummonMoveController controls movement requests emitted by a summon AI.
type SummonMoveController interface {
	MoveController
	MaybeStartFriendlyFollow(target attackable.Combatant, offset int) (bool, error)
}

// Summon drives one pet or servitor's owner-directed intentions.
type Summon struct {
	actor  SummonActor
	move   SummonMoveController
	attack AttackController
	cast   CastController
	log    zerolog.Logger

	// mu guards current and next. A Betray effect turns the summon on its
	// owner (TryToAttack) from the caster's queue.
	mu      sync.Mutex
	current intention
	next    intention
}

// NewSummon builds an idle summon AI loop.
func NewSummon(actor SummonActor, move SummonMoveController, attack AttackController) *Summon {
	return &Summon{
		actor:   actor,
		move:    move,
		attack:  attack,
		current: intention{kind: IntentionIdle},
	}
}

// SetLogger records where a broadcast error surfaced from Think (with no
// caller left to return it to) is logged. The zero value discards it.
func (s *Summon) SetLogger(log zerolog.Logger) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.log = log
}

// SetCastController wires the AI loop's TryToCast handling to controller.
// Left unset (the default), TryToCast is a no-op, matching a summon with no
// commandable special skill.
func (s *Summon) SetCastController(controller CastController) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.cast = controller
}

// CurrentIntention returns the currently active intention kind.
func (s *Summon) CurrentIntention() Intention {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.current.kind
}

// NextIntention returns the queued intention, if one exists.
func (s *Summon) NextIntention() (Intention, attackable.Combatant, bool) {
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.next.kind == IntentionIdle {
		return IntentionIdle, nil, false
	}
	return s.next.kind, s.next.target, true
}

// TryToAttack sets target as the attack intention and evaluates it once.
func (s *Summon) TryToAttack(target attackable.Combatant) bool {
	s.mu.Lock()
	defer s.mu.Unlock()

	if target == nil || s.actor.DenyAIAction() {
		return false
	}
	if s.busyLocked() {
		s.next = intention{kind: IntentionAttack, target: target}
		return true
	}
	s.current = intention{kind: IntentionAttack, target: target}
	accepted, err := s.thinkAttackLocked()
	if err != nil {
		s.log.Warn().Err(err).Msg("ai: summon broadcast")
	}
	return accepted
}

// TryToFollow sets target as the follow intention and evaluates it once.
func (s *Summon) TryToFollow(target attackable.Combatant) bool {
	s.mu.Lock()
	defer s.mu.Unlock()

	if target == nil || sameCombatant(s.actor, target) || s.actor.DenyAIAction() {
		return false
	}
	if s.busyLocked() {
		s.next = intention{kind: IntentionFollow, target: target}
		return true
	}
	s.current = intention{kind: IntentionFollow, target: target}
	accepted, err := s.thinkFollowLocked()
	if err != nil {
		s.log.Warn().Err(err).Msg("ai: summon broadcast")
	}
	return accepted
}

// TryToCast sets target/ref as the cast intention and evaluates it once,
// mirroring TryToAttack's shape for an owner-commanded special-skill cast.
func (s *Summon) TryToCast(target attackable.Combatant, ref skill.Ref) bool {
	s.mu.Lock()
	defer s.mu.Unlock()

	if target == nil || s.actor.DenyAIAction() || s.cast == nil {
		return false
	}
	if !s.cast.CanAttempt(target, ref) {
		return false
	}
	if s.busyLocked() {
		s.next = intention{kind: IntentionCast, target: target, skill: ref}
		return true
	}
	s.current = intention{kind: IntentionCast, target: target, skill: ref}
	accepted, err := s.thinkCastLocked()
	if err != nil {
		s.log.Warn().Err(err).Msg("ai: summon broadcast")
	}
	return accepted
}

// TryToIdle clears active and queued intentions, then stops movement.
func (s *Summon) TryToIdle() {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.current = intention{kind: IntentionIdle}
	s.next = intention{}
	s.move.Stop()
}

// FollowInstead makes following target the current intention, dropping any
// queued one, and acts on it once: the summon walks toward target only when
// it is able to move, and otherwise resumes on a later think.
func (s *Summon) FollowInstead(target attackable.Combatant) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.current = intention{kind: IntentionFollow, target: target}
	s.next = intention{}
	if _, err := s.thinkFollowLocked(); err != nil {
		s.log.Warn().Err(err).Msg("ai: summon broadcast")
	}
}

// StopMove stops movement; intentions are left as they are.
func (s *Summon) StopMove() { s.move.Stop() }

// StopAttack stops the attack cycle; intentions are left as they are.
func (s *Summon) StopAttack() { s.attack.Stop() }

// AbortAll stops movement, the attack cycle and any in-flight cast, in that
// order. Intentions are left as they are.
func (s *Summon) AbortAll() {
	s.mu.Lock()
	cast := s.cast
	s.mu.Unlock()
	s.move.Stop()
	s.attack.Stop()
	if cast != nil {
		cast.Stop()
	}
}

// StartOffensiveFollowTicker launches the 500 ms offensive-follow recheck
// loop on q, the owner's queue, and returns the func the caller stops it
// with on despawn.
func (s *Summon) StartOffensiveFollowTicker(q *sim.Queue) (stop func()) {
	return q.Every(summonOffensiveFollowTick, s.recheckOffensiveFollow).Stop
}

// recheckOffensiveFollow re-evaluates only the in-flight attack/cast
// offensive follow every 500 ms (CreatureMove.java:556-561), matching the
// reference's follow-task cadence, which runs independently of the shared
// 1 s AI think tick. That reference task (offensiveFollowTask,
// CreatureMove.java:563-584) manages movement only and has no attack/cast
// execution path, so this deliberately calls MaybeStartOffensiveFollow
// directly rather than the full thinkAttackLocked/thinkCastLocked — reusing
// those would also re-run DoAttack/Cast on this 500 ms cadence once a
// summon is already in range, doubling its attack/cast rate for any weapon
// or cast fast enough to clear its cooldown inside 500 ms. Attack/cast
// execution, friendly follow, and idle all stay on the shared 1 s Think
// cadence.
func (s *Summon) recheckOffensiveFollow() {
	s.mu.Lock()
	defer s.mu.Unlock()

	if s.actor.DenyAIAction() {
		return
	}

	target := s.current.target
	var attackRange int
	switch s.current.kind {
	case IntentionAttack:
		attackRange = s.actor.PhysicalAttackRange()
	case IntentionCast:
		if s.cast == nil {
			return
		}
		attackRange = s.cast.Range(s.current.skill)
	default:
		return
	}

	if lost, err := s.targetLostLocked(target); lost {
		if err != nil {
			s.log.Warn().Err(err).Msg("ai: summon broadcast")
		}
		return
	}

	if _, err := s.move.MaybeStartOffensiveFollow(target, attackRange); err != nil {
		s.log.Warn().Err(err).Msg("ai: summon broadcast")
	}
}

// Think advances the current summon intention once. Any broadcast error is
// logged through SetLogger — Think's own callers (the periodic AI task) have
// no per-actor error path of their own.
func (s *Summon) Think() {
	s.mu.Lock()
	defer s.mu.Unlock()

	s.promoteNextLocked()
	var err error
	switch s.current.kind {
	case IntentionAttack:
		_, err = s.thinkAttackLocked()
	case IntentionFollow:
		_, err = s.thinkFollowLocked()
	case IntentionCast:
		_, err = s.thinkCastLocked()
	}
	if err != nil {
		s.log.Warn().Err(err).Msg("ai: summon broadcast")
	}
}

func (s *Summon) promoteNextLocked() {
	if s.current.kind != IntentionIdle || s.next.kind == IntentionIdle {
		return
	}
	s.current = s.next
	s.next = intention{}
}

func (s *Summon) thinkAttackLocked() (bool, error) {
	if s.actor.DenyAIAction() {
		s.current = intention{kind: IntentionIdle}
		return false, nil
	}

	target := s.current.target
	if lost, err := s.targetLostLocked(target); lost {
		return false, err
	}

	following, err := s.move.MaybeStartOffensiveFollow(target, s.actor.PhysicalAttackRange())
	if following {
		return true, err
	}

	if s.busyLocked() {
		return false, nil
	}

	s.move.Stop()
	if !s.attack.CanAttack(target) {
		s.current = intention{kind: IntentionIdle}
		return false, nil
	}

	s.attack.DoAttack(target)
	return true, nil
}

func (s *Summon) thinkCastLocked() (bool, error) {
	if s.actor.DenyAIAction() || s.cast == nil {
		s.current = intention{kind: IntentionIdle}
		return false, nil
	}
	if s.cast.Disabled() {
		s.current = intention{kind: IntentionIdle}
		return false, nil
	}

	target := s.current.target
	ref := s.current.skill
	if lost, err := s.targetLostLocked(target); lost {
		return false, err
	}

	if !s.cast.CanAttempt(target, ref) {
		return false, nil
	}

	following, err := s.move.MaybeStartOffensiveFollow(target, s.cast.Range(ref))
	if following {
		return true, err
	}

	if s.cast.StopsMovement(ref) {
		s.move.Stop()
		if target.ObjectID() != s.actor.ObjectID() {
			s.actor.SetHeadingTo(target)
		}
	}

	if !s.cast.CanCast(target, ref) {
		s.current = intention{kind: IntentionIdle}
		if target.ObjectID() != s.actor.ObjectID() {
			s.actor.BroadcastMoveToPawn(target)
		}
		return false, nil
	}

	s.cast.Cast(target, ref)
	s.current = intention{kind: IntentionIdle}
	return true, nil
}

func (s *Summon) thinkFollowLocked() (bool, error) {
	if s.actor.DenyAIAction() {
		return false, nil
	}

	target := s.current.target
	if lost, err := s.targetLostLocked(target); lost {
		return false, err
	}

	_, err := s.move.MaybeStartFriendlyFollow(target, summonFollowOffset)
	return true, err
}

func (s *Summon) busyLocked() bool {
	return s.attack.BowCoolingDown() || s.attack.AttackingNow() || (s.cast != nil && s.cast.CastingNow())
}

// AttackingNow reports whether this summon's own attack cycle is currently
// in flight, matching CreatureAttack.isAttackingNow (CreatureAttack.java:56-59).
func (s *Summon) AttackingNow() bool {
	return s.attack != nil && s.attack.AttackingNow()
}

// targetLostLocked matches AbstractAI.isTargetLost (AbstractAI.java:586-594,
// unmodified by SummonAI's override at SummonAI.java:275-281): only a nil or
// no-longer-known target counts as lost. Death (true or fake) is not a loss
// condition here — a dead target keeps the attack/follow intention until it
// despawns, same as the reference. On loss it also cancels any in-flight
// movement leg (SummonMove.java:48-53's known-list-loss branch, which forces
// the idle path's move.stop() rather than leaving a stale follow/chase
// running toward a target the actor no longer knows about).
func (s *Summon) targetLostLocked(target attackable.Combatant) (bool, error) {
	if target == nil || !s.actor.Knows(target) {
		s.current = intention{kind: IntentionIdle}
		if sameCombatant(s.next.target, target) {
			s.next = intention{}
		}
		s.move.Stop()
		return true, nil
	}
	return false, nil
}
