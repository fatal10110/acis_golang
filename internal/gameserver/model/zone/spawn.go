package zone

import (
	"fmt"
	"math/rand/v2"

	"github.com/fatal10110/acis_golang/internal/gameserver/model/location"
)

// SpawnKind labels a group of spawn points inside a zone: where regular
// occupants respawn, where rule-breakers are thrown, and so on.
type SpawnKind uint8

// Spawn point groups, in data-contract order.
const (
	SpawnBanish SpawnKind = iota
	SpawnChaotic
	SpawnChallenger
	SpawnNormal
	SpawnOther
	SpawnOwner
)

// ParseSpawnKind maps the data files' spawn type names onto SpawnKind.
func ParseSpawnKind(s string) (SpawnKind, error) {
	switch s {
	case "BANISH":
		return SpawnBanish, nil
	case "CHAOTIC":
		return SpawnChaotic, nil
	case "CHALLENGER":
		return SpawnChallenger, nil
	case "NORMAL":
		return SpawnNormal, nil
	case "OTHER":
		return SpawnOther, nil
	case "OWNER":
		return SpawnOwner, nil
	default:
		return 0, fmt.Errorf("zone: unknown spawn kind %q", s)
	}
}

// String names the spawn kind as it appears in the data files.
func (k SpawnKind) String() string {
	switch k {
	case SpawnBanish:
		return "BANISH"
	case SpawnChaotic:
		return "CHAOTIC"
	case SpawnChallenger:
		return "CHALLENGER"
	case SpawnNormal:
		return "NORMAL"
	case SpawnOther:
		return "OTHER"
	case SpawnOwner:
		return "OWNER"
	default:
		return fmt.Sprintf("SpawnKind(%d)", uint8(k))
	}
}

// SpawnSite is implemented by zone kinds that carry grouped spawn points;
// the data loader feeds them through it.
type SpawnSite interface {
	AddSpawn(kind SpawnKind, loc location.Location)
}

// Spawns holds a zone's grouped spawn points. Kinds that need them embed
// it. Populated at load time, read-only afterwards.
type Spawns struct {
	groups map[SpawnKind][]location.Location
}

// AddSpawn appends loc to kind's group.
func (s *Spawns) AddSpawn(kind SpawnKind, loc location.Location) {
	if s.groups == nil {
		s.groups = make(map[SpawnKind][]location.Location)
	}
	s.groups[kind] = append(s.groups[kind], loc)
}

// Spawn returns kind's spawn points, falling back to the NORMAL group when
// the kind has none.
func (s *Spawns) Spawn(kind SpawnKind) []location.Location {
	if locs, ok := s.groups[kind]; ok {
		return locs
	}
	return s.groups[SpawnNormal]
}

// RandomSpawn picks a uniformly random spawn point of kind (with the same
// NORMAL fallback as Spawn). ok is false when no candidates exist.
func (s *Spawns) RandomSpawn(kind SpawnKind) (loc location.Location, ok bool) {
	locs := s.Spawn(kind)
	if len(locs) == 0 {
		return location.Location{}, false
	}
	return locs[rand.IntN(len(locs))], true
}
