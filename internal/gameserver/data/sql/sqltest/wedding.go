package sqltest

// modsWeddingSchema mirrors the shipped mods_wedding table definition
// verbatim.
const modsWeddingSchema = "CREATE TABLE IF NOT EXISTS `mods_wedding` (\n" +
	"  `id` INT UNSIGNED NOT NULL DEFAULT 0,\n" +
	"  `requesterId` INT UNSIGNED NOT NULL DEFAULT 0,\n" +
	"  `partnerId` INT UNSIGNED NOT NULL DEFAULT 0,\n" +
	"  PRIMARY KEY (`id`)\n" +
	")"
