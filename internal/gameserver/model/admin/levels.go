package admin

import (
	"maps"
	"slices"

	"github.com/fatal10110/acis_golang/internal/commons"
	"github.com/rs/zerolog"
)

// The name and title colors an access level takes when its entry leaves
// them out.
const (
	defaultNameColor  int32 = 0xFFFFFF
	defaultTitleColor int32 = 0xFFFF77
)

// DefinesLevel reports whether the access-level table has an entry for level,
// every negative level being read as -1. A nil table defines none.
func (d *Data) DefinesLevel(level int) bool {
	if d == nil {
		return false
	}
	if level < 0 {
		level = -1
	}
	_, ok := d.load().accessLevels[level]
	return ok
}

// Grant returns the access level the character named character plays under
// once given level, as Resolve does, logging a level the table does not
// define, which reads as the user level, and the master level.
func (d *Data) Grant(level int, character string, log zerolog.Logger) AccessLevel {
	if !d.DefinesLevel(level) {
		log.Warn().Int("level", level).Str("player", character).Msg("access level: undefined level granted, reset to the user level")
	}
	if level > 0 && level == d.MasterLevel() {
		log.Info().Str("player", character).Msg("access level: logged in with master access level")
	}
	return d.Resolve(level)
}

// MasterLevel is the highest level the table defines, 0 when it defines
// none.
func (d *Data) MasterLevel() int {
	levels := d.load().accessLevels
	if len(levels) == 0 {
		return 0
	}
	return slices.Max(slices.Collect(maps.Keys(levels)))
}

// Colors returns the name and title colors a character playing under a
// takes. A color the level leaves unset or unreadable is the default one;
// the loader rejects unreadable colors, so only a level built by hand
// reaches that fallback.
func (a AccessLevel) Colors() (name, title int32) {
	return color(a.NameColor, defaultNameColor), color(a.TitleColor, defaultTitleColor)
}

func color(hex string, fallback int32) int32 {
	if hex == "" {
		return fallback
	}
	v, err := commons.DecodeInt32("0x" + hex)
	if err != nil {
		return fallback
	}
	return v
}
