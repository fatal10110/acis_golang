package serverpackets

import (
	"github.com/fatal10110/acis_golang/internal/commons/wire"
	"github.com/fatal10110/acis_golang/internal/gameserver/sevensigns"
)

// OpcodeSSQStatus is the wire opcode for SSQStatus, one page of the Record
// of Seven Signs.
const OpcodeSSQStatus = 0xf5

// System messages the Record of Seven Signs and the Seven Signs period
// changes show.
const (
	SystemMessageQuestEventPeriod             = 1176
	SystemMessageValidationPeriod             = 1177
	SystemMessageInitialPeriod                = 1183
	SystemMessageResultsPeriod                = 1184
	SystemMessageQuestEventPeriodBegun        = 1210
	SystemMessageQuestEventPeriodEnded        = 1211
	SystemMessageDawnObtainedAvarice          = 1212
	SystemMessageDawnObtainedGnosis           = 1213
	SystemMessageDawnObtainedStrife           = 1214
	SystemMessageDuskObtainedAvarice          = 1215
	SystemMessageDuskObtainedGnosis           = 1216
	SystemMessageDuskObtainedStrife           = 1217
	SystemMessageSealValidationPeriodBegun    = 1218
	SystemMessageSealValidationPeriodEnded    = 1219
	SystemMessageDuskWon                      = 1240
	SystemMessageDawnWon                      = 1241
	SystemMessageUntilMonday6PM               = 1286
	SystemMessageUntilToday6PM                = 1287
	SystemMessageSealOwned10MoreVoted         = 1289
	SystemMessageSealNotOwned35MoreVoted      = 1290
	SystemMessageSealOwned10LessVoted         = 1291
	SystemMessageSealNotOwned35LessVoted      = 1292
	SystemMessageCompetitionTieSealNotAwarded = 1294
)

// Record of Seven Signs pages.
const (
	SSQPageRecord     = 1
	SSQPageFestival   = 2
	SSQPageSeals      = 3
	SSQPagePrediction = 4
)

// Seal-ownership thresholds the record's third page states, in percent.
const (
	ssqRetainPercent = 10
	ssqClaimPercent  = 35
)

func newSSQStatusWriter(page, period byte) *wire.Writer {
	w := newFrameWriter(OpcodeSSQStatus)
	w.WriteUint8(page)
	w.WriteUint8(period)
	return w
}

// FrameSSQStatus builds page of the Record of Seven Signs r: the calendar,
// the player's sign-up and both cabals' scores (SSQPageRecord), the seals'
// owners and votes (SSQPageSeals) or their predicted owners
// (SSQPagePrediction). Any other page carries only the page number and the
// active period; the festival page is FrameSSQStatusFestival.
func FrameSSQStatus(page byte, r sevensigns.Record) wire.Frame {
	w := newSSQStatusWriter(page, byte(r.Period))
	switch page {
	case SSQPageRecord:
		periodMsg, untilMsg := ssqPeriodMessages(r.Period)
		w.WriteInt32(int32(r.Cycle))
		w.WriteInt32(periodMsg)
		w.WriteInt32(untilMsg)
		w.WriteUint8(byte(r.PlayerCabal))
		w.WriteUint8(byte(r.PlayerSeal))
		w.WriteInt32(int32(r.PlayerStones))
		w.WriteInt32(int32(r.PlayerAncientAdena))
		for _, c := range [...]sevensigns.CabalStanding{r.Dusk, r.Dawn} {
			w.WriteInt32(int32(c.StoneProportion))
			w.WriteInt32(int32(c.FestivalScore))
			w.WriteInt32(int32(c.TotalScore))
			w.WriteUint8(byte(c.Percent))
		}
	case SSQPageSeals:
		w.WriteUint8(ssqRetainPercent)
		w.WriteUint8(ssqClaimPercent)
		w.WriteUint8(byte(len(r.Seals)))
		for _, s := range r.Seals {
			w.WriteUint8(byte(s.Seal))
			w.WriteUint8(byte(s.Owner))
			w.WriteUint8(byte(s.DuskPercent))
			w.WriteUint8(byte(s.DawnPercent))
		}
	case SSQPagePrediction:
		w.WriteUint8(byte(r.Winner))
		w.WriteUint8(byte(len(r.Seals)))
		for _, s := range r.Seals {
			w.WriteUint8(byte(s.Owner))
			w.WriteUint8(byte(s.Predicted))
			w.WriteUint16(ssqPredictionMessage(s.Prediction))
			w.WriteUint16(0)
		}
	}
	return wire.OwnedFrame(w.Frame(), w, releaseFrameWriter)
}

// ssqPeriodMessages names the active period and when it ends on the
// record's first page.
func ssqPeriodMessages(p sevensigns.Period) (period, until int32) {
	switch p {
	case sevensigns.Recruiting:
		return SystemMessageInitialPeriod, SystemMessageUntilToday6PM
	case sevensigns.Competition:
		return SystemMessageQuestEventPeriod, SystemMessageUntilMonday6PM
	case sevensigns.Results:
		return SystemMessageResultsPeriod, SystemMessageUntilToday6PM
	default:
		return SystemMessageValidationPeriod, SystemMessageUntilMonday6PM
	}
}

func ssqPredictionMessage(p sevensigns.Prediction) uint16 {
	switch p {
	case sevensigns.PredictionOwnedRetained:
		return SystemMessageSealOwned10MoreVoted
	case sevensigns.PredictionOwnedLost:
		return SystemMessageSealOwned10LessVoted
	case sevensigns.PredictionClaimed:
		return SystemMessageSealNotOwned35MoreVoted
	case sevensigns.PredictionNotClaimed:
		return SystemMessageSealNotOwned35LessVoted
	default:
		return SystemMessageCompetitionTieSealNotAwarded
	}
}
