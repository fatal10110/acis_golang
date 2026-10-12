package serverpackets

// Mercenary ticket system message ids.
const (
	SystemMessagePlaceS1InCurrentLocationAndDirection           = 651  // item name: the ticket placed
	SystemMessageNoAuthorityToPositionMercenaries               = 653  // no parameter
	SystemMessageMercenariesCannotBePositionedHere              = 655  // no parameter
	SystemMessageMercenaryCannotBePositionedAnymore             = 656  // no parameter
	SystemMessageMercenariesTooCloseToPosition                  = 657  // no parameter
	SystemMessageNotMercenaryOfOwnCastleCannotCancelPositioning = 658  // no parameter
	SystemMessageMercCanBeAssigned                              = 1194 // no parameter
	SystemMessageMercCantBeAssignedUsingStrife                  = 1195 // no parameter
)
