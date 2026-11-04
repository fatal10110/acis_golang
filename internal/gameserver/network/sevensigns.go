package network

import (
	"github.com/fatal10110/acis_golang/internal/commons/wire"
	"github.com/fatal10110/acis_golang/internal/gameserver/festival"
	"github.com/fatal10110/acis_golang/internal/gameserver/network/clientpackets"
	"github.com/fatal10110/acis_golang/internal/gameserver/network/serverpackets"
	"github.com/fatal10110/acis_golang/internal/gameserver/sevensigns"
	"github.com/fatal10110/acis_golang/internal/gameserver/world"
)

// sevenSignsRecordsHandler is the etc-item handler of the Record of Seven
// Signs, whose use opens the record's first page. The item is not consumed.
const sevenSignsRecordsHandler = "SevenSignsRecords"

// requestSSQStatus answers RequestSSQStatus with the asked page of live's
// Record of Seven Signs. The prediction page is refused silently once the
// competition is over (results and seal validation): the client shows the
// settled seals instead and waits for no answer.
func (l *GameClientLink) requestSSQStatus(live *livePlayer, req clientpackets.RequestSSQStatus) {
	if l.sevenSigns == nil {
		live.SendFrame(serverpackets.FrameActionFailed())
		return
	}
	period := l.sevenSigns.CurrentPeriod()
	if req.Page == serverpackets.SSQPagePrediction && (period == sevensigns.SealValidation || period == sevensigns.Results) {
		return
	}
	if req.Page == serverpackets.SSQPageFestival {
		frame, ok := l.ssqFestivalFrame(period)
		if !ok {
			// The festival keeps no score for this cycle: its data is
			// missing from the database, and the page cannot be built.
			l.log.Error().Msg("seven signs: no festival scores for the current cycle")
			live.SendFrame(serverpackets.FrameActionFailed())
			return
		}
		live.SendFrame(frame)
		return
	}
	live.SendFrame(l.ssqStatusFrame(live.ObjectID(), req.Page))
}

// ssqFestivalFrame builds the record's second page: each festival's worth
// and each cabal's best score in it this cycle, with the party that set
// it. ok is false when the festival keeps no score for one of them.
func (l *GameClientLink) ssqFestivalFrame(period sevensigns.Period) (wire.Frame, bool) {
	if l.festival == nil {
		return wire.Frame{}, false
	}
	festivals := make([]serverpackets.SSQFestival, festival.Count)
	for id := range festival.Count {
		dusk, okDusk := l.festival.HighestScore(sevensigns.Dusk, id)
		dawn, okDawn := l.festival.HighestScore(sevensigns.Dawn, id)
		if !okDusk || !okDawn {
			return wire.Frame{}, false
		}
		festivals[id] = serverpackets.SSQFestival{
			MaxScore: int32(festival.MaxScores[id]),
			Dusk:     serverpackets.SSQFestivalBest{Score: int32(dusk.Score), Members: dusk.MemberNames()},
			Dawn:     serverpackets.SSQFestivalBest{Score: int32(dawn.Score), Members: dawn.MemberNames()},
		}
	}
	return serverpackets.FrameSSQStatusFestival(byte(period), festivals), true
}

// ssqStatusFrame builds page of objectID's Record of Seven Signs.
func (l *GameClientLink) ssqStatusFrame(objectID int32, page byte) wire.Frame {
	return serverpackets.FrameSSQStatus(page, l.sevenSigns.Record(objectID))
}

// useSevenSignsRecords opens the first page of live's Record of Seven Signs.
func (l *GameClientLink) useSevenSignsRecords(live *livePlayer) {
	if l.sevenSigns == nil {
		live.SendFrame(serverpackets.FrameActionFailed())
		return
	}
	live.SendFrame(l.ssqStatusFrame(live.ObjectID(), serverpackets.SSQPageRecord))
}

// ssqSkyFrame builds the SSQInfo packet showing the sky the Seven Signs
// state calls for.
func ssqSkyFrame(state *sevensigns.State) wire.Frame {
	if state == nil {
		return serverpackets.FrameSSQInfo()
	}
	return serverpackets.FrameSSQInfoSky(serverpackets.SSQSky(state.Sky()))
}

// sevenSignsBroadcaster delivers Seven Signs period-change notices to every
// player in state.
type sevenSignsBroadcaster struct {
	state *world.State
}

// NewSevenSignsBroadcaster returns the delivery of Seven Signs period-change
// notices to every player online.
func NewSevenSignsBroadcaster(state *world.State) sevensigns.Broadcaster {
	return sevenSignsBroadcaster{state: state}
}

// Broadcast sends each notice, in order, to every player online.
func (b sevenSignsBroadcaster) Broadcast(notices []sevensigns.Notice) {
	if b.state == nil {
		return
	}
	for _, n := range notices {
		build, ok := sevenSignsNoticeFrame(n)
		if !ok {
			continue
		}
		broadcastFrame(build, func(send func(frameReceiver)) {
			for _, p := range b.state.Players() {
				if listener, ok := p.(*livePlayer); ok {
					send(listener)
				}
			}
		})
	}
}

// sevenSignsNoticeFrame returns the builder of the packet announcing n.
func sevenSignsNoticeFrame(n sevensigns.Notice) (func() wire.Frame, bool) {
	message := func(id int) (func() wire.Frame, bool) {
		return func() wire.Frame { return serverpackets.FrameSystemMessage(id) }, true
	}
	switch n.Kind {
	case sevensigns.NoticeSound:
		return func() wire.Frame { return serverpackets.FramePlaySound(n.Sound) }, true
	case sevensigns.NoticeCompetitionBegun:
		return message(serverpackets.SystemMessageQuestEventPeriodBegun)
	case sevensigns.NoticeCompetitionEnded:
		return message(serverpackets.SystemMessageQuestEventPeriodEnded)
	case sevensigns.NoticeValidationBegun:
		return message(serverpackets.SystemMessageSealValidationPeriodBegun)
	case sevensigns.NoticeValidationEnded:
		return message(serverpackets.SystemMessageSealValidationPeriodEnded)
	case sevensigns.NoticeCabalWon:
		if n.Cabal == sevensigns.Dawn {
			return message(serverpackets.SystemMessageDawnWon)
		}
		return message(serverpackets.SystemMessageDuskWon)
	case sevensigns.NoticeSealObtained:
		if id, ok := sealObtainedMessage(n.Cabal, n.Seal); ok {
			return message(id)
		}
	case sevensigns.NoticeSky:
		sky := serverpackets.SSQSky(n.Cabal)
		return func() wire.Frame { return serverpackets.FrameSSQInfoSky(sky) }, true
	}
	return nil, false
}

func sealObtainedMessage(c sevensigns.Cabal, s sevensigns.Seal) (int, bool) {
	if s < sevensigns.Avarice || s > sevensigns.Strife {
		return 0, false
	}
	offset := int(s - sevensigns.Avarice)
	switch c {
	case sevensigns.Dawn:
		return serverpackets.SystemMessageDawnObtainedAvarice + offset, true
	case sevensigns.Dusk:
		return serverpackets.SystemMessageDuskObtainedAvarice + offset, true
	}
	return 0, false
}
