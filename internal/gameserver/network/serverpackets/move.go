package serverpackets

import (
	"github.com/fatal10110/acis_golang/internal/commons/wire"
	"github.com/fatal10110/acis_golang/internal/gameserver/model/actor/event"
)

// FrameMove builds the packet for event: MoveToPawn when event is following
// a target (event.FollowTarget set, so the client re-derives the
// destination from the target's own position), MoveToLocation otherwise.
func FrameMove(objectID int32, ev event.Move) wire.Frame {
	if ev.FollowTarget != 0 {
		return FrameMoveToPawn(objectID, ev.FollowTarget, ev.FollowOffset, ev.Origin)
	}
	return FrameMoveToLocation(objectID, ev.Destination, ev.Origin)
}
