package sqltest

// cursedWeaponsSchema mirrors the shipped cursed_weapons table definition
// verbatim.
const cursedWeaponsSchema = "CREATE TABLE IF NOT EXISTS `cursed_weapons` (\n" +
	"  `itemId` INT,\n" +
	"  `playerId` INT DEFAULT 0,\n" +
	"  `playerKarma` INT DEFAULT 0,\n" +
	"  `playerPkKills` INT DEFAULT 0,\n" +
	"  `nbKills` INT DEFAULT 0,\n" +
	"  `currentStage` INT DEFAULT 0,\n" +
	"  `numberBeforeNextStage` INT DEFAULT 0,\n" +
	"  `hungryTime` INT DEFAULT 0,\n" +
	"  `endTime` DECIMAL(20,0) DEFAULT 0,\n" +
	"  PRIMARY KEY (`itemId`)\n" +
	")"
