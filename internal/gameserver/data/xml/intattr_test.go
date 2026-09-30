package xml

import (
	"fmt"
	"path/filepath"
	"regexp"
	"strings"
	"testing"

	"github.com/fatal10110/acis_golang/internal/gameserver/model/location"
	"github.com/fatal10110/acis_golang/internal/gameserver/model/restart"
)

// TestIntAttrsRejectMalformedValues pins the accepted input set of every
// non-coordinate integer attribute decoded into a tagged field, the same way
// TestCoordinateAttrRejectsMalformedValues pins the coordinates: a bare
// base-10 integer is read, while an empty, whitespace-padded, non-numeric,
// or out-of-int32 value fails the load naming the attribute and the file. A
// required attribute that is absent fails the same way; an optional one (a
// boat ticket item) defaults to 0. The decoder's own int conversion would
// read the empty and absent cases as 0 and trim the padding.
func TestIntAttrsRejectMalformedValues(t *testing.T) {
	t.Parallel()

	const square = `<node x="0" y="0"/><node x="100" y="0"/><node x="100" y="100"/><node x="0" y="100"/>`
	const boatNode = `<node x="1" y="2" z="3"/>`

	// Each doc holds one %s where the attribute under test is written, or
	// nothing when the case leaves it absent. load returns the value the
	// loader read for that attribute.
	fields := []struct {
		name     string
		attr     string
		doc      string
		optional bool
		load     func(path string) (int, error)
	}{
		{
			name: "manor id", attr: "id",
			doc: `<list><manor %s name="a"/></list>`,
			load: func(path string) (int, error) {
				table, err := LoadManors(path)
				if err != nil {
					return 0, err
				}
				return table.Manors[0].ID, nil
			},
		},
		{
			name: "manor area castleId", attr: "castleId",
			doc: `<list><area name="a" %s minZ="1" maxZ="2">` + square + `</area></list>`,
			load: func(path string) (int, error) {
				areas, err := LoadManorAreas(path)
				if err != nil {
					return 0, err
				}
				return areas[0].CastleID, nil
			},
		},
		{
			name: "manor area minZ", attr: "minZ",
			doc: `<list><area name="a" castleId="1" %s maxZ="2">` + square + `</area></list>`,
			load: func(path string) (int, error) {
				areas, err := LoadManorAreas(path)
				if err != nil {
					return 0, err
				}
				return areas[0].MinZ, nil
			},
		},
		{
			name: "manor area maxZ", attr: "maxZ",
			doc: `<list><area name="a" castleId="1" minZ="1" %s>` + square + `</area></list>`,
			load: func(path string) (int, error) {
				areas, err := LoadManorAreas(path)
				if err != nil {
					return 0, err
				}
				return areas[0].MaxZ, nil
			},
		},
		{
			name: "restart area minZ", attr: "minZ",
			doc: `<list><area %s maxZ="100">` + square + `</area></list>`,
			load: func(path string) (int, error) {
				table, err := LoadRestartPoints(path)
				if err != nil {
					return 0, err
				}
				lo, _ := restartZBand(table.Areas[0])
				return lo, nil
			},
		},
		{
			name: "restart area maxZ", attr: "maxZ",
			doc: `<list><area minZ="-100" %s>` + square + `</area></list>`,
			load: func(path string) (int, error) {
				table, err := LoadRestartPoints(path)
				if err != nil {
					return 0, err
				}
				_, hi := restartZBand(table.Areas[0])
				return hi, nil
			},
		},
		{
			name: "boat itinerary item1", attr: "item1", optional: true,
			doc: `<list><itinerary dock1="GIRAN" dock2="TALKING_ISLAND" %s item2="2" heading="1"><route>` + boatNode + `</route><route>` + boatNode + `</route></itinerary></list>`,
			load: func(path string) (int, error) {
				itineraries, err := LoadBoatRoutes(path)
				if err != nil {
					return 0, err
				}
				return itineraries[0].Routes[0].ItemID, nil
			},
		},
		{
			name: "boat itinerary item2", attr: "item2", optional: true,
			doc: `<list><itinerary dock1="GIRAN" dock2="TALKING_ISLAND" item1="1" %s heading="1"><route>` + boatNode + `</route><route>` + boatNode + `</route></itinerary></list>`,
			load: func(path string) (int, error) {
				itineraries, err := LoadBoatRoutes(path)
				if err != nil {
					return 0, err
				}
				return itineraries[0].Routes[1].ItemID, nil
			},
		},
		{
			name: "boat itinerary heading", attr: "heading",
			doc: `<list><itinerary dock1="GIRAN" %s><route>` + boatNode + `</route></itinerary></list>`,
			load: func(path string) (int, error) {
				itineraries, err := LoadBoatRoutes(path)
				if err != nil {
					return 0, err
				}
				return itineraries[0].Heading, nil
			},
		},
		{
			name: "observer group id", attr: "id",
			doc: `<list><groups><group %s><entry locId="1" x="1" y="2" z="3" yaw="0" pitch="0" cost="0" castle="0"/></group></groups></list>`,
			load: func(path string) (int, error) {
				table, err := LoadObserverGroups(path)
				if err != nil {
					return 0, err
				}
				for _, id := range []int{12, -12} {
					if _, ok := table.Group(id); ok {
						return id, nil
					}
				}
				return 0, fmt.Errorf("no group loaded under the expected id")
			},
		},
		{
			name: "teleport list npcId", attr: "npcId",
			doc: `<list><telPosList %s><loc desc="a" priceId="57" priceCount="1" x="1" y="2" z="3"/></telPosList></list>`,
			load: func(path string) (int, error) {
				table, err := LoadTeleports(path)
				if err != nil {
					return 0, err
				}
				for id := range table {
					return id, nil
				}
				return 0, fmt.Errorf("no teleport list loaded")
			},
		},
		{
			name: "instant teleport list npcId", attr: "npcId",
			doc: `<list><telPosList %s><loc x="1" y="2" z="3"/></telPosList></list>`,
			load: func(path string) (int, error) {
				table, err := LoadInstantTeleports(path)
				if err != nil {
					return 0, err
				}
				for id := range table {
					return id, nil
				}
				return 0, fmt.Errorf("no instant teleport list loaded")
			},
		},
	}

	values := []struct {
		name    string
		absent  bool
		value   string
		want    int
		wantErr bool
	}{
		{name: "plain value", value: "12", want: 12},
		{name: "negative value", value: "-12", want: -12},
		{name: "empty value is rejected", value: "", wantErr: true},
		{name: "padded value is rejected", value: " 12 ", wantErr: true},
		{name: "non-numeric value is rejected", value: "abc", wantErr: true},
		{name: "int32 overflow is rejected", value: "2147483648", wantErr: true},
		{name: "int32 underflow is rejected", value: "-2147483649", wantErr: true},
		{name: "absent", absent: true},
	}

	for _, f := range fields {
		for _, v := range values {
			t.Run(f.name+"/"+v.name, func(t *testing.T) {
				t.Parallel()
				attr := ""
				if !v.absent {
					attr = fmt.Sprintf(`%s=%q`, f.attr, v.value)
				}
				path := filepath.Join(t.TempDir(), "fixture.xml")
				writeXMLFixture(t, path, fmt.Sprintf(f.doc, attr))

				got, err := f.load(path)
				wantErr := v.wantErr || (v.absent && !f.optional)
				if wantErr {
					if err == nil {
						t.Fatalf("load(%s) = %d, want a rejection", attr, got)
					}
					// The attribute must be named as the failing one, not
					// merely appear somewhere (the temp path embeds the
					// subtest name, and a sibling attribute may share a
					// suffix, e.g. castleId and id).
					named := regexp.MustCompile(`(^|[^A-Za-z0-9])` + f.attr + `( is required|: )`)
					if msg := err.Error(); !named.MatchString(msg) || !strings.Contains(msg, path) {
						t.Fatalf("load(%s) error %q does not name attribute %q and file %q", attr, msg, f.attr, path)
					}
					return
				}
				if err != nil {
					t.Fatalf("load(%s) error: %v", attr, err)
				}
				if got != v.want {
					t.Fatalf("load(%s) = %d, want %d", attr, got, v.want)
				}
			})
		}
	}
}

// restartZBand probes a restart area's vertical extent at a point inside its
// polygon, since the area exposes only containment.
func restartZBand(a restart.Area) (lo, hi int) {
	lo, hi = 1, 0
	for z := -200; z <= 200; z++ {
		if !a.Contains(location.Location{X: 50, Y: 50, Z: z}) {
			continue
		}
		if lo > hi {
			lo = z
		}
		hi = z
	}
	return lo, hi
}
