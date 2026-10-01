package network

import (
	"time"

	"github.com/fatal10110/acis_golang/internal/commons/wire"
	"github.com/fatal10110/acis_golang/internal/gameserver/handler/chat"
	"github.com/fatal10110/acis_golang/internal/gameserver/network/clientpackets"
	"github.com/fatal10110/acis_golang/internal/gameserver/network/serverpackets"
	"github.com/fatal10110/acis_golang/internal/gameserver/world"
	"github.com/rs/zerolog"
)

// ChatConfig is the chat settings of server.properties.
type ChatConfig struct {
	// Log records every chat line said and every friend message delivered
	// (LogChat); the zero logger records nothing.
	Log zerolog.Logger
	// WalkerProtection drops whispers that start with a bot script command
	// (L2WalkerProtection).
	WalkerProtection bool
	// GlobalDelay is the reuse delay between two lines said on the general
	// and shout channels by one client session (GlobalChatTime).
	GlobalDelay time.Duration
	// TradeDelay is the same for the trade channel (TradeChatTime).
	TradeDelay time.Duration
	// HeroVoiceDelay is the same for the hero channel (HeroVoiceTime).
	HeroVoiceDelay time.Duration
}

// chatAllRadius is how far from the speaker a general chat line is heard.
const chatAllRadius = 1250

// chatHandler delivers one admitted line live said on client.
type chatHandler func(l *GameClientLink, client *Client, live *livePlayer, line chat.Line)

// chatHandlers delivers each chat channel's lines. A line on a channel
// absent here is logged and dropped, with no answer, as are the channels no
// player says lines on (GM, Announcement, Boat, L2Friend, MSNChat,
// CriticalAnnounce). The alliance and party match room channels register
// here with their systems.
var chatHandlers = map[chat.Type]chatHandler{
	chat.All:                (*GameClientLink).chatAll,
	chat.Shout:              (*GameClientLink).chatShout,
	chat.Tell:               (*GameClientLink).chatTell,
	chat.Party:              (*GameClientLink).chatParty,
	chat.Clan:               (*GameClientLink).chatClan,
	chat.Trade:              (*GameClientLink).chatTrade,
	chat.PartyRoomCommander: (*GameClientLink).chatChannelCommander,
	chat.PartyRoomAll:       (*GameClientLink).chatChannelAll,
	chat.HeroVoice:          (*GameClientLink).chatHeroVoice,
	chat.PetitionPlayer:     (*GameClientLink).chatPetition,
	chat.PetitionGM:         (*GameClientLink).chatPetition,
}

// handleSay2 delivers a chat line live says. A line the chat rules drop,
// or one on a channel with no handler, goes unanswered as in the
// reference: the client prints nothing of its own and waits on no reply.
func (l *GameClientLink) handleSay2(client *Client, live *livePlayer, req clientpackets.Say2) {
	line, ok := chat.Admit(req.Type, req.Text, req.Target, live.accessLevel().IsGM, l.chat.WalkerProtection)
	if !ok {
		return
	}
	// A chat-banned player, or a jailed one that is no game master, is
	// refused here with CHATTING_PROHIBITED once punishments exist (#3155).
	l.chat.Log.Info().Msg(chat.LogEntry(line, live.Name))
	handle := chatHandlers[line.Type]
	if handle == nil {
		l.log.Warn().Str("player", live.Name).Stringer("type", line.Type).Msg("chat line on a channel with no handler")
		return
	}
	line.Text = chat.Clean(line.Text)
	handle(l, client, live, line)
}

// frameCreatureSay builds live's line as its listeners see it.
func frameCreatureSay(live *livePlayer, line chat.Line) func() wire.Frame {
	return func() wire.Frame {
		return serverpackets.FrameCreatureSay(live.ObjectID(), int32(line.Type), live.Name, line.Text)
	}
}

// chatAll is heard by live and by every player within chatAllRadius.
func (l *GameClientLink) chatAll(client *Client, live *livePlayer, line chat.Line) {
	if !client.performFloodProtected(floodProtectorGlobalChat, l.chat.GlobalDelay, time.Now()) {
		return
	}
	broadcastFrame(frameCreatureSay(live, line), func(send func(frameReceiver)) {
		send(live)
		if l.world == nil {
			return
		}
		l.world.ForEachKnownInRadius(live, chatAllRadius, func(o world.Tracked) {
			if p, ok := o.(*livePlayer); ok {
				send(p)
			}
		})
	})
}

// chatShout is heard by every player in live's region.
func (l *GameClientLink) chatShout(client *Client, live *livePlayer, line chat.Line) {
	if !client.performFloodProtected(floodProtectorGlobalChat, l.chat.GlobalDelay, time.Now()) {
		return
	}
	l.broadcastToRegion(live, frameCreatureSay(live, line))
}

// chatTrade is heard by every player in live's region.
func (l *GameClientLink) chatTrade(client *Client, live *livePlayer, line chat.Line) {
	if !client.performFloodProtected(floodProtectorTradeChat, l.chat.TradeDelay, time.Now()) {
		return
	}
	l.broadcastToRegion(live, frameCreatureSay(live, line))
}

// chatHeroVoice is heard by every player online. Only a hero speaks on
// it; anyone else's line is dropped without an answer.
func (l *GameClientLink) chatHeroVoice(client *Client, live *livePlayer, line chat.Line) {
	if !live.IsHero() {
		return
	}
	if !client.performFloodProtected(floodProtectorHeroVoice, l.chat.HeroVoiceDelay, time.Now()) {
		return
	}
	if l.world == nil {
		return
	}
	broadcastFrame(frameCreatureSay(live, line), func(send func(frameReceiver)) {
		for _, p := range l.world.Players() {
			if listener, ok := p.(*livePlayer); ok {
				send(listener)
			}
		}
	})
}

// chatTell whispers the line to the named player, and shows live the line
// addressed "->" to that player. A player not in the world, one blocking
// everything, or one blocking live refuses it; a game master's whisper
// passes either block.
func (l *GameClientLink) chatTell(_ *Client, live *livePlayer, line chat.Line) {
	target, ok := l.livePlayerByName(line.Target)
	if !ok || target.detached() {
		live.SendFrame(serverpackets.FrameSystemMessage(serverpackets.SystemMessageTargetNotFound))
		return
	}
	// A jailed or chat-banned target refuses it with TARGET_IS_CHAT_BANNED
	// once punishments exist (#3155).
	if !live.accessLevel().IsGM {
		if target.BlockingAll() {
			live.SendFrame(serverpackets.FrameSystemMessageString(serverpackets.SystemMessageS1BlockedEverything, target.Name))
			return
		}
		if l.relations.IsBlocked(target.ObjectID(), live.ObjectID()) {
			live.SendFrame(serverpackets.FrameSystemMessage(serverpackets.SystemMessageInMessageRefusalMode))
			return
		}
	}
	target.SendFrame(serverpackets.FrameCreatureSay(live.ObjectID(), int32(line.Type), live.Name, line.Text))
	live.SendFrame(serverpackets.FrameCreatureSay(live.ObjectID(), int32(line.Type), "->"+target.Name, line.Text))
}

// chatParty is heard by every member of live's party, live included. With
// no party the line is dropped without an answer.
func (l *GameClientLink) chatParty(_ *Client, live *livePlayer, line chat.Line) {
	view, ok := l.parties.View(live.ObjectID())
	if !ok {
		return
	}
	broadcastFrame(frameCreatureSay(live, line), func(send func(frameReceiver)) {
		for _, m := range view.Members {
			send(m)
		}
	})
}

// chatClan is heard by every member of live's clan in the world, live
// included. With no clan the line is dropped without an answer.
func (l *GameClientLink) chatClan(_ *Client, live *livePlayer, line chat.Line) {
	cl, ok := l.clanService().ClanOf(live.Character)
	if !ok {
		return
	}
	l.broadcastToClan(cl, 0, frameCreatureSay(live, line))
}

// chatChannelAll is a party leader's line to its command channel. From a
// player leading no party in a channel it is dropped without an answer.
func (l *GameClientLink) chatChannelAll(_ *Client, live *livePlayer, line chat.Line) {
	view, ok := l.parties.View(live.ObjectID())
	if !ok || view.Leader.ObjectID() != live.ObjectID() {
		return
	}
	channel, ok := l.parties.Channel(live.ObjectID())
	if !ok {
		return
	}
	l.broadcastToChannel(live, channel.Members, frameCreatureSay(live, line))
}

// chatChannelCommander is the command channel leader's line to its
// channel. From anyone else it is dropped without an answer.
func (l *GameClientLink) chatChannelCommander(_ *Client, live *livePlayer, line chat.Line) {
	channel, ok := l.parties.Channel(live.ObjectID())
	if !ok || channel.Leader.ObjectID() != live.ObjectID() {
		return
	}
	l.broadcastToChannel(live, channel.Members, frameCreatureSay(live, line))
}

// broadcastToChannel sends speaker's line to every channel member that
// does not block speaker.
func (l *GameClientLink) broadcastToChannel(speaker *livePlayer, members []*livePlayer, build func() wire.Frame) {
	broadcastFrame(build, func(send func(frameReceiver)) {
		for _, m := range members {
			if !l.relations.IsBlocked(m.ObjectID(), speaker.ObjectID()) {
				send(m)
			}
		}
	})
}

// broadcastToRegion sends one copy of the built frame to every player in
// the world whose position falls in the same restart region as speaker's,
// speaker included. Players outside every region share the empty one.
func (l *GameClientLink) broadcastToRegion(speaker *livePlayer, build func() wire.Frame) {
	if l.world == nil {
		return
	}
	region := l.restarts.PointIndexAt(speaker.CurrentLocation())
	broadcastFrame(build, func(send func(frameReceiver)) {
		for _, p := range l.world.Players() {
			if listener, ok := p.(*livePlayer); ok && l.restarts.PointIndexAt(listener.CurrentLocation()) == region {
				send(listener)
			}
		}
	})
}
