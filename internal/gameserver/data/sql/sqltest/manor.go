package sqltest

// castleManorProcureSchema and castleManorProductionSchema mirror the
// shipped castle_manor_procure and castle_manor_production table
// definitions verbatim.
const (
	castleManorProcureSchema = "CREATE TABLE IF NOT EXISTS `castle_manor_procure` (\n" +
		" `castle_id` TINYINT(3) UNSIGNED NOT NULL DEFAULT '0',\n" +
		" `crop_id` INT(11) UNSIGNED NOT NULL DEFAULT '0',\n" +
		" `amount` INT(11) UNSIGNED NOT NULL DEFAULT '0',\n" +
		" `start_amount` INT(11) UNSIGNED NOT NULL DEFAULT '0',\n" +
		" `price` INT(11) UNSIGNED NOT NULL DEFAULT '0',\n" +
		" `reward_type` TINYINT(1) UNSIGNED NOT NULL DEFAULT '0',\n" +
		" `next_period` TINYINT(1) UNSIGNED NOT NULL DEFAULT '1',\n" +
		"  PRIMARY KEY (`castle_id`,`crop_id`,`next_period`)\n" +
		")"
	castleManorProductionSchema = "CREATE TABLE IF NOT EXISTS `castle_manor_production` (\n" +
		" `castle_id` TINYINT(3) UNSIGNED NOT NULL DEFAULT '0',\n" +
		" `seed_id` INT(11) UNSIGNED NOT NULL DEFAULT '0',\n" +
		" `amount` INT(11) UNSIGNED NOT NULL DEFAULT '0',\n" +
		" `start_amount` INT(11) UNSIGNED NOT NULL DEFAULT '0',\n" +
		" `price` INT(11) UNSIGNED NOT NULL DEFAULT '0',\n" +
		" `next_period` TINYINT(1) UNSIGNED NOT NULL DEFAULT '1',\n" +
		" PRIMARY KEY (`castle_id`, `seed_id`, `next_period`)\n" +
		")"
)
