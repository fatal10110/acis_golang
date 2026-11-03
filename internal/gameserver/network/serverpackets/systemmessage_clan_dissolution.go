package serverpackets

// Clan dissolution system message ids.
const (
	SystemMessageCannotDissolveSiegeRegistered = 13  // no parameter
	SystemMessageClanHasDispersed              = 193 // no parameter
	SystemMessageDissolutionInProgress         = 263 // no parameter
	SystemMessageCannotDissolveWhileInWar      = 264 // no parameter
	SystemMessageCannotDissolveWhileInSiege    = 265 // no parameter
	SystemMessageCannotDissolveOwningResidence = 266 // no parameter
	SystemMessageNoRequestsToDisperse          = 267 // no parameter
	SystemMessageCannotDisperseClansInAlly     = 554 // no parameter
)
