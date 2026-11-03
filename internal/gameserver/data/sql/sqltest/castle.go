package sqltest

// castleSchema mirrors the shipped castle table definition verbatim.
const castleSchema = "CREATE TABLE IF NOT EXISTS `castle` (\n" +
	"  `id` INT NOT NULL DEFAULT '0',\n" +
	"  `currentTaxPercent` INT NOT NULL DEFAULT '0',\n" +
	"  `nextTaxPercent` INT NOT NULL DEFAULT '0',\n" +
	"  `treasury` BIGINT NOT NULL DEFAULT '0',\n" +
	"  `taxRevenue` BIGINT NOT NULL DEFAULT '0',\n" +
	"  `seedIncome` BIGINT NOT NULL DEFAULT '0',\n" +
	"  `siegeDate` DECIMAL(20,0) NOT NULL DEFAULT '0',\n" +
	"  `regTimeOver` ENUM('true','false') DEFAULT 'true' NOT NULL,\n" +
	"  `certificates` SMALLINT NOT NULL DEFAULT '300',\n" +
	"  PRIMARY KEY (`id`)\n" +
	")"

// castleSeed seeds the nine castle rows the shipped schema inserts.
const castleSeed = "INSERT IGNORE INTO `castle` VALUES " +
	"(1,15,15,0,0,0,0,'true',300),(2,15,15,0,0,0,0,'true',300),(3,15,15,0,0,0,0,'true',300)," +
	"(4,15,15,0,0,0,0,'true',300),(5,15,15,0,0,0,0,'true',300),(6,15,15,0,0,0,0,'true',300)," +
	"(7,15,15,0,0,0,0,'true',300),(8,15,15,0,0,0,0,'true',300),(9,15,15,0,0,0,0,'true',300)"

// castleDoorUpgradeSchema and castleTrapUpgradeSchema mirror the shipped
// castle_doorupgrade and castle_trapupgrade table definitions verbatim.
const (
	castleDoorUpgradeSchema = "CREATE TABLE IF NOT EXISTS `castle_doorupgrade` (\n" +
		"  `doorId` INT NOT NULL DEFAULT 0,\n" +
		"  `hp` TINYINT NOT NULL DEFAULT 0,\n" +
		"  `castleId` TINYINT NOT NULL DEFAULT 0,\n" +
		"  PRIMARY KEY (`doorId`)\n" +
		")"
	castleTrapUpgradeSchema = "CREATE TABLE IF NOT EXISTS `castle_trapupgrade` (\n" +
		"  `castleId` tinyint(3) unsigned NOT NULL DEFAULT '0',\n" +
		"  `towerIndex` tinyint(3) unsigned NOT NULL DEFAULT '0',\n" +
		"  `level` tinyint(3) unsigned NOT NULL DEFAULT '0',\n" +
		"  PRIMARY KEY (`towerIndex`,`castleId`)\n" +
		")"
)
