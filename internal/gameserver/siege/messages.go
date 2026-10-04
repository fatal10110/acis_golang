package siege

// Message is a system message the sieges send: its id and, in this order,
// a clan name when Text is set, the castle's name when CastleID is set and
// a number when HasNumber is set.
type Message struct {
	ID        int
	Text      string
	CastleID  int
	Number    int32
	HasNumber bool
}

// The siege system message ids.
const (
	MsgEnteredCombatZone            = 283
	MsgLeftCombatZone               = 284
	MsgClanVictoriousOverSiege      = 291 // clan name, castle name
	MsgAnnouncedSiegeTime           = 292 // castle name
	MsgRegistrationTermEnded        = 293 // castle name
	MsgSiegeCanceledNoClans         = 295 // castle name
	MsgHoursUntilConclusion         = 358 // number
	MsgMinutesUntilConclusion       = 359 // number
	MsgSecondsLeft                  = 360 // number
	MsgCantAcceptAllyEnemy          = 509
	MsgAlreadyRequested             = 638
	MsgOnlyClanLevel4               = 645
	MsgAttackerSideFull             = 648
	MsgDefenderSideFull             = 649
	MsgOwnerAutomaticallyDefends    = 688
	MsgOwnerCannotJoinOther         = 689
	MsgCannotAttackAllianceCastle   = 690
	MsgSiegeStarted                 = 711 // castle name
	MsgSiegeEnded                   = 712 // castle name
	MsgDeadlinePassed               = 845 // castle name
	MsgSiegeCanceledNoInterest      = 846 // castle name
	MsgSiegeDraw                    = 856 // castle name
	MsgDissolutionInProgress        = 1114
	MsgTemporaryAlliance            = 1189
	MsgTemporaryAllianceDissolved   = 1190
	MsgClanDefeatedLostReputation   = 1772 // number
	MsgClanVictoriousGainReputation = 1773 // number
)

// The sounds every player hears as a siege starts and ends.
const (
	SoundSiegeStarted = "systemmsg_e.17"
	SoundSiegeEnded   = "systemmsg_e.18"
)

func castleMsg(id, castleID int) Message { return Message{ID: id, CastleID: castleID} }

func numberMsg(id int, n int32) Message { return Message{ID: id, Number: n, HasNumber: true} }
