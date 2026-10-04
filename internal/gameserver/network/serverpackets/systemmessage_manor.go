package serverpackets

// System messages of the manor items and the sow and harvest skills.
// SystemMessageHarvestFailedSeedNotSown (893) is declared with the corpse
// target failures.
const (
	// SystemMessageSeedHasBeenSown: the target was already sown.
	SystemMessageSeedHasBeenSown = 871
	// SystemMessageSeedMayNotBeSownHere: the seed belongs to another
	// castle's manor than the target's area.
	SystemMessageSeedMayNotBeSownHere = 872
	// SystemMessageSeedSuccessfullySown: the seed took.
	SystemMessageSeedSuccessfullySown = 889
	// SystemMessageSeedNotSown: the sow roll failed.
	SystemMessageSeedNotSown = 890
	// SystemMessageNotAuthorizedToHarvest: the harvester is neither the
	// sower nor in the sower's party.
	SystemMessageNotAuthorizedToHarvest = 891
	// SystemMessageHarvestHasFailed: the crop was already taken, or the
	// harvest roll failed.
	SystemMessageHarvestHasFailed = 892
	// SystemMessageS1HarvestedS3S2 names the harvesting member, the crop
	// and its count.
	SystemMessageS1HarvestedS3S2 = 1137
	// SystemMessageS1HarvestedS2 names the harvesting member and the crop.
	SystemMessageS1HarvestedS2 = 1138
	// SystemMessageTargetUnavailableForSeeding: the target is no seedable
	// monster spawned in a manor area.
	SystemMessageTargetUnavailableForSeeding = 1516
)
