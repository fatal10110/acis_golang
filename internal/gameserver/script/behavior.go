package script

import (
	"fmt"
	"slices"

	"github.com/fatal10110/acis_golang/internal/gameserver/model/actor/ai"
	"github.com/fatal10110/acis_golang/internal/gameserver/model/actor/attackable"
	"github.com/fatal10110/acis_golang/internal/gameserver/model/actor/npc"
	"github.com/fatal10110/acis_golang/internal/gameserver/model/location"
	"github.com/fatal10110/acis_golang/internal/gameserver/model/skill"
)

// The NPC operations of the behavior base: its AI parameters, its hate,
// its state and the moves a behavior makes it do. An operation only a
// hostile NPC supports panics on any other NPC, so the invocation aborts
// there; the seam gate keeps behaviors that call one off civilian NPCs.

// aiParams is what an NPC reads its AI parameters through: its spawn's
// values first, its template's second.
type aiParams interface {
	AIInt(name string, def int32) int32
	AIStringParam(name string) (string, bool)
	AISkill(name string) (skill.Ref, bool, error)
}

var (
	_ aiParams = (*npc.Hostile)(nil)
	_ aiParams = (*npc.Folk)(nil)
)

// params returns the NPC's AI parameters; a handle on nothing panics.
func (n *NPC) params() aiParams {
	switch o := n.combatant().(type) {
	case *npc.Hostile:
		return o
	case *npc.Folk:
		return o
	}
	panic("script: the AI parameters of a handle on nothing")
}

// hostile returns the hostile NPC n handles. Any other NPC panics: what
// names the operation.
func (n *NPC) hostile(what string) *npc.Hostile {
	h, ok := n.combatant().(*npc.Hostile)
	if !ok {
		panic(fmt.Sprintf("script: %s for %T: only a hostile npc does it", what, n.combatant()))
	}
	return h
}

// IntParam returns the NPC's int AI parameter name, 0 when neither its
// spawn nor its template sets it or the value is not an int.
func (n *NPC) IntParam(name string) int32 { return n.IntParamOr(name, 0) }

// IntParamOr is IntParam with def in place of 0.
func (n *NPC) IntParamOr(name string, def int32) int32 { return n.params().AIInt(name, def) }

// StringParam returns the NPC's AI parameter name; ok is false when
// neither its spawn nor its template sets it.
func (n *NPC) StringParam(name string) (v string, ok bool) { return n.params().AIStringParam(name) }

// SkillParam returns the NPC's AI parameter name read as an "id-level"
// skill; ok is false when neither its spawn nor its template sets it. A
// value that is not one panics.
func (n *NPC) SkillParam(name string) (skill.Ref, bool) {
	ref, ok, err := n.params().AISkill(name)
	if err != nil {
		panic(fmt.Sprintf("script: %v", err))
	}
	return ref, ok
}

// SkillByType returns the skill the NPC's template lists under t; ok is
// false for none.
func (n *NPC) SkillByType(t npc.SkillType) (skill.Ref, bool) {
	ref, ok := n.instance().Template.SkillsByType[t]
	return ref, ok
}

// Clans returns the clans the NPC's template belongs to.
func (n *NPC) Clans() []string { return slices.Clone(n.instance().Template.Clans) }

// LifeTime returns the AI cycles the NPC has completed since it spawned.
func (n *NPC) LifeTime() int32 {
	switch o := n.combatant().(type) {
	case *npc.Hostile:
		return o.LifeTime()
	case *npc.Folk:
		return o.LifeTime()
	}
	panic("script: the life time of a handle on nothing")
}

// InMyTerritory reports whether the NPC stands in its territory, its
// master's for a private.
func (n *NPC) InMyTerritory() bool {
	switch o := n.combatant().(type) {
	case *npc.Hostile:
		return o.InTerritory()
	case *npc.Folk:
		return o.InTerritory()
	}
	panic("script: the territory of a handle on nothing")
}

// InSpawnTerritory reports whether c stands in the territory of the spawn
// that placed the NPC.
func (n *NPC) InSpawnTerritory(c Creature) bool {
	l := locOf(c)
	return n.hostile("check the spawn territory").SpawnTerritoryContains(location.Location{X: l.X, Y: l.Y, Z: l.Z})
}

// MostHated returns the creature the NPC hates most, nil when it hates
// none.
func (n *NPC) MostHated() Creature {
	var (
		c  attackable.Combatant
		ok bool
	)
	switch o := n.combatant().(type) {
	case *npc.Hostile:
		var t attackable.Threat
		t, ok = o.AI().Threats().MostHated()
		c = t.Attacker
	case *npc.Folk:
		c, ok = o.MostHated()
	default:
		panic("script: the hate of a handle on nothing")
	}
	if !ok {
		return nil
	}
	return creatureOf(c)
}

// StopHate zeroes the NPC's hate of c and drops its attack desire on c;
// the NPC goes back to peace when it then hates no one.
func (n *NPC) StopHate(c Creature) {
	t := combatantOf(c)
	if t == nil {
		return
	}
	switch o := n.combatant().(type) {
	case *npc.Hostile:
		o.StopAggroHate(t)
	case *npc.Folk:
		o.StopHate(t)
	default:
		panic("script: the hate of a handle on nothing")
	}
}

// ClearDesires drops every desire the NPC holds, whatever its kind. A
// handle on nothing does nothing.
func (n *NPC) ClearDesires() {
	switch o := n.combatant().(type) {
	case *npc.Hostile:
		o.AI().Desires().Clear()
	case *npc.Folk:
		o.ClearDesires()
	}
}

// RemoveAllAttackDesire zeroes the NPC's hate of every attacker, drops its
// attack desires and stops its movement. It does not send the NPC back to
// peace.
func (n *NPC) RemoveAllAttackDesire() { n.hostile("remove the attack desires").RemoveAllAttackDesire() }

// Intention is the kind of intention an NPC's AI acts on.
type Intention = ai.Intention

// The intentions a behavior compares an NPC's with.
const (
	IntentionIdle    = ai.IntentionIdle
	IntentionAttack  = ai.IntentionAttack
	IntentionCast    = ai.IntentionCast
	IntentionFlee    = ai.IntentionFlee
	IntentionFollow  = ai.IntentionFollow
	IntentionMoveTo  = ai.IntentionMoveTo
	IntentionSocial  = ai.IntentionSocial
	IntentionWander  = ai.IntentionWander
	IntentionNothing = ai.IntentionNothing
)

// Intention returns the kind of the intention the NPC's AI acts on.
func (n *NPC) Intention() Intention {
	switch o := n.combatant().(type) {
	case *npc.Hostile:
		return o.AI().CurrentIntention()
	case *npc.Folk:
		return o.CurrentIntention()
	}
	panic("script: the intention of a handle on nothing")
}

// Stunned reports whether the NPC is stunned.
func (n *NPC) Stunned() bool { return n.hostile("check a stun").Stunned() }

// Paralyzed reports whether the NPC is paralyzed.
func (n *NPC) Paralyzed() bool { return n.hostile("check a paralysis").Paralyzed() }

// RechargeShots charges the NPC's soulshot when physical is set and its
// spiritshot when magic is set, as its shots allow, showing the players
// around it.
func (n *NPC) RechargeShots(physical, magic bool) {
	n.hostile("recharge shots").RechargeShots(physical, magic)
}

// Shout shouts text as the NPC's chat line to the players around it. A
// handle on nothing shouts nothing.
func (n *NPC) Shout(text string) {
	switch o := n.combatant().(type) {
	case *npc.Hostile:
		o.Shout(text)
	case *npc.Folk:
		o.Shout(text)
	}
}

// GeoPathFailCount returns how many times in a row the NPC's path finding
// has failed.
func (n *NPC) GeoPathFailCount() int { return n.hostile("read the path failures").GeoPathFailCount() }

// CanAttack reports whether the NPC may attack c now: it can attack, c is
// within reach when it cannot move, it knows c, c may be attacked by it, it
// sees c and c does not fake death.
func (n *NPC) CanAttack(c Creature) bool {
	t := combatantOf(c)
	return t != nil && n.hostile("check an attack").AI().CanAttack(t)
}

// DoDie kills the NPC, killer taking the kill. A dead NPC is left as it is.
func (n *NPC) DoDie(killer Creature) { n.hostile("die").Kill(combatantOf(killer)) }

// IntSlot names one integer of an NPC's script memory.
type IntSlot = npc.IntSlot

// CreatureSlot names one creature reference of an NPC's script memory.
type CreatureSlot = npc.CreatureSlot

// The script memory slots a behavior reads and writes. Param1 to Param3
// hold the spawn parameters a private started with; WeightPoint, 1 until a
// script sets it, holds a private's weight point from its declaration.
const (
	SlotIntAI0      = npc.IntAI0
	SlotIntAI1      = npc.IntAI1
	SlotIntAI2      = npc.IntAI2
	SlotIntAI3      = npc.IntAI3
	SlotIntAI4      = npc.IntAI4
	SlotParam1      = npc.IntParam1
	SlotParam2      = npc.IntParam2
	SlotParam3      = npc.IntParam3
	SlotFlag        = npc.IntFlag
	SlotWeightPoint = npc.IntWeightPoint
	SlotCreature0   = npc.CreatureAI0
	SlotCreature1   = npc.CreatureAI1
	SlotCreature2   = npc.CreatureAI2
	SlotCreature3   = npc.CreatureAI3
	SlotCreature4   = npc.CreatureAI4
)

// Int returns the integer the NPC's script memory holds in slot.
func (n *NPC) Int(slot IntSlot) int32 { return n.scratch().Int(slot) }

// SetInt stores v in slot of the NPC's script memory.
func (n *NPC) SetInt(slot IntSlot, v int32) { n.scratch().SetInt(slot, v) }

// Slot returns the creature the NPC's script memory holds in slot, nil
// for none. It is the creature stored, whatever became of it since.
func (n *NPC) Slot(slot CreatureSlot) Creature {
	c, ok := n.scratch().Creature(slot).(attackable.Combatant)
	if !ok {
		return nil
	}
	return creatureOf(c)
}

// SetSlot stores c in slot of the NPC's script memory; nil empties it.
func (n *NPC) SetSlot(slot CreatureSlot, c Creature) {
	var v npc.ScratchCreature
	if t := combatantOf(c); t != nil {
		v = t
	}
	n.scratch().SetCreature(slot, v)
}
