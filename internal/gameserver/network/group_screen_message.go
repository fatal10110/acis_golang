package network

import (
	"github.com/fatal10110/acis_golang/internal/commons/wire"
	"github.com/fatal10110/acis_golang/internal/gameserver/model/actor/event"
	"github.com/fatal10110/acis_golang/internal/gameserver/network/serverpackets"
)

// showGroupScreenMessage shows e's text to every member of live's party, or
// of the command channel that party is in, party by party, as the group
// stands now. A player in no such group shows it to no one.
func (l *GameClientLink) showGroupScreenMessage(live *livePlayer, e event.GroupScreenMessageShown) {
	var members []*livePlayer
	if e.Channel {
		view, ok := l.parties.Channel(live.ObjectID())
		if !ok {
			return
		}
		members = view.Members
	} else {
		view, ok := l.parties.View(live.ObjectID())
		if !ok {
			return
		}
		members = view.Members
	}
	sendToMembers(members, func() wire.Frame { return serverpackets.FrameExShowScreenMessage(e.Text, e.DurationMs) })
}
