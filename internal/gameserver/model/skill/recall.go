package skill

import (
	"strings"

	"github.com/fatal10110/acis_golang/internal/commons"
	"github.com/fatal10110/acis_golang/internal/gameserver/model/location"
)

// RecallType selects where a RECALL or TELEPORT skill without fixed
// coordinates sends each target: its nearest town, or its clan's castle or
// clan hall.
type RecallType uint8

const (
	RecallTown RecallType = iota
	RecallCastle
	RecallClanHall
)

// ParseRecallType resolves a "recallType" value, compared without regard
// to case. Any value other than Castle or ClanHall, the empty default
// included, recalls to town.
func ParseRecallType(s string) RecallType {
	switch {
	case strings.EqualFold(s, "Castle"):
		return RecallCastle
	case strings.EqualFold(s, "ClanHall"):
		return RecallClanHall
	default:
		return RecallTown
	}
}

// ParseTeleCoords resolves a "teleCoords" value, "x;y;z" with each part
// trimmed. Parts past the third are ignored but must still be integers;
// fewer than three parts, or any part that is not an integer, leaves the
// skill without coordinates (ok false), so it recalls by its RecallType.
func ParseTeleCoords(s string) (loc location.Location, ok bool) {
	parts := strings.Split(s, ";")
	if len(parts) < 3 {
		return location.Location{}, false
	}
	vals := make([]int, len(parts))
	for i, p := range parts {
		n, err := commons.ParseInt(strings.TrimSpace(p), 32)
		if err != nil {
			return location.Location{}, false
		}
		vals[i] = int(n)
	}
	return location.Location{X: vals[0], Y: vals[1], Z: vals[2]}, true
}
