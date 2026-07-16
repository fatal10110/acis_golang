package serverpackets

import (
	"github.com/fatal10110/acis_golang/internal/commons/wire"
	"github.com/fatal10110/acis_golang/internal/gameserver/model/actor/move"
)

// FrameMove builds the packet for event: MoveToPawn when event is following
// a target (event.FollowTarget set, so the client re-derives the
// destination from the target's own position), MoveToLocation otherwise.
func FrameMove(objectID int32, event move.Event) wire.Frame {
	if event.FollowTarget != 0 {
		return FrameMoveToPawn(objectID, event.FollowTarget, event.FollowOffset, event.Origin)
	}
	return FrameMoveToLocation(objectID, event.Destination, event.Origin)
}
