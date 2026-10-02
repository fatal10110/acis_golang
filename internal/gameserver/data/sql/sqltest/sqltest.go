// Package sqltest gives integration tests across this module a database
// carrying the shipped gameserver schema, backed by the single shared
// MariaDB instance from internal/dbtest rather than a container per package.
package sqltest

import (
	"database/sql"
	"testing"

	"github.com/fatal10110/acis_golang/internal/dbtest"
	_ "github.com/go-sql-driver/mysql"
)

// charactersSchema mirrors the shipped characters table definition verbatim.
const charactersSchema = "CREATE TABLE IF NOT EXISTS characters (\n" +
	"	`account_name` VARCHAR(45) DEFAULT NULL,\n" +
	"	`obj_Id` INT UNSIGNED NOT NULL DEFAULT 0,\n" +
	"	`char_name` VARCHAR(35) NOT NULL,\n" +
	"	`level` TINYINT UNSIGNED DEFAULT NULL,\n" +
	"	`maxHp` MEDIUMINT UNSIGNED DEFAULT NULL,\n" +
	"	`curHp` MEDIUMINT UNSIGNED DEFAULT NULL,\n" +
	"	`maxCp` MEDIUMINT UNSIGNED DEFAULT NULL,\n" +
	"	`curCp` MEDIUMINT UNSIGNED DEFAULT NULL,\n" +
	"	`maxMp` MEDIUMINT UNSIGNED DEFAULT NULL,\n" +
	"	`curMp` MEDIUMINT UNSIGNED DEFAULT NULL,\n" +
	"	`face` TINYINT UNSIGNED DEFAULT NULL,\n" +
	"	`hairStyle` TINYINT UNSIGNED DEFAULT NULL,\n" +
	"	`hairColor` TINYINT UNSIGNED DEFAULT NULL,\n" +
	"	`sex` TINYINT UNSIGNED DEFAULT NULL,\n" +
	"	`heading` MEDIUMINT DEFAULT NULL,\n" +
	"	`x` MEDIUMINT DEFAULT NULL,\n" +
	"	`y` MEDIUMINT DEFAULT NULL,\n" +
	"	`z` MEDIUMINT DEFAULT NULL,\n" +
	"	`exp` BIGINT UNSIGNED DEFAULT 0,\n" +
	"	`expBeforeDeath` BIGINT UNSIGNED DEFAULT 0,\n" +
	"	`sp` INT UNSIGNED NOT NULL DEFAULT 0,\n" +
	"	`karma` INT UNSIGNED DEFAULT NULL,\n" +
	"	`pvpkills` SMALLINT UNSIGNED DEFAULT NULL,\n" +
	"	`pkkills` SMALLINT UNSIGNED DEFAULT NULL,\n" +
	"	`clanid` INT UNSIGNED DEFAULT NULL,\n" +
	"	`race` TINYINT UNSIGNED DEFAULT NULL,\n" +
	"	`classid` TINYINT UNSIGNED DEFAULT NULL,\n" +
	"	`base_class` TINYINT UNSIGNED NOT NULL DEFAULT 0,\n" +
	"	`deletetime` BIGINT DEFAULT NULL,\n" +
	"	`title` VARCHAR(16) DEFAULT NULL,\n" +
	"	`rec_have` TINYINT UNSIGNED NOT NULL DEFAULT 0,\n" +
	"	`rec_left` TINYINT UNSIGNED NOT NULL DEFAULT 0,\n" +
	"	`accesslevel` MEDIUMINT DEFAULT 0,\n" +
	"	`online` TINYINT UNSIGNED DEFAULT NULL,\n" +
	"	`onlinetime` INT DEFAULT NULL,\n" +
	"	`lastAccess` BIGINT UNSIGNED DEFAULT NULL,\n" +
	"	`wantspeace` TINYINT UNSIGNED DEFAULT 0,\n" +
	"	`isin7sdungeon` TINYINT UNSIGNED NOT NULL DEFAULT 0,\n" +
	"	`punish_level` TINYINT UNSIGNED NOT NULL DEFAULT 0,\n" +
	"	`punish_timer` BIGINT UNSIGNED NOT NULL DEFAULT 0,\n" +
	"	`power_grade` TINYINT UNSIGNED DEFAULT NULL,\n" +
	"	`nobless` TINYINT UNSIGNED NOT NULL DEFAULT 0,\n" +
	"	`hero` TINYINT UNSIGNED NOT NULL DEFAULT 0,\n" +
	"	`subpledge` SMALLINT NOT NULL DEFAULT 0,\n" +
	"	`lvl_joined_academy` TINYINT UNSIGNED NOT NULL DEFAULT 0,\n" +
	"	`apprentice` INT UNSIGNED NOT NULL DEFAULT 0,\n" +
	"	`sponsor` INT UNSIGNED NOT NULL DEFAULT 0,\n" +
	"	`varka_ketra_ally` TINYINT NOT NULL DEFAULT 0,\n" +
	"	`clan_join_expiry_time` BIGINT UNSIGNED NOT NULL DEFAULT 0,\n" +
	"	`clan_create_expiry_time` BIGINT UNSIGNED NOT NULL DEFAULT 0,\n" +
	"	`death_penalty_level` SMALLINT UNSIGNED NOT NULL DEFAULT 0,\n" +
	"	PRIMARY KEY (obj_Id),\n" +
	"	KEY `clanid` (`clanid`)\n" +
	")"

// itemsSchema mirrors the shipped items table definition verbatim.
const itemsSchema = "CREATE TABLE IF NOT EXISTS `items` (\n" +
	"	`owner_id` INT,\n" +
	"	`object_id` INT NOT NULL DEFAULT 0,\n" +
	"	`item_id` SMALLINT UNSIGNED NOT NULL,\n" +
	"	`count` INT UNSIGNED NOT NULL DEFAULT 0,\n" +
	"	`enchant_level` SMALLINT UNSIGNED NOT NULL DEFAULT 0,\n" +
	"	`loc` VARCHAR(10),\n" +
	"	`loc_data` INT,\n" +
	"	`custom_type1` INT NOT NULL DEFAULT 0,\n" +
	"	`custom_type2` INT NOT NULL DEFAULT 0,\n" +
	"	`mana_left` INT NOT NULL DEFAULT -1,\n" +
	"	`time` BIGINT NOT NULL DEFAULT 0,\n" +
	"	PRIMARY KEY (`object_id`)\n" +
	")"

// augmentationsSchema mirrors the shipped augmentations table definition
// verbatim.
const augmentationsSchema = "CREATE TABLE IF NOT EXISTS `augmentations` (\n" +
	"	`item_oid` INT UNSIGNED NOT NULL DEFAULT 0,\n" +
	"	`attributes` INT NOT NULL DEFAULT -1,\n" +
	"	`skill_id` INT NOT NULL DEFAULT -1,\n" +
	"	`skill_level` INT NOT NULL DEFAULT -1,\n" +
	"	PRIMARY KEY (`item_oid`)\n" +
	")"

// spawnDataSchema mirrors the shipped spawn_data table definition verbatim.
const spawnDataSchema = "CREATE TABLE IF NOT EXISTS `spawn_data` (\n" +
	"  `name` VARCHAR(80) NOT NULL,\n" +
	"  `status` SMALLINT NOT NULL,\n" +
	"  `current_hp` INT unsigned NOT NULL,\n" +
	"  `current_mp` INT unsigned NOT NULL,\n" +
	"  `loc_x` INT NOT NULL DEFAULT 0,\n" +
	"  `loc_y` INT NOT NULL DEFAULT 0,\n" +
	"  `loc_z` INT NOT NULL DEFAULT 0,\n" +
	"  `heading` MEDIUMINT NOT NULL DEFAULT 0,\n" +
	"  `db_value` SMALLINT NOT NULL DEFAULT 0,\n" +
	"  `respawn_time` BIGINT unsigned NOT NULL default 0,\n" +
	"  PRIMARY KEY (`name`)\n" +
	")"

// itemsOnGroundSchema mirrors the shipped items_on_ground table definition verbatim.
const itemsOnGroundSchema = "CREATE TABLE IF NOT EXISTS `items_on_ground` (\n" +
	"  `object_id` int(11) NOT NULL default '0',\n" +
	"  `item_id` int(11) default NULL,\n" +
	"  `count` int(11) default NULL,\n" +
	"  `enchant_level` int(11) default NULL,\n" +
	"  `x` int(11) default NULL,\n" +
	"  `y` int(11) default NULL,\n" +
	"  `z` int(11) default NULL,\n" +
	"  `time` decimal(20,0) default NULL,\n" +
	"  PRIMARY KEY  (`object_id`)\n" +
	")"

// characterSkillsSchema mirrors the shipped character_skills table
// definition verbatim.
const characterSkillsSchema = "CREATE TABLE IF NOT EXISTS `character_skills` (\n" +
	"  `char_obj_id` INT UNSIGNED NOT NULL default 0,\n" +
	"  `skill_id` INT NOT NULL default 0,\n" +
	"  `skill_level` INT(3) NOT NULL default 1,\n" +
	"  `class_index` INT(1) NOT NULL DEFAULT 0,\n" +
	"  PRIMARY KEY (`char_obj_id`,`skill_id`,`class_index`)\n" +
	")"

// characterShortcutsSchema mirrors the shipped character_shortcuts table
// definition verbatim.
const characterShortcutsSchema = "CREATE TABLE IF NOT EXISTS `character_shortcuts` (\n" +
	"  `char_obj_id` INT UNSIGNED NOT NULL DEFAULT 0,\n" +
	"  `slot` TINYINT UNSIGNED NOT NULL DEFAULT 0,\n" +
	"  `page` TINYINT UNSIGNED NOT NULL DEFAULT 0,\n" +
	"  `type` VARCHAR(6) NOT NULL DEFAULT 'NONE',\n" +
	"  `id` INT UNSIGNED NOT NULL DEFAULT 0,\n" +
	"  `level` SMALLINT SIGNED NOT NULL DEFAULT 0,\n" +
	"  `class_index` TINYINT UNSIGNED NOT NULL DEFAULT 0,\n" +
	"  PRIMARY KEY (`char_obj_id`,`slot`,`page`,`class_index`),\n" +
	"  KEY `id` (`id`)\n" +
	")"

// characterHennasSchema mirrors the shipped character_hennas table
// definition verbatim.
const characterHennasSchema = "CREATE TABLE IF NOT EXISTS `character_hennas` (\n" +
	"  `char_obj_id` INT NOT NULL DEFAULT 0,\n" +
	"  `symbol_id` INT,\n" +
	"  `slot` INT NOT NULL DEFAULT 0,\n" +
	"  `class_index` INT(1) NOT NULL DEFAULT 0,\n" +
	"  PRIMARY KEY (`char_obj_id`,`slot`,`class_index`)\n" +
	")"

// characterRecipeBookSchema mirrors the shipped character_recipebook table
// definition verbatim.
const characterRecipeBookSchema = "CREATE TABLE IF NOT EXISTS `character_recipebook` (\n" +
	"  `charId` INT UNSIGNED NOT NULL DEFAULT 0,\n" +
	"  `recipeId` SMALLINT NOT NULL DEFAULT 0,\n" +
	"  PRIMARY KEY (`charId`,`recipeId`)\n" +
	")"

// characterMacrosesSchema mirrors the shipped character_macroses table
// definition verbatim.
const characterMacrosesSchema = "CREATE TABLE IF NOT EXISTS `character_macroses` (\n" +
	"  `char_obj_id` INT NOT NULL DEFAULT 0,\n" +
	"  `id` INT NOT NULL DEFAULT 0,\n" +
	"  `icon` INT,\n" +
	"  `name` VARCHAR(40) ,\n" +
	"  `descr` VARCHAR(80) ,\n" +
	"  `acronym` VARCHAR(4) ,\n" +
	"  `commands` VARCHAR(255) ,\n" +
	"  PRIMARY KEY (`char_obj_id`,`id`)\n" +
	")"

// characterRecommendsSchema mirrors the shipped character_recommends table
// definition verbatim.
const characterRecommendsSchema = "CREATE TABLE IF NOT EXISTS character_recommends ( \n" +
	" char_id INT NOT NULL default 0, \n" +
	" target_id INT(11) NOT NULL DEFAULT 0, \n" +
	" PRIMARY KEY (char_id,target_id) \n" +
	")"

// characterSubclassesSchema mirrors the shipped character_subclasses table
// definition verbatim.
const characterSubclassesSchema = "CREATE TABLE IF NOT EXISTS `character_subclasses` (\n" +
	"`char_obj_id` decimal(11,0) NOT NULL default '0',\n" +
	"`class_id` int(2) NOT NULL default '0',\n" +
	"`exp` decimal(20,0) NOT NULL default '0',\n" +
	"`sp` decimal(11,0) NOT NULL default '0',\n" +
	"`level` int(2) NOT NULL default '40',\n" +
	"`class_index` int(1) NOT NULL default '0',\n" +
	"PRIMARY KEY  (`char_obj_id`,`class_id`)\n" +
	")"

// buylistsSchema mirrors the shipped buylists table definition verbatim.
const buylistsSchema = "CREATE TABLE IF NOT EXISTS `buylists` (\n" +
	"  `buylist_id` INT UNSIGNED,\n" +
	"  `item_id` INT UNSIGNED,\n" +
	"  `count` INT UNSIGNED NOT NULL DEFAULT 0,\n" +
	"  `next_restock_time` BIGINT UNSIGNED NOT NULL DEFAULT 0,\n" +
	"  PRIMARY KEY (`buylist_id`, `item_id`)\n" +
	")"

// petsSchema mirrors the shipped pets table definition verbatim.
const petsSchema = "CREATE TABLE IF NOT EXISTS `pets` (\n" +
	"  `item_obj_id` decimal(11) NOT NULL default 0,\n" +
	"  `name` varchar(16),\n" +
	"  `level` decimal(11),\n" +
	"  `curHp` decimal(18,0),\n" +
	"  `curMp` decimal(18,0),\n" +
	"  `exp` decimal(20, 0),\n" +
	"  `sp` decimal(11),\n" +
	"  `fed` decimal(11),\n" +
	"  PRIMARY KEY (`item_obj_id`)\n" +
	")"

// characterSkillsSaveSchema mirrors the shipped character_skills_save table
// definition verbatim.
const characterSkillsSaveSchema = "CREATE TABLE IF NOT EXISTS `character_skills_save` (\n" +
	"  `char_obj_id` INT NOT NULL default 0,\n" +
	"  `skill_id` INT NOT NULL default 0,\n" +
	"  `skill_level` INT(3) NOT NULL default 1,\n" +
	"  `effect_count` INT NOT NULL default 0,\n" +
	"  `effect_cur_time` INT NOT NULL default 0,\n" +
	"  `reuse_delay` INT(8) NOT NULL DEFAULT 0,\n" +
	"  `systime` BIGINT UNSIGNED NOT NULL DEFAULT 0,\n" +
	"  `restore_type` INT(1) NOT NULL DEFAULT 0,\n" +
	"  `class_index` INT(1) NOT NULL DEFAULT 0,\n" +
	"  `buff_index` INT(2) NOT NULL default 0,\n" +
	"  PRIMARY KEY (`char_obj_id`,`skill_id`,`skill_level`,`class_index`)\n" +
	")"

// sevenSignsStatusSchema mirrors the shipped seven_signs_status table
// definition verbatim, including its seeded status row.
const sevenSignsStatusSchema = "CREATE TABLE IF NOT EXISTS `seven_signs_status` (\n" +
	"  `id` int(3) NOT NULL default '0',\n" +
	"  `current_cycle` int(10) NOT NULL DEFAULT '1',\n" +
	"  `festival_cycle` int(10) NOT NULL DEFAULT '1',\n" +
	"  `active_period` VARCHAR(16) NOT NULL DEFAULT 'COMPETITION',\n" +
	"  `date` bigint(13) unsigned NOT NULL DEFAULT '0',\n" +
	"  `previous_winner` VARCHAR(8) NOT NULL DEFAULT 'NORMAL',\n" +
	"  `dawn_stone_score` DECIMAL(20,0) NOT NULL DEFAULT '0',\n" +
	"  `dawn_festival_score` int(10) NOT NULL DEFAULT '0',\n" +
	"  `dusk_stone_score` DECIMAL(20,0) NOT NULL DEFAULT '0',\n" +
	"  `dusk_festival_score` int(10) NOT NULL DEFAULT '0',\n" +
	"  `avarice_owner` VARCHAR(8) NOT NULL DEFAULT 'NORMAL',\n" +
	"  `gnosis_owner` VARCHAR(8) NOT NULL DEFAULT 'NORMAL',\n" +
	"  `strife_owner` VARCHAR(8) NOT NULL DEFAULT 'NORMAL',\n" +
	"  `avarice_dawn_score` int(10) NOT NULL DEFAULT '0',\n" +
	"  `gnosis_dawn_score` int(10) NOT NULL DEFAULT '0',\n" +
	"  `strife_dawn_score` int(10) NOT NULL DEFAULT '0',\n" +
	"  `avarice_dusk_score` int(10) NOT NULL DEFAULT '0',\n" +
	"  `gnosis_dusk_score` int(10) NOT NULL DEFAULT '0',\n" +
	"  `strife_dusk_score` int(10) NOT NULL DEFAULT '0',\n" +
	"  `accumulated_bonus0` int(10) NOT NULL DEFAULT '0',\n" +
	"  `accumulated_bonus1` int(10) NOT NULL DEFAULT '0',\n" +
	"  `accumulated_bonus2` int(10) NOT NULL DEFAULT '0',\n" +
	"  `accumulated_bonus3` int(10) NOT NULL DEFAULT '0',\n" +
	"  `accumulated_bonus4` int(10) NOT NULL DEFAULT '0',\n" +
	"  PRIMARY KEY  (`id`)\n" +
	")"

// characterRelationsSchema mirrors the shipped character_relations table
// definition verbatim.
const characterRelationsSchema = "CREATE TABLE IF NOT EXISTS `character_relations` (\n" +
	"  `char_id` INT UNSIGNED NOT NULL default 0,\n" +
	"  `friend_id` INT UNSIGNED NOT NULL DEFAULT 0,\n" +
	"  `relation` INT UNSIGNED NOT NULL DEFAULT 0,\n" +
	"  PRIMARY KEY (`char_id`,`friend_id`)\n" +
	")"

// petitionSchema mirrors the shipped petition table definition verbatim.
const petitionSchema = "CREATE TABLE IF NOT EXISTS `petition` (\n" +
	"  `oid` INT UNSIGNED NOT NULL DEFAULT 0,\n" +
	"  `type` VARCHAR(20) NOT NULL,\n" +
	"  `petitioner_oid` INT UNSIGNED NOT NULL DEFAULT 0,\n" +
	"  `submit_date` BIGINT UNSIGNED NOT NULL DEFAULT 0,\n" +
	"  `content` VARCHAR(256) NOT NULL,\n" +
	"  `is_unread` SMALLINT(1) NOT NULL DEFAULT 1,\n" +
	"  `state` VARCHAR(20) NOT NULL,\n" +
	"  `rate` VARCHAR(10) NOT NULL,\n" +
	"  `feedback` VARCHAR(512) NOT NULL,\n" +
	"  `responders` VARCHAR(150) NOT NULL,\n" +
	"  PRIMARY KEY  (`oid`)\n" +
	")"

// petitionMessageSchema mirrors the shipped petition_message table
// definition verbatim.
const petitionMessageSchema = "CREATE TABLE IF NOT EXISTS `petition_message` (\n" +
	"  `id` INT UNSIGNED NOT NULL DEFAULT 0,\n" +
	"  `petition_oid` INT UNSIGNED NOT NULL DEFAULT 0,\n" +
	"  `player_oid` INT UNSIGNED NOT NULL DEFAULT 0,\n" +
	"  `type` VARCHAR(20) NOT NULL,\n" +
	"  `player_name` VARCHAR(20) NOT NULL,\n" +
	"  `content` VARCHAR(120) NOT NULL,\n" +
	"  PRIMARY KEY  (`id`, `petition_oid`)\n" +
	")"

// clanDataSchema mirrors the shipped clan_data table definition verbatim.
const clanDataSchema = "CREATE TABLE IF NOT EXISTS `clan_data` (\n" +
	"  `clan_id` INT NOT NULL DEFAULT 0,\n" +
	"  `clan_name` VARCHAR(20),\n" +
	"  `clan_level` INT NOT NULL DEFAULT 0,\n" +
	"  `reputation_score` INT NOT NULL DEFAULT 0,\n" +
	"  `hasCastle` TINYINT NOT NULL DEFAULT 0,\n" +
	"  `ally_id` INT NOT NULL DEFAULT 0,\n" +
	"  `ally_name` VARCHAR(20),\n" +
	"  `leader_id` INT NOT NULL DEFAULT 0,\n" +
	"  `new_leader_id` INT NOT NULL DEFAULT 0,\n" +
	"  `crest_id` INT NOT NULL DEFAULT 0,\n" +
	"  `crest_large_id` INT NOT NULL DEFAULT 0,\n" +
	"  `ally_crest_id` INT NOT NULL DEFAULT 0,\n" +
	"  `auction_bid_at` INT NOT NULL DEFAULT 0,\n" +
	"  `ally_penalty_expiry_time` BIGINT NOT NULL DEFAULT 0,\n" +
	"  `ally_penalty_type` INT NOT NULL DEFAULT 0,\n" +
	"  `char_penalty_expiry_time` BIGINT NOT NULL DEFAULT 0,\n" +
	"  `dissolving_expiry_time` BIGINT NOT NULL DEFAULT 0,\n" +
	"  `enabled` TINYINT NOT NULL DEFAULT 0,\n" +
	"  `notice` TEXT,\n" +
	"  `introduction` TEXT,\n" +
	"  PRIMARY KEY (`clan_id`),\n" +
	"  KEY `leader_id` (`leader_id`),\n" +
	"  KEY `ally_id` (`ally_id`)\n" +
	")"

// clanPrivsSchema mirrors the shipped clan_privs table definition verbatim.
const clanPrivsSchema = "CREATE TABLE IF NOT EXISTS `clan_privs` (\n" +
	"  `clan_id` INT NOT NULL DEFAULT'0',\n" +
	"  `ranking` INT NOT NULL DEFAULT '0',\n" +
	"  `privs` INT NOT NULL DEFAULT '0',\n" +
	"  PRIMARY KEY (`clan_id`,`ranking`)\n" +
	")"

// bbsMailSchema mirrors the shipped bbs_mail table definition verbatim.
const bbsMailSchema = "CREATE TABLE IF NOT EXISTS `bbs_mail` (\n" +
	"  `id` INT UNSIGNED NOT NULL DEFAULT 0,\n" +
	"  `receiver_id` INT UNSIGNED NOT NULL DEFAULT 0,\n" +
	"  `sender_id` INT UNSIGNED NOT NULL DEFAULT 0,\n" +
	"  `location` VARCHAR(15) NOT NULL,\n" +
	"  `recipients` VARCHAR(200) DEFAULT NULL,\n" +
	"  `subject` VARCHAR(128) DEFAULT NULL,\n" +
	"  `message` VARCHAR(3000) DEFAULT NULL,\n" +
	"  `sent_date` TIMESTAMP NULL DEFAULT NULL,\n" +
	"  `is_unread` SMALLINT(1) DEFAULT 1,\n" +
	"  PRIMARY KEY  (`id`)\n" +
	")"

// bbsForumSchema mirrors the shipped bbs_forum table definition verbatim.
const bbsForumSchema = "CREATE TABLE IF NOT EXISTS `bbs_forum` (\n" +
	"  `id` int(8) NOT NULL default '0',\n" +
	"  `type` VARCHAR(10) NOT NULL default '0',\n" +
	"  `access` VARCHAR(12) NOT NULL default '0',\n" +
	"  `owner_id` int(8) NOT NULL default '0',\n" +
	"  UNIQUE KEY `id` (`id`)\n" +
	")"

// bbsTopicSchema mirrors the shipped bbs_topic table definition verbatim.
const bbsTopicSchema = "CREATE TABLE IF NOT EXISTS `bbs_topic` (\n" +
	"  `id` int(8) NOT NULL DEFAULT '0',\n" +
	"  `forum_id` int(8) NOT NULL DEFAULT '0',\n" +
	"  `name` varchar(255) NOT NULL DEFAULT '',\n" +
	"  `date` decimal(20,0) NOT NULL DEFAULT '0',\n" +
	"  `owner_name` varchar(255) NOT NULL DEFAULT '0',\n" +
	"  `owner_id` int(8) NOT NULL DEFAULT '0'\n" +
	")"

// bbsPostSchema mirrors the shipped bbs_post table definition verbatim.
const bbsPostSchema = "CREATE TABLE IF NOT EXISTS `bbs_post` (\n" +
	"  `id` int(8) NOT NULL DEFAULT '0',\n" +
	"  `owner_name` varchar(255) NOT NULL DEFAULT '',\n" +
	"  `owner_id` int(8) NOT NULL DEFAULT '0',\n" +
	"  `date` decimal(20,0) NOT NULL DEFAULT '0',\n" +
	"  `topic_id` int(8) NOT NULL DEFAULT '0',\n" +
	"  `forum_id` int(8) NOT NULL DEFAULT '0',\n" +
	"  `txt` text NOT NULL\n" +
	")"

// bbsFavoriteSchema mirrors the shipped bbs_favorite table definition verbatim.
const bbsFavoriteSchema = "CREATE TABLE IF NOT EXISTS `bbs_favorite` (\n" +
	"  `id` INT UNSIGNED NOT NULL DEFAULT 0,\n" +
	"  `player_id` INT UNSIGNED NOT NULL DEFAULT 0,\n" +
	"  `title` VARCHAR(35) DEFAULT NULL,\n" +
	"  `bypass` VARCHAR(128) DEFAULT NULL,\n" +
	"  `date` TIMESTAMP NULL DEFAULT NULL,\n" +
	"  PRIMARY KEY  (`id`)\n" +
	")"

// accountsSchema mirrors the shipped accounts table definition verbatim.
// The login server owns it; the game server's behavior harness runs a login
// server on the same database.
const accountsSchema = "CREATE TABLE IF NOT EXISTS `accounts` (\n" +
	"	`login` VARCHAR(45) NOT NULL DEFAULT '',\n" +
	"	`password` VARCHAR(60) NOT NULL DEFAULT '',\n" +
	"	`last_active` BIGINT NOT NULL DEFAULT 0,\n" +
	"	`access_level` INT(3) NOT NULL DEFAULT 0,\n" +
	"	`last_server` INT(4) NOT NULL DEFAULT 1,\n" +
	"	PRIMARY KEY (`login`)\n" +
	")"

// clanSkillsSchema mirrors the shipped clan_skills table definition verbatim.
const clanSkillsSchema = "CREATE TABLE IF NOT EXISTS clan_skills (\n" +
	"  clan_id INT NOT NULL DEFAULT 0,\n" +
	"  skill_id INT NOT NULL DEFAULT 0,\n" +
	"  skill_level INT NOT NULL DEFAULT 0,\n" +
	"  PRIMARY KEY (`clan_id`,`skill_id`)\n" +
	")"

// clanSubpledgesSchema mirrors the shipped clan_subpledges table definition
// verbatim.
const clanSubpledgesSchema = "CREATE TABLE IF NOT EXISTS `clan_subpledges` (\n" +
	"  `clan_id` INT NOT NULL default '0',\n" +
	"  `sub_pledge_id` INT NOT NULL default '0',\n" +
	"  `name` varchar(45),\n" +
	"  `leader_id` INT NOT NULL default '0',\n" +
	"  PRIMARY KEY (`clan_id`,`sub_pledge_id`)\n" +
	")"

// clanWarsSchema mirrors the shipped clan_wars table definition verbatim.
const clanWarsSchema = "CREATE TABLE IF NOT EXISTS `clan_wars` (\n" +
	"  `clan1` varchar(35) NOT NULL DEFAULT '',\n" +
	"  `clan2` varchar(35) NOT NULL DEFAULT '',\n" +
	"  `expiry_time` decimal(20,0) NOT NULL DEFAULT '0',\n" +
	"  PRIMARY KEY (`clan1`,`clan2`)\n" +
	")"

// sevenSignsStatusSeed seeds the single status row the gameserver reads and
// writes, matching the shipped schema seed.
const sevenSignsStatusSeed = "INSERT IGNORE INTO `seven_signs_status` VALUES " +
	"(0,1,1,'COMPETITION',0,'NORMAL',0,0,0,0,'NORMAL','NORMAL','NORMAL',0,0,0,0,0,0,0,0,0,0,0)"

// NewDB creates a fresh database on the shared MariaDB instance (see
// internal/dbtest), creates the gameserver tables used by integration
// tests, and returns a pool connected to it. The database is dropped and
// the pool closed when the test completes. Prefer SharedDB; NewDB is for
// store-level tests that want a database no other test has touched.
func NewDB(t *testing.T) *sql.DB {
	t.Helper()
	return dbtest.NewDB(t, append(append([]string(nil), schemaStmts...), seedStmts...)...)
}

// SharedDB returns tb's database from a per-test-binary pool of databases
// carrying the gameserver schema (see dbtest.Pool). The package's TestMain
// must call dbtest.Main.
func SharedDB(tb testing.TB) *sql.DB {
	tb.Helper()
	return pool.DB(tb)
}

var pool = dbtest.NewPool(dbtest.PoolConfig{Schema: schemaStmts, Seed: seedStmts})

var schemaStmts = []string{
	charactersSchema, itemsSchema, augmentationsSchema, spawnDataSchema,
	itemsOnGroundSchema, characterSkillsSchema, characterShortcutsSchema,
	characterHennasSchema, characterRecipeBookSchema, petsSchema, characterSkillsSaveSchema,
	sevenSignsStatusSchema, buylistsSchema, characterSubclassesSchema,
	characterRelationsSchema, petitionSchema, petitionMessageSchema,
	characterMacrosesSchema, characterRecommendsSchema,
	clanDataSchema, clanPrivsSchema, clanSkillsSchema, clanSubpledgesSchema, clanWarsSchema,
	accountsSchema,
	bbsMailSchema, bbsForumSchema, bbsTopicSchema, bbsPostSchema, bbsFavoriteSchema,
	olympiadNoblesSchema, olympiadNoblesEomSchema, serverMemoSchema,
}

var seedStmts = []string{sevenSignsStatusSeed}
