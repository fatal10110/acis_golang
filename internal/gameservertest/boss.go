package gameservertest

import "database/sql"

// WithBossSeed adjusts the database once the characters are seeded and
// before the raid points (character_raid_points) and the boss zones'
// allowed players (grandboss_list, for the zones WithZones supplies) are
// restored from it.
func WithBossSeed(seed func(db *sql.DB)) Option {
	return func(o *options) { o.seedBoss = seed }
}
