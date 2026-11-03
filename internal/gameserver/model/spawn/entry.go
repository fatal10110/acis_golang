package spawn

import (
	"fmt"
	"strconv"
	"strings"
	"time"

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
	// AIParamKeys are the names of AIParams in the order the entry first
	// declares each.
	AIParamKeys []string
	DBName      string
	DBSaving    []string
}

// NewEntry returns e as the spawnlist loads it: Total is scaled and
// rounded by multiplier (the configured spawn multiplier) only when
// Positions is empty, distinguishing a fixed/weighted "pos" list (total
// kept as declared) from a coordinate-less territory spawn (total scaled).
// Empty Positions, Privates, AIParams and DBSaving read as nil, and blank
// DBSaving names are dropped.
func NewEntry(e Entry, multiplier float64) Entry {
	if len(e.Positions) == 0 {
		e.Total = scaleBySpawnMultiplier(e.Total, multiplier)
	}
	e.Positions = append([]Position(nil), e.Positions...)
	e.Privates = append([]Private(nil), e.Privates...)
	e.AIParams = copyStringMap(e.AIParams)
	e.AIParamKeys = append([]string(nil), e.AIParamKeys...)
	e.DBSaving = cleanStrings(e.DBSaving)
	return e
}

// ParsePositions decodes the semicolon-separated "pos" attribute shape used
// by spawnlist XML: either one fixed [x;y;z;heading] tuple or N weighted
// [x;y;z;heading;chance%] tuples.
func ParsePositions(raw string) ([]Position, error) {
	raw = strings.TrimSpace(raw)
	if raw == "" {
		return nil, nil
	}
	raw = strings.TrimRight(raw, ";")
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
	if len(parts) < 5 {
		return nil, fmt.Errorf("spawn: malformed pos %q", raw)
	}

	// The weighted list holds count/5 tuples: a token count of 5 or more
	// that isn't a multiple of 5 yields floor(count/5) complete tuples, and
	// the remainder tokens are silently dropped without being parsed.
	groups := len(parts) / 5
	positions := make([]Position, 0, groups)
	for i := 0; i < groups; i++ {
		pos, err := parsePosition(parts[i*5:i*5+5], true)
		if err != nil {
			return nil, err
		}
		positions = append(positions, pos)
	}
	return positions, nil
}

func parsePosition(parts []string, weighted bool) (Position, error) {
	nums := make([]int, 4)
	for i, name := range []string{"x", "y", "z", "heading"} {
		n, err := strconv.Atoi(parts[i])
		if err != nil {
			return Position{}, fmt.Errorf("spawn: pos %s: %w", name, err)
		}
		nums[i] = n
	}
	chance := 0
	if weighted {
		n, err := strconv.Atoi(strings.TrimSuffix(parts[4], "%"))
		if err != nil {
			return Position{}, fmt.Errorf("spawn: pos chance: %w", err)
		}
		chance = n
	}

	return Position{
		Location: location.Location{X: nums[0], Y: nums[1], Z: nums[2]},
		Heading:  nums[3],
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
