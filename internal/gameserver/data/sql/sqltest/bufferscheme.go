package sqltest

// bufferSchemesSchema mirrors the shipped buffer_schemes table definition
// verbatim.
const bufferSchemesSchema = "CREATE TABLE IF NOT EXISTS `buffer_schemes` (\n" +
	"  `object_id` INT UNSIGNED NOT NULL DEFAULT '0',\n" +
	"  `scheme_name` VARCHAR(16) NOT NULL DEFAULT 'default',\n" +
	"  `skills` VARCHAR(200) NOT NULL,\n" +
	"  PRIMARY KEY (`object_id`, `scheme_name`)\n" +
	")"
