package serverpackets

// Olympiad and noblesse system message ids.
const (
	SystemMessageOlympiadPeriodS1HasStarted                 = 1639 // number parameter: the cycle
	SystemMessageOlympiadPeriodS1HasEnded                   = 1640 // number parameter: the cycle
	SystemMessageTheOlympiadGameHasStarted                  = 1641 // no parameter
	SystemMessageTheOlympiadGameHasEnded                    = 1642 // no parameter
	SystemMessageOlympiadRecordS1MatchesS2WinsS3Defeats     = 1673 // four numbers: matches, wins, defeats, points
	SystemMessageNoblesseOnly                               = 1674 // no parameter
	SystemMessageOnlyNoblesseLeaderCanViewSiegeStatusWindow = 1694 // no parameter
	SystemMessageOnlyDuringSiege                            = 1695 // no parameter
	SystemMessageOlympiadRegistrationPeriodEnded            = 1919 // no parameter

	SystemMessageCantJoinOlympiadWithSubJob             = 1500 // no parameter
	SystemMessageOnlyNoblessCanParticipateInOlympiad    = 1501 // no parameter
	SystemMessageAlreadyRegisteredInEventWaitingList    = 1502 // no parameter
	SystemMessageRegisteredInClassifiedGamesWaitingList = 1503 // no parameter
	SystemMessageRegisteredInNoClassGamesWaitingList    = 1504 // no parameter
	SystemMessageDeletedFromGameWaitingList             = 1505 // no parameter
	SystemMessageNotRegisteredInGameWaitingList         = 1506 // no parameter
	SystemMessageOlympiadGameNotInProgress              = 1651 // no parameter
	SystemMessageAlreadyOnClassWaitingList              = 1689 // no parameter
	SystemMessageAlreadyOnAllClassesWaitingList         = 1690 // no parameter
	SystemMessageInventoryTooFullForOlympiad            = 1691 // no parameter
	SystemMessageCannotJoinOlympiadPossessingS1         = 1750 // item name: the cursed weapon
	SystemMessageGameRequestCannotBeMade                = 1803 // no parameter
)
