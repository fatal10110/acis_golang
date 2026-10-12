package serverpackets

// System messages of the manor cycle.
const (
	// SystemMessageManorInformationUpdated tells a castle owner's clan
	// leader the manor period rolled over.
	SystemMessageManorInformationUpdated = 884
)

// System messages of the manor windows.
const (
	// SystemMessageManorUnderMaintenance turns a player away from the
	// manor windows while the period rolls over.
	SystemMessageManorUnderMaintenance = 879
	// SystemMessageManorCannotBeSetUp refuses the seed and crop setting
	// windows while the next period's lists are approved.
	SystemMessageManorCannotBeSetUp = 1675
)
