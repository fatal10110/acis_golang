package sqltest

// siegeClansSchema mirrors the shipped siege_clans table definition
// verbatim.
const siegeClansSchema = "CREATE TABLE IF NOT EXISTS `siege_clans` (\n" +
	"   `castle_id` TINYINT NOT NULL DEFAULT '0',\n" +
	"   `clan_id` INT(11) NOT NULL DEFAULT '0',\n" +
	"   `type` VARCHAR(8) DEFAULT 'PENDING',\n" +
	"   PRIMARY KEY (`castle_id`, `clan_id`)\n" +
	")"
