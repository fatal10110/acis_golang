package ai

import "github.com/fatal10110/acis_golang/internal/gameserver/model/actor/attackable"

const attackHateDecay = 6.6

// AttackableActor is the actor state used by the hostile NPC intention loop.
type AttackableActor interface {
	attackable.Combatant
	DenyAIAction() bool
	Knows(attackable.Combatant) bool
	PhysicalAttackRange() int
	ReturnHome() bool
	InTerritory() bool
}

// MoveController controls movement requests emitted by the AI loop.
type MoveController interface {
	MaybeStartOffensiveFollow(target attackable.Combatant, attackRange int) bool
	Stop()
}

// AttackController controls attack requests emitted by the AI loop.
type AttackController interface {
	BowCoolingDown() bool
	AttackingNow() bool
	CanAttack(attackable.Combatant) bool
	DoAttack(attackable.Combatant)
}

type intention struct {
	kind   Intention
	target attackable.Combatant
}

// Attackable drives one hostile NPC's combat and wander intentions.
//
// One AI loop owns the current and next intentions. Threat and hate tables
// are internally synchronized so combat code can raise hate while the loop
// reads target selection.
type Attackable struct {
	actor   AttackableActor
	move    MoveController
	attack  AttackController
	threats *attackable.ThreatTable
	hates   *attackable.HateTable

	current intention
	next    intention
	step    int
}

// NewAttackable builds an idle hostile NPC AI loop.
func NewAttackable(actor AttackableActor, move MoveController, attack AttackController) *Attackable {
	return &Attackable{
		actor:   actor,
		move:    move,
		attack:  attack,
		threats: attackable.NewThreatTable(actor),
		hates:   attackable.NewHateTable(actor),
		current: intention{kind: IntentionIdle},
	}
}

// ObjectID returns the actor id controlled by this AI loop.
func (a *Attackable) ObjectID() int32 {
	return a.actor.ObjectID()
}

// Threats returns the physical-attack threat table.
func (a *Attackable) Threats() *attackable.ThreatTable {
	return a.threats
}

// Hates returns the skill-cast hate table.
func (a *Attackable) Hates() *attackable.HateTable {
	return a.hates
}

// AddDamageHate records an attacker in the physical threat table.
func (a *Attackable) AddDamageHate(attacker attackable.Combatant, damage, hate float64) {
	a.threats.AddDamage(attacker, damage, hate)
}

// AddHate records an attacker in the skill-cast hate table.
func (a *Attackable) AddHate(attacker attackable.Combatant, hate float64) {
	a.hates.Add(attacker, hate)
}

// SetWander makes the next Think process wander/return-home behavior.
func (a *Attackable) SetWander() {
	a.current = intention{kind: IntentionWander}
}

// CurrentIntention returns the currently active intention kind.
func (a *Attackable) CurrentIntention() Intention {
	return a.current.kind
}

// NextIntention returns the queued intention, if one exists.
func (a *Attackable) NextIntention() (Intention, attackable.Combatant, bool) {
	if a.next.kind == IntentionIdle {
		return IntentionIdle, nil, false
	}
	return a.next.kind, a.next.target, true
}

// Think advances the current intention once.
func (a *Attackable) Think() {
	if a.current.kind == IntentionIdle {
		if threat, ok := a.threats.MostHated(); ok {
			a.current = intention{kind: IntentionAttack, target: threat.Attacker}
		}
	}

	switch a.current.kind {
	case IntentionAttack:
		a.thinkAttack()
	case IntentionWander:
		a.thinkWander()
	}
}

// Tick advances the AI clock and applies periodic hate decay.
func (a *Attackable) Tick() {
	a.step++
	if a.step%3 != 0 {
		return
	}
	a.threats.ReduceAllHate(attackHateDecay)
	a.hates.ReduceAllHate(66000)
	a.step = 0
}

func (a *Attackable) thinkAttack() {
	if a.actor.DenyAIAction() {
		return
	}

	target := a.current.target
	if a.targetLost(target) {
		return
	}

	if a.move.MaybeStartOffensiveFollow(target, a.actor.PhysicalAttackRange()) {
		return
	}

	if a.attack.BowCoolingDown() || a.attack.AttackingNow() {
		a.next = a.current
		return
	}

	if !a.attack.CanAttack(target) {
		return
	}

	a.move.Stop()
	a.attack.DoAttack(target)
}

func (a *Attackable) thinkWander() {
	if a.actor.ReturnHome() {
		return
	}
	if !a.actor.InTerritory() {
		a.current = intention{kind: IntentionIdle}
	}
}

func (a *Attackable) targetLost(target attackable.Combatant) bool {
	if target == nil {
		return true
	}
	if target.AlikeDead() {
		return true
	}
	return !a.actor.Knows(target)
}
