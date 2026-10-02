package network

import (
	"github.com/fatal10110/acis_golang/internal/commons/wire"
	"github.com/fatal10110/acis_golang/internal/gameserver/network/clientpackets"
	"github.com/fatal10110/acis_golang/internal/gameserver/network/serverpackets"
	"github.com/fatal10110/acis_golang/internal/gameserver/sevensigns"
	"github.com/fatal10110/acis_golang/internal/gameserver/world"
)

// sevenSignsRecordsHandler is the etc-item handler of the Record of Seven
// Signs, whose use opens the record's first page. The item is not consumed.
const sevenSignsRecordsHandler = "SevenSignsRecords"

// Record of Seven Signs pages.
const (
	ssqPageRecord     = 1
	ssqPageFestival   = 2
	ssqPageSeals      = 3
	ssqPagePrediction = 4
)

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
	if req.Page == ssqPagePrediction && (period == sevensigns.SealValidation || period == sevensigns.Results) {
		return
	}
	if req.Page == ssqPageFestival {
		// ponytail: the festival page lists each festival's best score and
		// party, which only the Festival of Darkness keeps (#223); until it
		// exists the page is refused.
		l.log.Debug().Msg("seven signs: festival record page needs the Festival of Darkness")
		live.SendFrame(serverpackets.FrameActionFailed())
		return
	}
	live.SendFrame(l.ssqStatusFrame(live.ObjectID(), req.Page))
}

// ssqStatusFrame builds page of objectID's Record of Seven Signs. A page
// with no content carries only the page number and the active period.
func (l *GameClientLink) ssqStatusFrame(objectID int32, page byte) wire.Frame {
	r := l.sevenSigns.Record(objectID)
	period := byte(r.Period)
	switch page {
	case ssqPageRecord:
		periodMsg, untilMsg := ssqPeriodMessages(r.Period)
		return serverpackets.FrameSSQStatusRecord(period, serverpackets.SSQRecord{
			Cycle:         int32(r.Cycle),
			PeriodMessage: periodMsg,
			UntilMessage:  untilMsg,
			PlayerCabal:   byte(r.PlayerCabal),
			PlayerSeal:    byte(r.PlayerSeal),
			PlayerStones:  int32(r.PlayerStones),
			PlayerAdena:   int32(r.PlayerAncientAdena),
			Dusk:          ssqCabalStanding(r.Dusk),
			Dawn:          ssqCabalStanding(r.Dawn),
		})
	case ssqPageSeals:
		seals := make([]serverpackets.SSQSealVotes, len(r.Seals))
		for i, s := range r.Seals {
			seals[i] = serverpackets.SSQSealVotes{
				Seal: byte(s.Seal), Owner: byte(s.Owner),
				DuskPercent: byte(s.DuskPercent), DawnPercent: byte(s.DawnPercent),
			}
		}
		return serverpackets.FrameSSQStatusSeals(period, seals)
	case ssqPagePrediction:
		seals := make([]serverpackets.SSQSealPrediction, len(r.Seals))
		for i, s := range r.Seals {
			seals[i] = serverpackets.SSQSealPrediction{
				Owner: byte(s.Owner), Predicted: byte(s.Predicted), Message: ssqPredictionMessage(s.Prediction),
			}
		}
		return serverpackets.FrameSSQStatusPrediction(period, byte(r.Winner), seals)
	default:
		return serverpackets.FrameSSQStatusHeader(page, period)
	}
}

func ssqCabalStanding(c sevensigns.CabalStanding) serverpackets.SSQCabalStanding {
	return serverpackets.SSQCabalStanding{
		StoneScore:    int32(c.StoneProportion),
		FestivalScore: int32(c.FestivalScore),
		TotalScore:    int32(c.TotalScore),
		Percent:       byte(c.Percent),
	}
}

// ssqPeriodMessages names the active period and when it ends on the
// record's first page.
func ssqPeriodMessages(p sevensigns.Period) (period, until int32) {
	switch p {
	case sevensigns.Recruiting:
		return serverpackets.SystemMessageInitialPeriod, serverpackets.SystemMessageUntilToday6PM
	case sevensigns.Competition:
		return serverpackets.SystemMessageQuestEventPeriod, serverpackets.SystemMessageUntilMonday6PM
	case sevensigns.Results:
		return serverpackets.SystemMessageResultsPeriod, serverpackets.SystemMessageUntilToday6PM
	default:
		return serverpackets.SystemMessageValidationPeriod, serverpackets.SystemMessageUntilMonday6PM
	}
}

func ssqPredictionMessage(p sevensigns.Prediction) uint16 {
	switch p {
	case sevensigns.PredictionOwnedRetained:
		return serverpackets.SystemMessageSealOwned10MoreVoted
	case sevensigns.PredictionOwnedLost:
		return serverpackets.SystemMessageSealOwned10LessVoted
	case sevensigns.PredictionClaimed:
		return serverpackets.SystemMessageSealNotOwned35MoreVoted
	case sevensigns.PredictionNotClaimed:
		return serverpackets.SystemMessageSealNotOwned35LessVoted
	default:
		return serverpackets.SystemMessageCompetitionTieSealNotAwarded
	}
}

// useSevenSignsRecords opens the first page of live's Record of Seven Signs.
func (l *GameClientLink) useSevenSignsRecords(live *livePlayer) {
	if l.sevenSigns == nil {
		live.SendFrame(serverpackets.FrameActionFailed())
		return
	}
	live.SendFrame(l.ssqStatusFrame(live.ObjectID(), ssqPageRecord))
}

// ssqSkyFrame builds the SSQInfo packet showing the sky the Seven Signs
// state calls for.
func ssqSkyFrame(state *sevensigns.State) wire.Frame {
	if state == nil {
		return serverpackets.FrameSSQInfo()
	}
	return serverpackets.FrameSSQInfoSky(ssqSky(state.Sky()))
}

func ssqSky(c sevensigns.Cabal) uint16 {
	switch c {
	case sevensigns.Dawn:
		return serverpackets.SSQSkyDawn
	case sevensigns.Dusk:
		return serverpackets.SSQSkyDusk
	default:
		return serverpackets.SSQSkyRegular
	}
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
		sky := ssqSky(n.Cabal)
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
