package sqltest

// characterRaidPointsSchema mirrors the shipped character_raid_points table
// definition verbatim.
const characterRaidPointsSchema = "CREATE TABLE IF NOT EXISTS `character_raid_points` (\n" +
	"  `char_id` INT UNSIGNED NOT NULL DEFAULT 0,\n" +
	"  `boss_id` INT UNSIGNED NOT NULL DEFAULT 0,\n" +
	"  `points` INT UNSIGNED NOT NULL DEFAULT 0,\n" +
	"  PRIMARY KEY (`char_id`,`boss_id`)\n" +
	")"

// grandbossListSchema mirrors the shipped grandboss_list table definition
// verbatim.
const grandbossListSchema = "CREATE TABLE IF NOT EXISTS `grandboss_list` (\n" +
	"  `player_id` decimal(11,0) NOT NULL,\n" +
	"  `zone` decimal(11,0) NOT NULL,\n" +
	"  PRIMARY KEY (`player_id`,`zone`)\n" +
	")"
