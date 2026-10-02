package serverpackets

// Alliance system message ids.
const (
	SystemMessageS1IsNotAClanLeader               = 9   // text: the player's name
	SystemMessageTargetMustBeInClan               = 234 // no parameter
	SystemMessageFeatureOnlyForAllianceLeader     = 464 // no parameter
	SystemMessageNoCurrentAlliances               = 465 // no parameter
	SystemMessageYouHaveExceededTheLimit          = 466 // no parameter
	SystemMessageCantInviteClanWithin1Day         = 467 // no parameter
	SystemMessageCantEnterAllianceWithin1Day      = 468 // no parameter
	SystemMessageMayNotAllyClanBattle             = 469 // no parameter
	SystemMessageOnlyClanLeaderWithdrawAlly       = 470 // no parameter
	SystemMessageAllianceLeaderCantWithdraw       = 471 // no parameter
	SystemMessageDifferentAlliance                = 473 // no parameter
	SystemMessageClanDoesntExist                  = 474 // no parameter
	SystemMessageNoResponseToAllyInvitation       = 477 // no parameter
	SystemMessageYouDidNotRespondToAllyInvitation = 478 // no parameter
	SystemMessageAllianceInfoHead                 = 491 // no parameter
	SystemMessageAllianceNameS1                   = 492 // text: the alliance's name
	SystemMessageConnectionS1TotalS2              = 493 // numbers: online, total
	SystemMessageAllianceLeaderS2OfS1             = 494 // text: the leading clan, its leader
	SystemMessageAllianceClanTotalS1              = 495 // number: clans
	SystemMessageClanInfoHead                     = 496 // no parameter
	SystemMessageClanInfoNameS1                   = 497 // text: the clan's name
	SystemMessageClanInfoLeaderS1                 = 498 // text: the clan leader's name
	SystemMessageClanInfoLevelS1                  = 499 // number: the clan's level
	SystemMessageClanInfoSeparator                = 500 // no parameter
	SystemMessageClanInfoFoot                     = 501 // no parameter
	SystemMessageAlreadyJoinedAlliance            = 502 // no parameter
	SystemMessageOnlyClanLeaderCreateAlliance     = 504 // no parameter
	SystemMessageCantCreateAlliance10DaysDissolve = 505 // no parameter
	SystemMessageIncorrectAllianceName            = 506 // no parameter
	SystemMessageIncorrectAllianceNameLength      = 507 // no parameter
	SystemMessageAllianceAlreadyExists            = 508 // no parameter
	SystemMessageYouAcceptedAlliance              = 517 // no parameter
	SystemMessageYouHaveWithdrawnFromAlliance     = 519 // no parameter
	SystemMessageYouHaveExpelledAClan             = 521 // no parameter
	SystemMessageAllianceDissolved                = 523 // no parameter
	SystemMessageS2AllianceLeaderOfS1Requested    = 527 // text: the alliance's name, the inviter's name
	SystemMessageCreateAllyClanLevel5Needed       = 549 // no parameter
	SystemMessageCannotCreateAllyWhileDissolving  = 550 // no parameter
	SystemMessageS1ClanAlreadyMemberOfS2Alliance  = 691 // text: the clan's name, its alliance's name
	SystemMessageS1CantEnterAllianceWithin1Day    = 761 // text: the clan's name
)
