package sqltest

// mdtBetsSchema and mdtHistorySchema mirror the shipped mdt_bets and
// mdt_history table definitions verbatim.
const mdtBetsSchema = "CREATE TABLE IF NOT EXISTS `mdt_bets` (\n" +
	"  `lane_id` INT(1) DEFAULT 0,\n" +
	"  `bet` INT DEFAULT 0,\n" +
	"  PRIMARY KEY (`lane_id`)\n" +
	")"

const mdtHistorySchema = "CREATE TABLE IF NOT EXISTS `mdt_history` (\n" +
	"  `race_id` MEDIUMINT DEFAULT 0,\n" +
	"  `first` INT(1) DEFAULT 0,\n" +
	"  `second` INT(1) DEFAULT 0,\n" +
	"  `odd_rate` DOUBLE(10,2) DEFAULT 0,\n" +
	"  PRIMARY KEY (`race_id`)\n" +
	")"

// mdtBetsSeed is the shipped mdt_bets content: one empty stake per lane.
const mdtBetsSeed = "INSERT INTO `mdt_bets` VALUES\n" +
	"('1','0'),\n" +
	"('2','0'),\n" +
	"('3','0'),\n" +
	"('4','0'),\n" +
	"('5','0'),\n" +
	"('6','0'),\n" +
	"('7','0'),\n" +
	"('8','0')"
