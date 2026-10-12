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

// System messages of the manor trade windows.
const (
	// SystemMessageFailedInTradingS2OfCropS1 names a crop row a sale
	// skipped: the crop (item name) and its count (item number).
	SystemMessageFailedInTradingS2OfCropS1 = 1491
	// SystemMessageHereYouCanBuyOnlySeedsOfS1Manor refuses another castle's
	// seed purchase window, naming the manor manager's castle (text).
	SystemMessageHereYouCanBuyOnlySeedsOfS1Manor = 1605
)
