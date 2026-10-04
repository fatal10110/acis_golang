package sqltest

// sevenSignsFestivalSchema mirrors the shipped seven_signs_festival table
// definition verbatim.
const sevenSignsFestivalSchema = "CREATE TABLE IF NOT EXISTS `seven_signs_festival` (\n" +
	"`festivalId` int(1) NOT NULL DEFAULT '0',\n" +
	"`cabal` varchar(4) NOT NULL DEFAULT '',\n" +
	"`cycle` int(4) NOT NULL DEFAULT '0',\n" +
	"`date` bigint(50) DEFAULT '0',\n" +
	"`score` int(5) NOT NULL DEFAULT '0',\n" +
	"`members` varchar(255) NOT NULL DEFAULT '',\n" +
	"PRIMARY KEY (`festivalId`,`cabal`,`cycle`)\n" +
	")"

// sevenSignsFestivalSeed seeds the first cycle's blank scores, matching the
// shipped schema seed.
const sevenSignsFestivalSeed = "INSERT IGNORE INTO `seven_signs_festival` VALUES " +
	`(0, "DAWN", 1, 0, 0, ""),` +
	`(1, "DAWN", 1, 0, 0, ""),` +
	`(2, "DAWN", 1, 0, 0, ""),` +
	`(3, "DAWN", 1, 0, 0, ""),` +
	`(4, "DAWN", 1, 0, 0, ""),` +
	`(0, "DUSK", 1, 0, 0, ""),` +
	`(1, "DUSK", 1, 0, 0, ""),` +
	`(2, "DUSK", 1, 0, 0, ""),` +
	`(3, "DUSK", 1, 0, 0, ""),` +
	`(4, "DUSK", 1, 0, 0, "")`
