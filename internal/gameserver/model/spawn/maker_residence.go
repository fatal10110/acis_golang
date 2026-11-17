package spawn

import (
	"math"
	"strconv"
	"strings"
)

// ResidenceParam returns the residence id the maker gives the NPCs it
// spawns: the first parameter of a spawnTime such as
// "siege_warfare_start(7)", when the kind is known and is not door_open and
// the parameter is all ASCII digits. ok is false otherwise, and the NPCs
// keep their template's residence.
func (m *Maker) ResidenceParam() (id int, ok bool) {
	if m == nil {
		return 0, false
	}
	parts := splitSpawnTime(m.SpawnTime)
	if len(parts) != 2 {
		return 0, false
	}
	// Every known kind but door_open names the residence first.
	if kind := SpawnTimeKindOf(parts[0]); kind == SpawnTimeNone || kind == SpawnTimeDoorOpen {
		return 0, false
	}
	param, _, _ := strings.Cut(parts[1], ";")
	if param == "" || strings.TrimLeft(param, "0123456789") != "" {
		return 0, false
	}
	n, err := strconv.ParseInt(param, 10, 64)
	if err != nil || n > math.MaxInt32 {
		return 0, false
	}
	return int(n), true
}

// splitSpawnTime splits s at every bracket, keeping empty parts but the
// trailing ones, as a regular-expression split on "[()]" does.
func splitSpawnTime(s string) []string {
	var parts []string
	start := 0
	for i := 0; i < len(s); i++ {
		if s[i] == '(' || s[i] == ')' {
			parts = append(parts, s[start:i])
			start = i + 1
		}
	}
	parts = append(parts, s[start:])
	for len(parts) > 0 && parts[len(parts)-1] == "" {
		parts = parts[:len(parts)-1]
	}
	return parts
}
