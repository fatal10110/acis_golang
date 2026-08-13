package ai

import (
	"errors"
	"sync"

	"github.com/rs/zerolog"

	"github.com/fatal10110/acis_golang/internal/gameserver/model/actor/attackable"
	"github.com/fatal10110/acis_golang/internal/gameserver/model/skill"
)

const summonFollowOffset = 70

// SummonActor is the live summon state needed by the summon AI loop.
type SummonActor interface {
	attackable.Combatant
	DenyAIAction() bool
	Knows(attackable.Combatant) bool
	PhysicalAttackRange() int
	SetHeadingTo(attackable.Combatant)
	BroadcastMoveToPawn(attackable.Combatant) error
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

	mu      sync.Mutex // guards current and next.
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
	if err := s.move.Stop(); err != nil {
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
	if s.targetLostLocked(target) {
		return false, nil
	}

	following, err := s.move.MaybeStartOffensiveFollow(target, s.actor.PhysicalAttackRange())
	if following {
		return true, err
	}

	if s.busyLocked() {
		return false, nil
	}

	stopErr := s.move.Stop()
	if !s.attack.CanAttack(target) {
		s.current = intention{kind: IntentionIdle}
		return false, stopErr
	}

	attackErr := s.attack.DoAttack(target)
	return true, errors.Join(stopErr, attackErr)
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
	if s.targetLostLocked(target) {
		return false, nil
	}

	if !s.cast.CanAttempt(target, ref) {
		return false, nil
	}

	following, err := s.move.MaybeStartOffensiveFollow(target, s.cast.Range(ref))
	if following {
		return true, err
	}

	var stopErr error
	if s.cast.StopsMovement(ref) {
		stopErr = s.move.Stop()
		if target.ObjectID() != s.actor.ObjectID() {
			s.actor.SetHeadingTo(target)
		}
	}

	if !s.cast.CanCast(target, ref) {
		s.current = intention{kind: IntentionIdle}
		var pawnErr error
		if target.ObjectID() != s.actor.ObjectID() {
			pawnErr = s.actor.BroadcastMoveToPawn(target)
		}
		return false, errors.Join(stopErr, pawnErr)
	}

	s.cast.Cast(target, ref)
	s.current = intention{kind: IntentionIdle}
	return true, stopErr
}

func (s *Summon) thinkFollowLocked() (bool, error) {
	if s.actor.DenyAIAction() {
		return false, nil
	}

	target := s.current.target
	if s.targetLostLocked(target) {
		return false, nil
	}

	_, err := s.move.MaybeStartFriendlyFollow(target, summonFollowOffset)
	return true, err
}

func (s *Summon) busyLocked() bool {
	return s.attack.BowCoolingDown() || s.attack.AttackingNow() || (s.cast != nil && s.cast.Disabled())
}

func (s *Summon) targetLostLocked(target attackable.Combatant) bool {
	if target == nil || target.AlikeDead() || !s.actor.Knows(target) {
		s.current = intention{kind: IntentionIdle}
		if sameCombatant(s.next.target, target) {
			s.next = intention{}
		}
		return true
	}
	return false
}
