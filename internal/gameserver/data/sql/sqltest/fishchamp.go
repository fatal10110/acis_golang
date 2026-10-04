package sqltest

// fishingChampionshipSchema mirrors the shipped fishing_championship table
// definition verbatim.
const fishingChampionshipSchema = "CREATE TABLE IF NOT EXISTS `fishing_championship` (\n" +
	"  `player_name` VARCHAR(35) NOT NULL,\n" +
	"  `fish_length` DOUBLE(10,3) NOT NULL,\n" +
	"  `rewarded` INT(1) NOT NULL\n" +
	")"
