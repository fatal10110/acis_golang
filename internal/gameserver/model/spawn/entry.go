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
	f := commons.NewFields(set, "spawn private")
	private := Private{
		NPCID:        f.Int32("id"),
		Weight:       f.Int("weight"),
		RespawnDelay: parseDuration(f, "respawn"),
	}
	if err := f.Err(); err != nil {
		return Private{}, err
	}
	return private, nil
}

// NewEntry builds one spawn entry from its XML attributes and already-parsed
// child blocks.
func NewEntry(set *commons.StatSet, positions []Position, privates []Private, aiParams map[string]string) (Entry, error) {
	idf := commons.NewFields(set, "spawn entry")
	npcID := idf.Int32("id")
	if err := idf.Err(); err != nil {
		return Entry{}, err
	}

	f := commons.NewFields(set, fmt.Sprintf("spawn entry %d", npcID))
	entry := Entry{
		NPCID:         npcID,
		Total:         f.Int("total"),
		RespawnDelay:  parseDuration(f, "respawn"),
		RespawnRandom: parseDuration(f, "respawnRand"),
		Positions:     append([]Position(nil), positions...),
		Privates:      append([]Private(nil), privates...),
		AIParams:      copyStringMap(aiParams),
		DBName:        f.StringDefault("dbName", ""),
		DBSaving:      cleanStrings(f.StringArrayDefault("dbSaving", nil)),
	}
	if err := f.Err(); err != nil {
		return Entry{}, err
	}
	return entry, nil
}

func parseDuration(f *commons.Fields, key string) time.Duration {
	raw := f.StringDefault(key, "")
	if raw == "" {
		return 0
	}
	d, err := commons.ParseGameDuration(raw)
	if err != nil {
		f.Fail(fmt.Errorf("%s: %w", key, err))
		return 0
	}
	return d
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
