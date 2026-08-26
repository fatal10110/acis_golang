// Package sqltest starts a real, disposable MariaDB instance carrying the
// shipped gameserver schema, for integration tests across this module that
// need to read and write through it rather than a mock.
package sqltest

import (
	"context"
	"database/sql"
	"fmt"
	"os"
	"sync"
	"testing"

	_ "github.com/go-sql-driver/mysql"
	"github.com/testcontainers/testcontainers-go/modules/mariadb"
)

// sharedTables lists every table SharedDB truncates between tests, in an
// order safe for TRUNCATE (no cross-table FKs today, so order doesn't
// matter, but keep it in sync with the CREATE TABLE calls below).
var sharedTables = []string{
	"characters", "items", "augmentations", "spawn_data",
	"items_on_ground", "character_skills", "character_shortcuts",
	"pets", "character_skills_save", "seven_signs_status",
}

// sharedReseeds restores shipped seed rows that TRUNCATE removes, keyed by
// table, so every test starts from the schema's default data.
var sharedReseeds = map[string]string{
	"seven_signs_status": sevenSignsStatusSeed,
}

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

// sevenSignsStatusSeed seeds the single status row the gameserver reads and
// writes, matching the shipped schema seed.
const sevenSignsStatusSeed = "INSERT IGNORE INTO `seven_signs_status` VALUES " +
	"(0,1,1,'COMPETITION',0,'NORMAL',0,0,0,0,'NORMAL','NORMAL','NORMAL',0,0,0,0,0,0,0,0,0,0,0)"

// NewDB starts a real MariaDB container, creates the gameserver tables used
// by integration tests, and returns a pool connected to it. The container is
// terminated and the pool closed when the test completes.
func NewDB(t *testing.T) *sql.DB {
	t.Helper()
	ctx := context.Background()

	container, err := mariadb.Run(ctx, "mariadb:11")
	if err != nil {
		t.Fatalf("start mariadb container: %v", err)
	}
	t.Cleanup(func() {
		if err := container.Terminate(ctx); err != nil {
			t.Logf("terminate mariadb container: %v", err)
		}
	})

	db, err := openSchema(ctx, container)
	if err != nil {
		t.Fatalf("%v", err)
	}
	t.Cleanup(func() { db.Close() })
	return db
}

func openSchema(ctx context.Context, container *mariadb.MariaDBContainer) (*sql.DB, error) {
	dsn, err := container.ConnectionString(ctx)
	if err != nil {
		return nil, fmt.Errorf("connection string: %w", err)
	}

	db, err := sql.Open("mysql", dsn)
	if err != nil {
		return nil, fmt.Errorf("open db: %w", err)
	}

	for _, stmt := range []string{
		charactersSchema, itemsSchema, augmentationsSchema, spawnDataSchema,
		itemsOnGroundSchema, characterSkillsSchema, characterShortcutsSchema,
		petsSchema, characterSkillsSaveSchema, sevenSignsStatusSchema,
		sevenSignsStatusSeed,
	} {
		if _, err := db.ExecContext(ctx, stmt); err != nil {
			db.Close()
			return nil, fmt.Errorf("create table: %w", err)
		}
	}
	return db, nil
}

var (
	sharedOnce      sync.Once
	sharedDB        *sql.DB
	sharedContainer *mariadb.MariaDBContainer
	sharedErr       error
)

// SharedDB returns a MariaDB pool shared by every test in the current test
// binary, i.e. one container per package (Go compiles each package's tests
// into its own binary, so the sync.Once below fires exactly once per
// package). The package's TestMain must call Main so the container is
// terminated once, after every test in the package has run, instead of
// leaking for a reaper to clean up.
//
// Each caller gets the tables truncated after its own test via tb.Cleanup,
// so tests don't see rows left behind by earlier tests in the package.
func SharedDB(tb testing.TB) *sql.DB {
	tb.Helper()
	sharedOnce.Do(func() {
		container, err := mariadb.Run(context.Background(), "mariadb:11")
		if err != nil {
			sharedErr = fmt.Errorf("start mariadb container: %w", err)
			return
		}
		sharedContainer = container
		sharedDB, sharedErr = openSchema(context.Background(), container)
	})
	if sharedErr != nil {
		tb.Fatalf("shared mariadb db: %v", sharedErr)
	}

	tb.Cleanup(func() {
		ctx := context.Background()
		for _, table := range sharedTables {
			if _, err := sharedDB.ExecContext(ctx, "TRUNCATE TABLE `"+table+"`"); err != nil {
				tb.Fatalf("truncate %s: %v", table, err)
			}
			if seed, ok := sharedReseeds[table]; ok {
				if _, err := sharedDB.ExecContext(ctx, seed); err != nil {
					tb.Fatalf("reseed %s: %v", table, err)
				}
			}
		}
	})
	return sharedDB
}

// Main runs a package's tests and terminates the SharedDB container
// afterward, if one was started. Every package using SharedDB must call it
// from a TestMain:
//
//	func TestMain(m *testing.M) { os.Exit(sqltest.Main(m)) }
func Main(m *testing.M) int {
	code := m.Run()
	if sharedDB != nil {
		sharedDB.Close()
	}
	if sharedContainer != nil {
		if err := sharedContainer.Terminate(context.Background()); err != nil {
			fmt.Fprintf(os.Stderr, "terminate shared mariadb container: %v\n", err)
		}
	}
	return code
}
