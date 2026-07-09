package probe

import (
	"fmt"
	"math/rand/v2"
	"strconv"
	"strings"

	"github.com/fatal10110/acis_golang/internal/datadiff"
	"github.com/fatal10110/acis_golang/internal/gameserver/geo/engine"
	"github.com/fatal10110/acis_golang/internal/gameserver/geo/pathfind"
	"github.com/fatal10110/acis_golang/internal/gameserver/model/location"
)

// Category names a kind of geodata question a Query poses.
type Category string

const (
	Height      Category = "height"
	CanMove     Category = "canmove"
	LineOfSight Category = "los"
	Path        Category = "path"
)

// categories lists every Category, in the fixed order Random cycles
// through to spread a generated sample evenly across kinds.
var categories = [...]Category{Height, CanMove, LineOfSight, Path}

// Query is one geodata question: a single point for Height, or a from/to
// pair for the others.
type Query struct {
	Category Category
	From     location.Location
	To       location.Location // zero for Height
}

// ID renders q as the canonical string used as a datadiff.Record ID:
// "<category>:x,y,z" for Height, "<category>:x,y,z->x,y,z" otherwise.
// ParseQuery reverses it.
func (q Query) ID() string {
	if q.Category == Height {
		return fmt.Sprintf("%s:%s", q.Category, formatPoint(q.From))
	}
	return fmt.Sprintf("%s:%s->%s", q.Category, formatPoint(q.From), formatPoint(q.To))
}

// ParseQuery parses an ID produced by Query.ID back into a Query.
func ParseQuery(id string) (Query, error) {
	category, rest, ok := strings.Cut(id, ":")
	if !ok {
		return Query{}, fmt.Errorf("probe: query id %q: missing category separator", id)
	}

	if Category(category) == Height {
		from, err := parsePoint(rest)
		if err != nil {
			return Query{}, fmt.Errorf("probe: query id %q: %w", id, err)
		}
		return Query{Category: Height, From: from}, nil
	}

	fromStr, toStr, ok := strings.Cut(rest, "->")
	if !ok {
		return Query{}, fmt.Errorf(`probe: query id %q: missing "->" separator`, id)
	}
	from, err := parsePoint(fromStr)
	if err != nil {
		return Query{}, fmt.Errorf("probe: query id %q: %w", id, err)
	}
	to, err := parsePoint(toStr)
	if err != nil {
		return Query{}, fmt.Errorf("probe: query id %q: %w", id, err)
	}

	switch c := Category(category); c {
	case CanMove, LineOfSight, Path:
		return Query{Category: c, From: from, To: to}, nil
	default:
		return Query{}, fmt.Errorf("probe: query id %q: unknown category %q", id, category)
	}
}

func formatPoint(l location.Location) string {
	return fmt.Sprintf("%d,%d,%d", l.X, l.Y, l.Z)
}

func parsePoint(s string) (location.Location, error) {
	parts := strings.Split(s, ",")
	if len(parts) != 3 {
		return location.Location{}, fmt.Errorf("point %q: want 3 comma-separated coordinates", s)
	}
	var coords [3]int
	for i, p := range parts {
		// World coordinates are int32-range; parse at that width instead of
		// strconv.Atoi so an out-of-range dump value fails here rather than
		// silently truncating wherever the geo engine later narrows it.
		v, err := strconv.ParseInt(p, 10, 32)
		if err != nil {
			return location.Location{}, fmt.Errorf("point %q: %w", s, err)
		}
		coords[i] = int(v)
	}
	return location.Location{X: coords[0], Y: coords[1], Z: coords[2]}, nil
}

// Evaluate runs q against e (and f, for Path) and returns the result as a
// datadiff.Record: q.ID() as the ID, and the answer as its fields, so two
// evaluations of the same query — Go's and an oracle's — diff as plain text
// via datadiff.Compare.
func Evaluate(e *engine.Engine, f *pathfind.Finder, q Query) datadiff.Record {
	fields := make(map[string]string)
	switch q.Category {
	case Height:
		fields["height"] = strconv.Itoa(int(e.Height(q.From.X, q.From.Y, q.From.Z)))
	case CanMove:
		fields["result"] = strconv.FormatBool(e.CanMove(q.From.X, q.From.Y, q.From.Z, q.To.X, q.To.Y, q.To.Z))
	case LineOfSight:
		fields["result"] = strconv.FormatBool(e.CanSee(q.From.X, q.From.Y, q.From.Z, q.To.X, q.To.Y, q.To.Z))
	case Path:
		path, cost, ok := f.Find(q.From, q.To)
		fields["found"] = strconv.FormatBool(ok)
		if ok {
			fields["cost"] = strconv.Itoa(cost)
			fields["points"] = formatPath(path)
		}
	}
	return datadiff.Record{ID: q.ID(), Fields: fields}
}

func formatPath(path []location.Location) string {
	points := make([]string, len(path))
	for i, p := range path {
		points[i] = formatPoint(p)
	}
	return strings.Join(points, ";")
}

// zBand bounds the random Z offset Random draws around sea level.
//
// ponytail: no spawn-Z distribution to sample from, so this is a generous
// fixed band rather than a per-region estimate; widen it if a real sample
// needs deeper dungeons or taller towers than it currently reaches.
const zBand = 8192

// Random generates n queries spread evenly across every Category, with X/Y
// drawn uniformly from the whole supported map and Z drawn from a band
// around sea level; the engine resolves each point to its nearest actual
// geodata layer, the same way a caller with an approximate Z would. seed
// makes the sample reproducible: the same seed always yields the same
// query set.
func Random(n int, seed uint64) []Query {
	rng := rand.New(rand.NewPCG(seed, seed))
	queries := make([]Query, n)
	for i := range queries {
		queries[i] = Query{
			Category: categories[i%len(categories)],
			From:     randomPoint(rng),
			To:       randomPoint(rng),
		}
	}
	return queries
}

func randomPoint(rng *rand.Rand) location.Location {
	return location.Location{
		X: engine.WorldXMin + rng.IntN(engine.WorldXMax-engine.WorldXMin+1),
		Y: engine.WorldYMin + rng.IntN(engine.WorldYMax-engine.WorldYMin+1),
		Z: rng.IntN(2*zBand+1) - zBand,
	}
}
