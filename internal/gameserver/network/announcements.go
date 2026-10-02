package network

import (
	"github.com/fatal10110/acis_golang/internal/commons/wire"
	"github.com/fatal10110/acis_golang/internal/gameserver/handler/chat"
	"github.com/fatal10110/acis_golang/internal/gameserver/network/serverpackets"
	"github.com/fatal10110/acis_golang/internal/gameserver/world"
)

// worldAnnouncer delivers the automatic announcements to every player in
// state.
type worldAnnouncer struct {
	state *world.State
}

// NewAnnouncer returns the announcement delivery to every player in state.
func NewAnnouncer(state *world.State) worldAnnouncer {
	return worldAnnouncer{state: state}
}

// Announce sends text to every player online, on the critical
// announcement channel when critical.
func (a worldAnnouncer) Announce(text string, critical bool) {
	announceToOnline(a.state, text, critical)
}

// announceChannel is the chat channel an announcement is shown on.
func announceChannel(critical bool) chat.Type {
	if critical {
		return chat.CriticalAnnounce
	}
	return chat.Announcement
}

// announceToOnline sends text, said by no one, to every player online on
// the announcement channel, the critical one when critical.
func announceToOnline(state *world.State, text string, critical bool) {
	if state == nil {
		return
	}
	typ := int32(announceChannel(critical))
	broadcastFrame(func() wire.Frame {
		return serverpackets.FrameCreatureSay(0, typ, "", text)
	}, func(send func(frameReceiver)) {
		for _, p := range state.Players() {
			if listener, ok := p.(*livePlayer); ok {
				send(listener)
			}
		}
	})
}

// sendLoginAnnouncements reads live the announcements that are not
// automatic, each under live's own name.
func (l *GameClientLink) sendLoginAnnouncements(live *livePlayer) {
	for _, a := range l.announcements.Login() {
		live.SendFrame(serverpackets.FrameCreatureSay(0, int32(announceChannel(a.Critical)), live.Name, a.Message))
	}
}
