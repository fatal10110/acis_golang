package serverpackets

// Clan system message ids.
const (
	SystemMessageS1DoesNotExist                     = 2    // text parameter
	SystemMessageCannotInviteYourself               = 4    // no parameter
	SystemMessageS1AlreadyExists                    = 5    // text parameter
	SystemMessageS1WorkingWithAnotherClan           = 10   // text parameter
	SystemMessageS1HasInvitedYouToJoinTheClanS2     = 67   // two text parameters
	SystemMessageInvitedUserNotOnline               = 161  // no parameter
	SystemMessageClanCreated                        = 189  // no parameter
	SystemMessageFailedToCreateClan                 = 190  // no parameter
	SystemMessageClanMemberS1Expelled               = 191  // text parameter
	SystemMessageEnteredTheClan                     = 195  // no parameter
	SystemMessageYouHaveWithdrawnFromClan           = 197  // no parameter
	SystemMessageClanMembershipTerminated           = 199  // no parameter
	SystemMessageS1HasJoinedClan                    = 222  // text parameter
	SystemMessageS1HasWithdrawnFromTheClan          = 223  // text parameter
	SystemMessageS1DidNotRespondToClanInvitation    = 224  // text parameter
	SystemMessageYouDidNotRespondToS1ClanInvitation = 225  // text parameter
	SystemMessageNotMeetCriteriaToCreateClan        = 229  // no parameter
	SystemMessageMustWaitBeforeCreatingClan         = 230  // no parameter
	SystemMessageMustWaitBeforeAcceptingNewMember   = 231  // no parameter
	SystemMessageMustWaitBeforeJoiningAnotherClan   = 232  // no parameter
	SystemMessageSubclanIsFull                      = 233  // no parameter
	SystemMessageClanLeaderCannotWithdraw           = 239  // no parameter
	SystemMessageClanNameInvalid                    = 261  // no parameter
	SystemMessageClanNameLengthIncorrect            = 262  // no parameter
	SystemMessageCannotDismissYourself              = 269  // no parameter
	SystemMessageClanLevelIncreased                 = 274  // no parameter
	SystemMessageClanMemberS1LoggedIn               = 304  // text parameter
	SystemMessageSucceededInExpellingClanMember     = 309  // no parameter
	SystemMessageCannotRiseLevelWhileDissolving     = 551  // no parameter
	SystemMessageS1MustWaitBeforeJoiningAnotherClan = 760  // text parameter
	SystemMessageCannotLeaveDuringCombat            = 1116 // no parameter
	SystemMessageClanMemberCannotBeDismissedCombat  = 1117 // no parameter
	SystemMessageClanMemberS1PrivilegeChangedToS2   = 1761 // text then number parameter
	SystemMessageClanCanAccumulateReputation        = 1771 // no parameter
	SystemMessageS1DeductedFromClanRep              = 1787 // number parameter
	SystemMessageReputationLowClanSkillsDeactivated = 1789 // no parameter
	SystemMessageFailedToIncreaseClanLevel          = 1790 // no parameter
	SystemMessageS1ClanIsFull                       = 1835 // text parameter
	SystemMessageClanSkillsActivatedReputation      = 1862 // no parameter
)
