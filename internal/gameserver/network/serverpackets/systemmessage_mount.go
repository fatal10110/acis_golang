package serverpackets

// Pet mount refusal system message ids, none with a parameter.
const (
	SystemMessageStriderCantBeRiddenWhileDead        = 1009
	SystemMessageDeadStriderCantBeRidden             = 1010
	SystemMessageStriderInBattleCantBeRidden         = 1011
	SystemMessageStriderCantBeRiddenWhileInBattle    = 1012
	SystemMessageStriderCanBeRiddenOnlyWhileStanding = 1013
	SystemMessageTooFarAwayFromStriderToMount        = 1846
)
