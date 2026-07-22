package ai

import (
	"sync"

	"github.com/fatal10110/acis_golang/internal/gameserver/model/actor/attackable"
)

const summonFollowOffset = 70

// SummonActor is the live summon state needed by the summon AI loop.
type SummonActor interface {
	attackable.Combatant
	DenyAIAction() bool
	Knows(attackable.Combatant) bool
	PhysicalAttackRange() int
}

// SummonMoveController controls movement requests emitted by a summon AI.
type SummonMoveController interface {
	MoveController
	MaybeStartFriendlyFollow(target attackable.Combatant, offset int) bool
}

// Summon drives one pet or servitor's owner-directed intentions.
type Summon struct {
	actor  SummonActor
	move   SummonMoveController
	attack AttackController

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
	return s.thinkAttackLocked()
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
	return s.thinkFollowLocked()
}

// TryToIdle clears active and queued intentions, then stops movement.
func (s *Summon) TryToIdle() {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.current = intention{kind: IntentionIdle}
	s.next = intention{}
	s.move.Stop()
}

// Think advances the current summon intention once.
func (s *Summon) Think() {
	s.mu.Lock()
	defer s.mu.Unlock()

	s.promoteNextLocked()
	switch s.current.kind {
	case IntentionAttack:
		s.thinkAttackLocked()
	case IntentionFollow:
		s.thinkFollowLocked()
	}
}

func (s *Summon) promoteNextLocked() {
	if s.current.kind != IntentionIdle || s.next.kind == IntentionIdle {
		return
	}
	s.current = s.next
	s.next = intention{}
}

func (s *Summon) thinkAttackLocked() bool {
	if s.actor.DenyAIAction() {
		s.current = intention{kind: IntentionIdle}
		return false
	}

	target := s.current.target
	if s.targetLostLocked(target) {
		return false
	}

	if s.move.MaybeStartOffensiveFollow(target, s.actor.PhysicalAttackRange()) {
		return true
	}

	if s.busyLocked() {
		s.next = s.current
		return false
	}

	s.move.Stop()
	if !s.attack.CanAttack(target) {
		s.current = intention{kind: IntentionIdle}
		return false
	}

	s.attack.DoAttack(target)
	return true
}

func (s *Summon) thinkFollowLocked() bool {
	if s.actor.DenyAIAction() {
		return false
	}

	target := s.current.target
	if s.targetLostLocked(target) {
		return false
	}

	s.move.MaybeStartFriendlyFollow(target, summonFollowOffset)
	return true
}

func (s *Summon) busyLocked() bool {
	return s.attack.BowCoolingDown() || s.attack.AttackingNow()
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
