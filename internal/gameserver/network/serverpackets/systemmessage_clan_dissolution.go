package serverpackets

// Clan dissolution system message ids.
const (
	SystemMessageClanHasDispersed              = 193 // no parameter
	SystemMessageDissolutionInProgress         = 263 // no parameter
	SystemMessageCannotDissolveWhileInWar      = 264 // no parameter
	SystemMessageCannotDissolveOwningResidence = 266 // no parameter
	SystemMessageNoRequestsToDisperse          = 267 // no parameter
	SystemMessageCannotDisperseClansInAlly     = 554 // no parameter
)
