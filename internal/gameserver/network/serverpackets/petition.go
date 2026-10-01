package serverpackets

import "github.com/fatal10110/acis_golang/internal/commons/wire"

// OpcodePetitionVote is the wire opcode for PetitionVote.
const OpcodePetitionVote = 0xf6

// FramePetitionVote builds PetitionVote, which opens the petitioner's
// feedback window. It has no body.
func FramePetitionVote() wire.Frame {
	w := newFrameWriter(OpcodePetitionVote)
	return wire.OwnedFrame(w.Frame(), w, releaseFrameWriter)
}

// Petition system messages.
const (
	SystemMessageGameClientUnableToConnectToPetitionServer = 381 // no parameter
	SystemMessageClientNotLoggedOntoGameServer             = 384 // no parameter
	SystemMessageThisEndThePetitionPleaseProvideFeedback   = 387 // no parameter
	SystemMessageNotUnderPetitionConsultation              = 388 // no parameter
	SystemMessagePetitionAcceptedRecentNoS1                = 389 // number parameter
	SystemMessageOnlyOneActivePetitionAtTime               = 390 // no parameter
	SystemMessageReceiptNoS1Canceled                       = 391 // number parameter
	SystemMessageFailedCancelPetitionTryLater              = 393 // no parameter
	SystemMessagePetitionWithS1UnderWay                    = 394 // text parameter
	SystemMessagePetitionEndedWithS1                       = 395 // text parameter
	SystemMessagePetitionAppAccepted                       = 406 // no parameter
	SystemMessagePetitionUnderProcess                      = 407 // no parameter
	SystemMessageS1PetitionOnWaitingList                   = 601 // number parameter
	SystemMessagePetitionSystemCurrentUnavailable          = 602 // no parameter
	SystemMessageSubmittedYourS1ThPetitionS2Left           = 730 // two number parameters
	SystemMessagePetitionS1ReceivedCodeIsS2                = 731 // text, number parameters
	SystemMessageS1ReceivedConsultationRequest             = 732 // text parameter
	SystemMessageWeHaveReceivedS1PetitionsToday            = 733 // number parameter
	SystemMessagePetitionFailedS1AlreadySubmitted          = 734 // text parameter
	SystemMessagePetitionFailedForS1ErrorNumberS2          = 735 // text, number parameters
	SystemMessagePetitionCanceledSubmitS1MoreToday         = 736 // number parameter
	SystemMessagePetitionNotSubmitted                      = 738 // no parameter
	SystemMessageS1ParticipatePetition                     = 740 // text parameter
	SystemMessageFailedAddingS1ToPetition                  = 741 // text parameter
	SystemMessagePetitionAddingS1FailedErrorNumberS2       = 742 // text, number parameters
	SystemMessageS1LeftPetitionChat                        = 743 // text parameter
	SystemMessageYouAreNotInPetitionChat                   = 745 // no parameter
	SystemMessagePetitionMaxChars255                       = 971 // no parameter
)
