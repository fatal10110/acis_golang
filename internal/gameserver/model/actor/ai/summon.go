package ai

import (
	"sync"
	"time"

	"github.com/rs/zerolog"

	"github.com/fatal10110/acis_golang/internal/gameserver/model/actor/attackable"
	"github.com/fatal10110/acis_golang/internal/gameserver/model/location"
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
	// RefuseAttackTarget tells the owner an attack target was refused
	// (TARGET_IS_INCORRECT).
	RefuseAttackTarget()
}

// SummonMoveController controls movement requests emitted by a summon AI.
type SummonMoveController interface {
	MoveController
	MaybeStartFriendlyFollow(target attackable.Combatant, offset int) (bool, error)
}

// SummonCastController is the cast controller a summon AI drives: the shared
// AI cast gates plus the playable target conditions checked last, after the
// cost gates.
type SummonCastController interface {
	CastController
	// MeetsCastConditions applies ref's target-type conditions against
	// target, reporting any failure to the owner.
	MeetsCastConditions(target attackable.Combatant, ref skill.Ref, ctrl bool) bool
}

// Summon drives one pet or servitor's owner-directed intentions.
type Summon struct {
	actor  SummonActor
	move   SummonMoveController
	attack AttackController
	cast   SummonCastController
	log    zerolog.Logger

	// mu guards current, next and previous. A Betray effect turns the
	// summon on its owner (TryToAttack) from the caster's queue.
	mu      sync.Mutex
	current intention
	next    intention
	// previous is the intention current last replaced; a finished cast
	// resumes it when it was an attack.
	previous intention
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
func (s *Summon) SetCastController(controller SummonCastController) {
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
// Past the deny and busy checks, a target the playable attack gate refuses
// is reported to the owner and leaves the current intention untouched.
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
	if refusesPlayableTarget(s.actor, target) {
		s.actor.RefuseAttackTarget()
		return false
	}
	s.setCurrentLocked(intention{kind: IntentionAttack, target: target})
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
	s.setCurrentLocked(intention{kind: IntentionFollow, target: target})
	accepted, err := s.thinkFollowLocked()
	if err != nil {
		s.log.Warn().Err(err).Msg("ai: summon broadcast")
	}
	return accepted
}

// TryToCast sets target/ref as the cast intention and evaluates it once,
// mirroring TryToAttack's shape for an owner-commanded special-skill cast.
// ctrl is the command's forced-use modifier, read by the target conditions.
func (s *Summon) TryToCast(target attackable.Combatant, ref skill.Ref, ctrl bool) bool {
	s.mu.Lock()
	defer s.mu.Unlock()

	if target == nil || s.actor.DenyAIAction() || s.cast == nil {
		return false
	}
	if !s.cast.CanAttempt(target, ref) {
		return false
	}
	if s.busyLocked() {
		s.next = intention{kind: IntentionCast, target: target, skill: ref, ctrl: ctrl}
		return true
	}
	s.setCurrentLocked(intention{kind: IntentionCast, target: target, skill: ref, ctrl: ctrl})
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
	s.idleLocked()
}

func (s *Summon) idleLocked() {
	s.setCurrentLocked(intention{kind: IntentionIdle})
	s.move.Stop()
}

// WaitOutIdle reports whether an idle request has to wait for the summon's
// swing or cast to end, and if so drops the queued intention: the idle
// takes its place, which leaves nothing queued, so the swing or cast ending
// carries on as it would with nothing queued.
func (s *Summon) WaitOutIdle() bool {
	s.mu.Lock()
	defer s.mu.Unlock()
	if !s.attack.AttackingNow() && (s.cast == nil || !s.cast.CastingNow()) {
		return false
	}
	s.next = intention{}
	return true
}

// FollowInstead makes following target the current intention, dropping any
// queued one, and acts on it once: the summon walks toward target only when
// it is able to move, and otherwise resumes on a later think.
func (s *Summon) FollowInstead(target attackable.Combatant) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.followInsteadLocked(target)
}

func (s *Summon) followInsteadLocked(target attackable.Combatant) {
	s.setCurrentLocked(intention{kind: IntentionFollow, target: target})
	if _, err := s.thinkFollowLocked(); err != nil {
		s.log.Warn().Err(err).Msg("ai: summon broadcast")
	}
}

// TryToMoveTo makes walking to dest the current intention, dropping any
// queued one, and starts the walk; it reports whether the walk started. The
// caller checks the summon can act and move. The intention holds until
// Arrived, and Think leaves it alone meanwhile, so the walk is requested once.
func (s *Summon) TryToMoveTo(dest location.Location) bool {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.setCurrentLocked(intention{kind: IntentionMoveTo, loc: dest})
	accepted, err := s.move.MoveToLocation(dest)
	if err != nil {
		s.log.Warn().Err(err).Msg("ai: summon broadcast")
	}
	return accepted
}

// StepAside makes walking to dest the current intention, as TryToMoveTo
// does, when the summon is idle or following and free to act; it reports
// whether the walk started. Any other intention keeps the summon where it
// is. A swing or cast only runs under an attack or cast intention, so an
// idle or following summon never has one to wait out.
func (s *Summon) StepAside(dest location.Location) bool {
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.current.kind != IntentionIdle && s.current.kind != IntentionFollow {
		return false
	}
	if s.actor.DenyAIAction() || s.attack.AttackingNow() || (s.cast != nil && s.cast.CastingNow()) {
		return false
	}
	s.setCurrentLocked(intention{kind: IntentionMoveTo, loc: dest})
	accepted, err := s.move.MoveToLocation(dest)
	if err != nil {
		s.log.Warn().Err(err).Msg("ai: summon broadcast")
	}
	return accepted
}

// Arrived ends a walk-to intention whose walk just finished and reports
// whether there was one; the caller then sends the summon idle. Any other
// intention is left for Think.
func (s *Summon) Arrived() bool {
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.current.kind != IntentionMoveTo {
		return false
	}
	s.current = intention{kind: IntentionIdle}
	return true
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

	if s.current.kind == IntentionIdle && !s.busyLocked() {
		s.runNextLocked()
	}
	s.thinkLocked()
}

// FinishedAttack runs the queued intention once a swing ends, replacing the
// attack; with none queued, the current intention carries on.
//
// ponytail: with none queued, a target the summon cannot keep attacking
// (an unflagged player outside duel, Olympiad and PVP zones) should send it
// idle instead; not modeled yet (#2700).
func (s *Summon) FinishedAttack() {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.runNextLocked()
	s.thinkLocked()
}

// FinishedCasting runs the queued intention once a cast ends. With none
// queued, it resumes the attack the cast replaced, or else goes idle and
// reports true: following follow when it is non-nil (as FollowInstead),
// otherwise standing still (as TryToIdle). The idle is decided and applied
// under one hold of mu, so a Betray TryToAttack from the caster's queue lands
// either before it (and is resumed as the attack) or after it (and replaces
// the idle), never between the two.
func (s *Summon) FinishedCasting(follow attackable.Combatant) (idled bool) {
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.runNextLocked() {
		s.thinkLocked()
		return false
	}
	if s.current.kind != IntentionIdle {
		return false
	}
	if s.previous.kind == IntentionAttack {
		s.setCurrentLocked(s.previous)
		s.thinkLocked()
		return false
	}
	if follow != nil {
		s.followInsteadLocked(follow)
	} else {
		s.idleLocked()
	}
	return true
}

// runNextLocked makes the queued intention current, if there is one.
func (s *Summon) runNextLocked() bool {
	if s.next.kind == IntentionIdle {
		return false
	}
	s.setCurrentLocked(s.next)
	return true
}

// setCurrentLocked makes in the current intention, remembering the one it
// replaces and dropping the queued one.
func (s *Summon) setCurrentLocked(in intention) {
	s.previous = s.current
	s.current = in
	s.next = intention{}
}

func (s *Summon) thinkLocked() {
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

	if !s.cast.CanCast(target, ref) || !s.cast.MeetsCastConditions(target, ref, s.current.ctrl) {
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
