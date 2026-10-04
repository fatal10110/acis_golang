package network

import (
	"github.com/fatal10110/acis_golang/internal/gameserver/model/actor/attackable"
	"github.com/fatal10110/acis_golang/internal/gameserver/network/serverpackets"
	"github.com/fatal10110/acis_golang/internal/gameserver/world"
)

// playerFollowOffset is how close a following player keeps to its target.
const playerFollowOffset = 70

// followIntention is a follow request queued behind a swing, a cast or a
// sit/stand transition.
type followIntention struct {
	target attackable.Combatant
	shift  bool
}

// actOnPlayer answers a click on an already-selected player and reports
// whether target was one. A player attackable without forcing, or forced
// (ctrl) and attackable, is attacked; one operating a store is interacted
// with, which walks to it and opens its store window; anyone else is
// followed.
func (l *GameClientLink) actOnPlayer(live *livePlayer, target world.Tracked, ctrl, shift bool) bool {
	other, ok := target.(*livePlayer)
	if !ok || live == nil {
		return false
	}
	switch {
	case live.InBoat() != other.InBoat():
		// One aboard a boat and the other not: nothing to do.
		live.SendFrame(serverpackets.FrameActionFailed())
	case other.AttackableWithoutForceBy(live.Character) || (ctrl && other.AttackableBy(live.Character)):
		l.attackLiveTarget(live, other, shift)
	case other.Operating():
		l.tryToInteract(live, other, shift)
	default:
		l.followLiveTarget(live, other, ctrl, shift)
	}
	return true
}

// followLiveTarget makes following target live's intention. A player that
// cannot take AI actions, or that clicked itself, is only answered
// ActionFailed; so is a forced (ctrl) request from a player out of control,
// the attack request that player may not send at all. One still swinging,
// casting, sitting down or standing up queues the follow for that to end,
// answered ActionFailed too.
func (l *GameClientLink) followLiveTarget(live *livePlayer, target attackable.Combatant, ctrl, shift bool) {
	if target.ObjectID() == live.ObjectID() || live.DenyAIAction() || (ctrl && liveOutOfControl(live)) {
		live.SendFrame(serverpackets.FrameActionFailed())
		return
	}
	if itemAICastBusy(live) {
		live.deferFollow(target, shift)
		live.SendFrame(serverpackets.FrameActionFailed())
		return
	}
	l.startLiveFollow(live, target, shift)
}

// startLiveFollow replaces every intention live holds with following target
// and thinks it once. The think is always answered ActionFailed. A player
// that cannot act or move, or that held shift, goes idle instead; otherwise
// the follow task starts, leaving any walk under way to finish first.
func (l *GameClientLink) startLiveFollow(live *livePlayer, target attackable.Combatant, shift bool) {
	live.clearParkedApproaches()
	live.takeDeferredPickup()
	live.takeDeferredItemAICast()
	if live.combat != nil {
		live.combat.Replace()
	}
	live.SendFrame(serverpackets.FrameActionFailed())
	if live.DenyAIAction() || live.MovementDisabled() || liveMoveSpeed(live) == 0 || shift || live.move == nil {
		live.tryToIdle(false)
		return
	}
	if _, err := live.move.MaybeStartFriendlyFollow(target, playerFollowOffset); err != nil {
		l.log.Warn().Err(err).Msg("move: broadcast")
	}
}

// finishDeferredFollow runs the follow queued as the next intention, if any,
// and reports whether one was waiting. During a sit-down or stand-up the
// follow stays queued for PostureSettled.
func (l *GameClientLink) finishDeferredFollow(live *livePlayer) bool {
	if live == nil || live.detached() {
		return false
	}
	if inPostureTransition(live) {
		return live.hasDeferredFollow()
	}
	return l.runDeferredFollow(live)
}

// runDeferredFollow starts the queued follow, if any, whatever the posture,
// and reports whether one was waiting.
func (l *GameClientLink) runDeferredFollow(live *livePlayer) bool {
	follow := live.takeDeferredFollow()
	if follow == nil {
		return false
	}
	l.startLiveFollow(live, follow.target, follow.shift)
	return true
}
