package serverpackets

// System messages of the community board and its mail.
const (
	SystemMessageOnlyClanLeaderEnabled  = 236  // no parameter
	SystemMessageCommunityBoardOffline  = 938  // no parameter
	SystemMessageNoCommunityBoardInClan = 1050 // no parameter
	SystemMessageMailboxFull            = 1205 // no parameter
	SystemMessageS1BlockedYouCannotMail = 1228 // text parameter
	SystemMessageNoMoreMessagesToday    = 1229 // no parameter
	SystemMessageOnlyFiveRecipients     = 1230 // no parameter
	SystemMessageSentMail               = 1231 // no parameter
	SystemMessageMessageNotSent         = 1232 // no parameter
	SystemMessageNewMail                = 1233 // no parameter
	SystemMessageCannotMailGMS1         = 1370 // text parameter
)

// SoundNewMail is the sound a new mail plays.
const SoundNewMail = "systemmsg_e.1233"
