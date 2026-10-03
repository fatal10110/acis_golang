package sqltest

// olympiadNoblesSchema mirrors the shipped olympiad_nobles table definition
// verbatim.
const olympiadNoblesSchema = "CREATE TABLE IF NOT EXISTS `olympiad_nobles` (\n" +
	"  `char_id` INT UNSIGNED NOT NULL default 0,\n" +
	"  `class_id` tinyint(3) unsigned NOT NULL default 0,\n" +
	"  `olympiad_points` int(10) NOT NULL default 0,\n" +
	"  `competitions_done` smallint(3) NOT NULL default 0,\n" +
	"  `competitions_won` smallint(3) NOT NULL default 0,\n" +
	"  `competitions_lost` smallint(3) NOT NULL default 0,\n" +
	"  `competitions_drawn` smallint(3) NOT NULL default 0,\n" +
	"  `rewarded` smallint(1) NOT NULL default 0,\n" +
	"  PRIMARY KEY (`char_id`)\n" +
	")"

// olympiadNoblesEomSchema mirrors the shipped olympiad_nobles_eom table
// definition verbatim.
const olympiadNoblesEomSchema = "CREATE TABLE IF NOT EXISTS `olympiad_nobles_eom` (\n" +
	"  `char_id` INT UNSIGNED NOT NULL default 0,\n" +
	"  `class_id` tinyint(3) unsigned NOT NULL default 0,\n" +
	"  `olympiad_points` int(10) NOT NULL default 0,\n" +
	"  `competitions_done` smallint(3) NOT NULL default 0,\n" +
	"  `competitions_won` smallint(3) NOT NULL default 0,\n" +
	"  `competitions_lost` smallint(3) NOT NULL default 0,\n" +
	"  `competitions_drawn` smallint(3) NOT NULL default 0,\n" +
	"  PRIMARY KEY (`char_id`)\n" +
	")"

// serverMemoSchema mirrors the shipped server_memo table definition
// verbatim.
const serverMemoSchema = "CREATE TABLE IF NOT EXISTS `server_memo` (\n" +
	"  `var` VARCHAR(20) NOT NULL DEFAULT '',\n" +
	"  `value` VARCHAR(255) NOT NULL DEFAULT '',\n" +
	"  PRIMARY KEY (`var`)\n" +
	")"
