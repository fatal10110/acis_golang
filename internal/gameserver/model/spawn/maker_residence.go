package spawn

import (
	"math"
	"strconv"
	"strings"
)

// residenceSpawnTimes are the spawn-time kinds whose first parameter names
// the residence the maker's NPCs belong to: every kind but door_open.
var residenceSpawnTimes = []string{
	"agit_battle_royal_start",
	"agit_final_start",
	"agit_defend_warfare_start",
	"agit_attack_warfare_start",
	"siege_warfare_start",
	"pc_siege_warfare_start",
}

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
	known := false
	for _, kind := range residenceSpawnTimes {
		if strings.EqualFold(kind, parts[0]) {
			known = true
			break
		}
	}
	if !known {
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
// trailing ones, as the reference's split on "[()]" does.
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
