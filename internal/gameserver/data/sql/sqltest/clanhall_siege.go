package sqltest

// clanhallSiegeAttackersSchema mirrors the shipped clanhall_siege_attackers
// table definition verbatim.
const clanhallSiegeAttackersSchema = "CREATE TABLE IF NOT EXISTS `clanhall_siege_attackers` (\n" +
	"  `clanhall_id` TINYINT(2) NOT NULL DEFAULT '0',\n" +
	"  `attacker_id` INT NOT NULL DEFAULT '0',\n" +
	"  PRIMARY KEY (`clanhall_id`, `attacker_id`)\n" +
	")"

// rainbowspringsAttackerListSchema mirrors the shipped
// rainbowsprings_attacker_list table definition verbatim.
const rainbowspringsAttackerListSchema = "CREATE TABLE IF NOT EXISTS `rainbowsprings_attacker_list` (\n" +
	"  `clanId` INT NOT NULL DEFAULT 0,\n" +
	"  `war_decrees_count`INT NOT NULL DEFAULT 0,\n" +
	"  PRIMARY KEY `clanId` (`clanId`)\n" +
	")"

// clanhallFlagwarAttackersSchema mirrors the shipped
// clanhall_flagwar_attackers table definition verbatim.
const clanhallFlagwarAttackersSchema = "CREATE TABLE IF NOT EXISTS `clanhall_flagwar_attackers` (\n" +
	"  `clanhall_id` TINYINT(2) UNSIGNED NOT NULL DEFAULT '0',\n" +
	"  `flag` INT(10) UNSIGNED NOT NULL DEFAULT '0',\n" +
	"  `npc` INT(10) UNSIGNED NOT NULL DEFAULT '0',\n" +
	"  `clan_id` INT(10) UNSIGNED NOT NULL DEFAULT '0',\n" +
	"  PRIMARY KEY (`flag`),\n" +
	"  KEY `hall_id` (`clanhall_id`),\n" +
	"  KEY `clan_id` (`clan_id`)\n" +
	")"

// clanhallFlagwarMembersSchema mirrors the shipped clanhall_flagwar_members
// table definition verbatim.
const clanhallFlagwarMembersSchema = "CREATE TABLE IF NOT EXISTS `clanhall_flagwar_members` (\n" +
	"  `clanhall_id` TINYINT(2) UNSIGNED NOT NULL DEFAULT '0',\n" +
	"  `clan_id` INT(10) UNSIGNED NOT NULL DEFAULT '0',\n" +
	"  `object_id` INT(10) UNSIGNED NOT NULL DEFAULT '0',\n" +
	"  KEY `clanhall_id` (`clanhall_id`),\n" +
	"  KEY `clan_id` (`clan_id`),\n" +
	"  KEY `object_id` (`object_id`)\n" +
	")"

// clanhallFlagwarOwnerNpcsSchema mirrors the shipped
// clanhall_flagwar_owner_npcs table definition verbatim.
const clanhallFlagwarOwnerNpcsSchema = "CREATE TABLE IF NOT EXISTS `clanhall_flagwar_owner_npcs` (\n" +
	"  `clanhall_id` TINYINT(2) UNSIGNED NOT NULL DEFAULT '0',\n" +
	"  `npc_id` INT(10) UNSIGNED NOT NULL DEFAULT '0',\n" +
	"  `clan_id` INT(10) UNSIGNED NOT NULL DEFAULT '0',\n" +
	"  PRIMARY KEY (`clanhall_id`),\n" +
	"  KEY `npc_id` (`npc_id`),\n" +
	"  KEY `clan_id` (`clan_id`)\n" +
	")"
