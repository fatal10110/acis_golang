package npc

import (
	"errors"
	"fmt"

	"github.com/fatal10110/acis_golang/internal/gameserver/model/actor/ai"
	"github.com/fatal10110/acis_golang/internal/gameserver/model/actor/attackable"
	"github.com/fatal10110/acis_golang/internal/gameserver/world"
)

var hostileInstanceKinds = map[InstanceKind]struct{}{
	"Chest":           {},
	"FeedableBeast":   {},
	"FestivalMonster": {},
	"FriendlyMonster": {},
	"GrandBoss":       {},
	"Guard":           {},
	"HalishaChest":    {},
	"Monster":         {},
	"RaidBoss":        {},
	"SiegeGuard":      {},
}

// Hostile is a live attackable NPC with world presence and an AI loop.
type Hostile struct {
	world.Presence

	Instance *Instance

	brain *ai.Attackable
}

// NewHostile creates a live attackable NPC wrapper for inst.
func NewHostile(inst *Instance, movement ai.MoveController, attack ai.AttackController) (*Hostile, error) {
	if inst == nil {
		return nil, errors.New("npc: nil hostile instance")
	}
	if inst.Template == nil {
		return nil, errors.New("npc: hostile instance has nil template")
	}
	kind := hostileKind(inst)
	if _, ok := hostileInstanceKinds[kind]; !ok {
		return nil, fmt.Errorf("npc %d: instance type %q is not attackable", inst.Template.ID, kind)
	}
	if movement == nil {
		return nil, errors.New("npc: nil hostile movement")
	}
	if attack == nil {
		return nil, errors.New("npc: nil hostile attack")
	}

	h := &Hostile{Instance: inst}
	h.brain = ai.NewAttackable(h, movement, attack)
	return h, nil
}

// ObjectID returns the world object id assigned to this NPC.
func (h *Hostile) ObjectID() int32 {
	return h.Instance.ObjectID
}

// AI returns the hostile NPC brain.
func (h *Hostile) AI() *ai.Attackable {
	return h.brain
}

// AddDamageHate records physical threat against this NPC.
func (h *Hostile) AddDamageHate(attacker attackable.Combatant, damage, hate float64) {
	h.brain.AddDamageHate(attacker, damage, hate)
}

// AddHate records skill-cast hate against this NPC.
func (h *Hostile) AddHate(attacker attackable.Combatant, hate float64) {
	h.brain.AddHate(attacker, hate)
}

// Tick advances the hostile AI clock once.
func (h *Hostile) Tick() {
	h.brain.Tick()
}

// Think runs one hostile AI decision cycle.
func (h *Hostile) Think() {
	h.brain.Think()
}

// SiegeGuard reports whether this NPC is a defensive siege guard.
func (h *Hostile) SiegeGuard() bool {
	return hostileKind(h.Instance) == "SiegeGuard"
}

// AlikeDead reports whether this NPC should be ignored as a live target.
func (h *Hostile) AlikeDead() bool {
	return false
}

// DenyAIAction reports whether this NPC is unable to act.
func (h *Hostile) DenyAIAction() bool {
	return h.AlikeDead()
}

// Knows reports whether target is currently visible to this NPC.
func (h *Hostile) Knows(target attackable.Combatant) bool {
	tracked, ok := target.(world.Tracked)
	return ok && world.Knows(h, tracked)
}

// PhysicalAttackRange returns this NPC's melee attack range.
func (h *Hostile) PhysicalAttackRange() int {
	return h.Instance.Template.BaseAttackRange
}

// ReturnHome reports whether this NPC started returning to its spawn.
func (h *Hostile) ReturnHome() bool {
	return false
}

// InTerritory reports whether this NPC is inside its spawn territory.
func (h *Hostile) InTerritory() bool {
	return true
}

func hostileKind(inst *Instance) InstanceKind {
	if inst.Kind != "" {
		return inst.Kind
	}
	return InstanceKind(inst.Template.Type)
}
