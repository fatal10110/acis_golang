package admin

import (
	"maps"
	"slices"

	"github.com/fatal10110/acis_golang/internal/commons"
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
	_, ok := d.accessLevels[level]
	return ok
}

// MasterLevel is the highest level the table defines, 0 when it defines
// none.
func (d *Data) MasterLevel() int {
	if d == nil || len(d.accessLevels) == 0 {
		return 0
	}
	return slices.Max(slices.Collect(maps.Keys(d.accessLevels)))
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
