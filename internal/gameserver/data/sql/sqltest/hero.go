package sqltest

// heroesSchema mirrors the shipped heroes table definition verbatim.
const heroesSchema = "CREATE TABLE IF NOT EXISTS `heroes` (\n" +
	"  `char_id` decimal(11,0) NOT NULL default '0',\n" +
	"  `class_id` decimal(3,0) NOT NULL default '0',\n" +
	"  `count` decimal(3,0) NOT NULL default '0',\n" +
	"  `played` decimal(1,0) NOT NULL default '0',\n" +
	"  `active` tinyint NOT NULL default 0,\n" +
	"  `message` varchar(300) NOT NULL default '',\n" +
	"  PRIMARY KEY  (`char_id`)\n" +
	")"

// heroesDiarySchema mirrors the shipped heroes_diary table definition
// verbatim.
const heroesDiarySchema = "CREATE TABLE IF NOT EXISTS `heroes_diary` (\n" +
	"  `char_id` int(10) unsigned NOT NULL,\n" +
	"  `time` bigint(13) unsigned NOT NULL DEFAULT '0',\n" +
	"  `action` tinyint(2) unsigned NOT NULL default '0',\n" +
	"  `param` int(11) unsigned NOT NULL default '0',\n" +
	"  KEY `char_id` (`char_id`)\n" +
	")"
