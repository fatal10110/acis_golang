package serverpackets

// Clan hall auction and lease system message ids.
const (
	SystemMessageAuctionOnlyClanLevel2Higher     = 673  // no parameter
	SystemMessageNoClanHallsUpForAuction         = 675  // no parameter
	SystemMessageAlreadySubmittedBid             = 676  // no parameter
	SystemMessageBidPriceMustBeHigher            = 677  // no parameter
	SystemMessageCanceledBid                     = 679  // no parameter
	SystemMessageCannotParticipateInAuction      = 680  // no parameter
	SystemMessageClanHallAwardedToClanS1         = 776  // string: the winning clan
	SystemMessageClanHallNotSold                 = 777  // no parameter
	SystemMessageRegisteredForClanHall           = 1004 // no parameter
	SystemMessageNotEnoughAdenaInClanWarehouse   = 1005 // no parameter
	SystemMessageBidInClanHallAuction            = 1006 // no parameter
	SystemMessageClanHallPaymentDueTomorrowS1    = 1051 // number: the lease
	SystemMessageClanHallFeeOverdueOwnershipLost = 1052 // no parameter
	SystemMessageNoOfferingsOwnOrMadeBidFor      = 1883 // no parameter
)
