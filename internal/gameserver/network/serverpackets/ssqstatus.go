package serverpackets

import "github.com/fatal10110/acis_golang/internal/commons/wire"

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

// SSQCabalStanding is one cabal's scores on the record's first page.
type SSQCabalStanding struct {
	StoneScore    int32
	FestivalScore int32
	TotalScore    int32
	Percent       byte
}

// SSQRecord is the record's first page: the calendar, the player's own
// sign-up, and both cabals' scores.
type SSQRecord struct {
	Cycle         int32
	PeriodMessage int32
	UntilMessage  int32
	PlayerCabal   byte
	PlayerSeal    byte
	PlayerStones  int32
	PlayerAdena   int32
	Dusk          SSQCabalStanding
	Dawn          SSQCabalStanding
}

// SSQSealVotes is one seal's owner and the percent of each cabal's members
// who chose it, on the record's third page.
type SSQSealVotes struct {
	Seal        byte
	Owner       byte
	DuskPercent byte
	DawnPercent byte
}

// SSQSealPrediction is one seal's owner, predicted owner, and the message
// explaining it, on the record's fourth page.
type SSQSealPrediction struct {
	Owner     byte
	Predicted byte
	Message   uint16
}

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

// FrameSSQStatusHeader builds an SSQStatus page with no content: the page
// number and the active period only.
func FrameSSQStatusHeader(page, period byte) wire.Frame {
	w := newSSQStatusWriter(page, period)
	return wire.OwnedFrame(w.Frame(), w, releaseFrameWriter)
}

// FrameSSQStatusRecord builds SSQStatus page 1.
func FrameSSQStatusRecord(period byte, r SSQRecord) wire.Frame {
	w := newSSQStatusWriter(1, period)
	w.WriteInt32(r.Cycle)
	w.WriteInt32(r.PeriodMessage)
	w.WriteInt32(r.UntilMessage)
	w.WriteUint8(r.PlayerCabal)
	w.WriteUint8(r.PlayerSeal)
	w.WriteInt32(r.PlayerStones)
	w.WriteInt32(r.PlayerAdena)
	for _, c := range [...]SSQCabalStanding{r.Dusk, r.Dawn} {
		w.WriteInt32(c.StoneScore)
		w.WriteInt32(c.FestivalScore)
		w.WriteInt32(c.TotalScore)
		w.WriteUint8(c.Percent)
	}
	return wire.OwnedFrame(w.Frame(), w, releaseFrameWriter)
}

// FrameSSQStatusSeals builds SSQStatus page 3.
func FrameSSQStatusSeals(period byte, seals []SSQSealVotes) wire.Frame {
	w := newSSQStatusWriter(3, period)
	w.WriteUint8(ssqRetainPercent)
	w.WriteUint8(ssqClaimPercent)
	w.WriteUint8(byte(len(seals)))
	for _, s := range seals {
		w.WriteUint8(s.Seal)
		w.WriteUint8(s.Owner)
		w.WriteUint8(s.DuskPercent)
		w.WriteUint8(s.DawnPercent)
	}
	return wire.OwnedFrame(w.Frame(), w, releaseFrameWriter)
}

// FrameSSQStatusPrediction builds SSQStatus page 4.
func FrameSSQStatusPrediction(period, winner byte, seals []SSQSealPrediction) wire.Frame {
	w := newSSQStatusWriter(4, period)
	w.WriteUint8(winner)
	w.WriteUint8(byte(len(seals)))
	for _, s := range seals {
		w.WriteUint8(s.Owner)
		w.WriteUint8(s.Predicted)
		w.WriteUint16(s.Message)
		w.WriteUint16(0)
	}
	return wire.OwnedFrame(w.Frame(), w, releaseFrameWriter)
}
