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
// with; anyone else is followed.
func (l *GameClientLink) actOnPlayer(live *livePlayer, target world.Tracked, ctrl, shift bool) bool {
	other, ok := target.(*livePlayer)
	if !ok || live == nil {
		return false
	}
	// Boats are not ported, so neither side is ever aboard one and the
	// boat-mismatch refusal never applies.
	switch {
	case other.AttackableWithoutForceBy(live.Character) || (ctrl && other.AttackableBy(live.Character)):
		l.attackLiveTarget(live, other)
	case other.Operating():
		// Private stores are not ported (#137): the store window a click
		// opens has nothing to show yet, so the click is only released.
		l.log.Debug().Int32("target", other.ObjectID()).Msg("targeting: private store interact not modeled")
		live.SendFrame(serverpackets.FrameActionFailed())
	default:
		l.followLiveTarget(live, other, shift)
	}
	return true
}

// followLiveTarget makes following target live's intention. A player that
// cannot take AI actions, or that clicked itself, is only answered
// ActionFailed. One still swinging, casting, sitting down or standing up
// queues the follow for that to end, answered ActionFailed too.
func (l *GameClientLink) followLiveTarget(live *livePlayer, target attackable.Combatant, shift bool) {
	if target.ObjectID() == live.ObjectID() || live.DenyAIAction() {
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
	follow := live.takeDeferredFollow()
	if follow == nil {
		return false
	}
	l.startLiveFollow(live, follow.target, follow.shift)
	return true
}
