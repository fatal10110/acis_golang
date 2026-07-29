package manager

import (
	"github.com/fatal10110/acis_golang/internal/gameserver/model/actor/attack"
	"github.com/fatal10110/acis_golang/internal/gameserver/model/actor/creature"
	"github.com/fatal10110/acis_golang/internal/gameserver/model/actor/move"
	"github.com/fatal10110/acis_golang/internal/gameserver/model/actor/npc"
	"github.com/fatal10110/acis_golang/internal/gameserver/skill/effect"
	"github.com/fatal10110/acis_golang/internal/gameserver/task"
)

type locatedRef struct{ move.Actor }
type creatureActorRef struct{ attack.CreatureActor }
type statOwnerRef struct{ effect.StatOwner }

// newLiveHostile builds a live Hostile for inst, wiring a real movement
// controller (over the Hostile's lifetime movement state) and a real attack
// controller, resolving their mutual construction-order dependency on the
// finished Hostile via locatedRef/creatureActorRef/statOwnerRef.
func newLiveHostile(inst *npc.Instance, speed float64, geo move.Geo, positions *task.PositionUpdates) (*npc.Hostile, error) {
	statRef := &statOwnerRef{}
	live, err := creature.NewLive(inst.Home, speed, geo, statRef)
	if err != nil {
		return nil, err
	}

	locRef := &locatedRef{}
	moveCtl, err := move.NewController(live.Move(), locRef)
	if err != nil {
		return nil, err
	}
	moveCtl.SetPositionUpdates(positions)

	actorRef := &creatureActorRef{}
	attackCtl := attack.NewAttackable(actorRef)

	hostile, err := npc.NewHostile(inst, live, moveCtl, attackCtl)
	if err != nil {
		return nil, err
	}
	if los, ok := geo.(npc.LineOfSight); ok {
		hostile.SetLineOfSight(los)
	}

	locRef.Actor = hostile
	actorRef.CreatureActor = hostile
	statRef.StatOwner = hostile

	// Re-evaluate the AI loop as soon as a chase leg completes or a swing
	// finishes, rather than waiting for the next fixed AI tick — otherwise
	// a hostile NPC only closes distance on, or re-attacks, its target once
	// per task.AITick. CreatureMove tracks position for its own timing only;
	// the arrived hook must push that position into the world-grid presence
	// range checks actually read before re-thinking, or the AI loop re-runs
	// against a stale position forever.
	moveCtl.SetArrived(func() {
		pos := moveCtl.Position()
		hostile.SyncPosition(pos)
		hostile.Think()
	})
	attackCtl.SetFinished(hostile.Think)

	return hostile, nil
}

// deathRewards applies one victim's live death rewards at its position at
// the moment of death, rather than a position fixed when it spawned —
// hostile NPCs can move (offensive follow) between spawning and dying.
