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
