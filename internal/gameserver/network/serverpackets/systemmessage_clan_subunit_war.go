package serverpackets

// Clan war and sub-unit system message ids.
const (
	SystemMessageYouHaveSurrenderedToS1Clan     = 218  // text parameter
	SystemMessageWarProclamationRefused         = 626  // no parameter
	SystemMessageAlreadyAtWarWithS1Wait5Days    = 628  // text parameter
	SystemMessageNotInvolvedInWar               = 636  // no parameter
	SystemMessageClanS1DeclaredWar              = 1561 // text parameter
	SystemMessageWarDeclaredAgainstS1           = 1562 // text parameter
	SystemMessageS1ClanCannotDeclareWarTooWeak  = 1563 // text parameter
	SystemMessageWarNeedsLevel3Or15Members      = 1564 // no parameter
	SystemMessageWarClanDoesNotExist            = 1565 // no parameter
	SystemMessageClanS1DecidedToStopWar         = 1566 // text parameter
	SystemMessageWarAgainstS1Stopped            = 1567 // text parameter
	SystemMessageWarAgainstAlliedClan           = 1569 // no parameter
	SystemMessageTooManyClanWars                = 1570 // no parameter
	SystemMessageWarAlreadyDeclared             = 1609 // no parameter
	SystemMessageCannotDeclareAgainstOwnClan    = 1610 // no parameter
	SystemMessageCannotStopWarInCombat          = 1677 // no parameter
	SystemMessageNoWarAgainstDissolvingClan     = 1684 // no parameter
	SystemMessageNotMeetCriteriaForAcademy      = 1730 // no parameter
	SystemMessageAcademyRequirements            = 1734 // no parameter
	SystemMessageS1NotMeetAcademyRequirements   = 1735 // text parameter
	SystemMessageClanAlreadyHasAcademy          = 1738 // no parameter
	SystemMessageS1ClanAcademyCreated           = 1741 // text parameter
	SystemMessageS2DesignatedApprenticeOfS1     = 1755 // two text parameters
	SystemMessageNoRightToDismissApprentice     = 1762 // no parameter
	SystemMessageS2ApprenticeOfS1Removed        = 1763 // two text parameters
	SystemMessageNotMeetCriteriaForMilitaryUnit = 1791 // no parameter
	SystemMessageS1SelectedAsCaptainOfS2        = 1793 // two text parameters
	SystemMessageKnightsOfS1Created             = 1794 // text parameter
	SystemMessageRoyalGuardOfS1Created          = 1795 // text parameter
	SystemMessageKnightCaptainCannotBeAppointed = 1850 // no parameter
	SystemMessageRoyalCaptainCannotBeAppointed  = 1851 // no parameter
	SystemMessageMilitaryUnitNameTaken          = 1855 // no parameter
)
