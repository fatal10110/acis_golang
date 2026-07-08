package spawn

import (
	"fmt"
	"strings"
	"time"

	"github.com/fatal10110/acis_golang/internal/commons"
	"github.com/fatal10110/acis_golang/internal/gameserver/model/location"
)

// Position is one explicit spawn position from the "pos" attribute.
type Position struct {
	Location location.Location
	Heading  int
	Chance   int
}

// Private is one child NPC declared under a spawn entry's <privates> block.
type Private struct {
	NPCID        int32
	Weight       int
	RespawnDelay time.Duration
}

// Entry is one <npc .../> row under an <npcmaker>.
type Entry struct {
	NPCID         int32
	Total         int
	RespawnDelay  time.Duration
	RespawnRandom time.Duration
	Positions     []Position
	Privates      []Private
	AIParams      map[string]string
	DBName        string
	DBSaving      []string
}

// NewPrivate builds one private spawn row from its XML attributes.
func NewPrivate(set *commons.StatSet) (Private, error) {
	npcID, err := set.GetInt32("id")
	if err != nil {
		return Private{}, err
	}
	weight, err := set.GetInt("weight")
	if err != nil {
		return Private{}, err
	}
	respawn, err := parseDuration(set, "respawn")
	if err != nil {
		return Private{}, err
	}
	return Private{
		NPCID:        npcID,
		Weight:       weight,
		RespawnDelay: respawn,
	}, nil
}

// NewEntry builds one spawn entry from its XML attributes and already-parsed
// child blocks.
func NewEntry(set *commons.StatSet, positions []Position, privates []Private, aiParams map[string]string) (Entry, error) {
	npcID, err := set.GetInt32("id")
	if err != nil {
		return Entry{}, err
	}
	total, err := set.GetInt("total")
	if err != nil {
		return Entry{}, err
	}
	respawnDelay, err := parseDuration(set, "respawn")
	if err != nil {
		return Entry{}, err
	}
	respawnRandom, err := parseDuration(set, "respawnRand")
	if err != nil {
		return Entry{}, err
	}

	return Entry{
		NPCID:         npcID,
		Total:         total,
		RespawnDelay:  respawnDelay,
		RespawnRandom: respawnRandom,
		Positions:     append([]Position(nil), positions...),
		Privates:      append([]Private(nil), privates...),
		AIParams:      copyStringMap(aiParams),
		DBName:        set.GetStringDefault("dbName", ""),
		DBSaving:      cleanStrings(set.GetStringArrayDefault("dbSaving", nil)),
	}, nil
}

func parseDuration(set *commons.StatSet, key string) (time.Duration, error) {
	raw := set.GetStringDefault(key, "")
	if raw == "" {
		return 0, nil
	}
	d, err := commons.ParseGameDuration(raw)
	if err != nil {
		return 0, fmt.Errorf("%s: %w", key, err)
	}
	return d, nil
}

// ParsePositions decodes the semicolon-separated "pos" attribute shape used
// by spawnlist XML: either one fixed [x;y;z;heading] tuple or N weighted
// [x;y;z;heading;chance%] tuples.
func ParsePositions(raw string) ([]Position, error) {
	raw = strings.TrimSpace(raw)
	if raw == "" {
		return nil, nil
	}

	parts := strings.Split(raw, ";")
	if len(parts) == 4 {
		pos, err := parsePosition(parts, false)
		if err != nil {
			return nil, err
		}
		return []Position{pos}, nil
	}
	if len(parts)%5 != 0 {
		return nil, fmt.Errorf("spawn: malformed pos %q", raw)
	}

	positions := make([]Position, 0, len(parts)/5)
	for i := 0; i < len(parts); i += 5 {
		pos, err := parsePosition(parts[i:i+5], true)
		if err != nil {
			return nil, err
		}
		positions = append(positions, pos)
	}
	return positions, nil
}

func parsePosition(parts []string, weighted bool) (Position, error) {
	set := commons.NewStatSetWithCapacity(4)
	set.Set("x", parts[0])
	set.Set("y", parts[1])
	set.Set("z", parts[2])
	loc, err := location.NewLocation(set)
	if err != nil {
		return Position{}, err
	}

	set = commons.NewStatSetWithCapacity(2)
	set.Set("heading", parts[3])
	heading, err := set.GetInt("heading")
	if err != nil {
		return Position{}, err
	}
	chance := 0
	if weighted {
		set.Set("chance", strings.TrimSuffix(parts[4], "%"))
		chance, err = set.GetInt("chance")
		if err != nil {
			return Position{}, err
		}
	}

	return Position{
		Location: loc,
		Heading:  heading,
		Chance:   chance,
	}, nil
}

func copyStringMap(src map[string]string) map[string]string {
	if len(src) == 0 {
		return nil
	}
	dst := make(map[string]string, len(src))
	for k, v := range src {
		dst[k] = v
	}
	return dst
}

func cleanStrings(in []string) []string {
	if len(in) == 0 {
		return nil
	}
	out := make([]string, 0, len(in))
	for _, s := range in {
		s = strings.TrimSpace(s)
		if s == "" {
			continue
		}
		out = append(out, s)
	}
	if len(out) == 0 {
		return nil
	}
	return out
}
