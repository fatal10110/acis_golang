package network

import (
	"github.com/fatal10110/acis_golang/internal/gameserver/task"
	"github.com/fatal10110/acis_golang/internal/gameserver/world"
)

var _ task.AttackStanceEffects = attackStanceEffects{}

// attackStanceEffects delivers combat-stance timeout side effects: the
// expired actor's known recipients get AutoAttackStop, and a player's live
// summon gets the same packet. It does not abort combat, casts, pickups,
// seating, or cubic runtimes.
type attackStanceEffects struct {
	state *world.State
}

// NewAttackStanceEffects returns the production combat-stance timeout
// adapter over state.
func NewAttackStanceEffects(state *world.State) task.AttackStanceEffects {
	return attackStanceEffects{state: state}
}

type autoAttackStopBroadcaster interface {
	BroadcastAutoAttackStop()
}

func (e attackStanceEffects) AutoAttackStop(actor task.AttackStanceActor) {
	if actor == nil || e.state == nil {
		return
	}
	e.broadcastStop(actor.ObjectID())
	if _, ok := e.state.Player(actor.ObjectID()); !ok {
		return
	}
	summon, ok := e.state.Summon(actor.ObjectID())
	if !ok || summon == nil {
		return
	}
	e.broadcastStop(summon.ObjectID())
}

func (e attackStanceEffects) broadcastStop(id int32) {
	obj, ok := e.state.Object(id)
	if !ok {
		obj, ok = e.state.Player(id)
		if !ok {
			return
		}
	}
	if b, ok := obj.(autoAttackStopBroadcaster); ok {
		b.BroadcastAutoAttackStop()
	}
}
