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
)
