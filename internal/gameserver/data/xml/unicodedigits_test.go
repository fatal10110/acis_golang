package xml

import (
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
	"unicode/utf8"

	"github.com/fatal10110/acis_golang/internal/commons"
	"github.com/fatal10110/acis_golang/internal/gameserver/model/item"
	"github.com/fatal10110/acis_golang/internal/gameserver/model/location"
	"github.com/fatal10110/acis_golang/internal/gameserver/model/recipe"
	"github.com/fatal10110/acis_golang/internal/gameserver/model/residence/castle"
	"github.com/fatal10110/acis_golang/internal/gameserver/model/restart"
	"github.com/fatal10110/acis_golang/internal/gameserver/model/route"
	"github.com/fatal10110/acis_golang/internal/gameserver/model/skill"
	"github.com/fatal10110/acis_golang/internal/gameserver/model/spawn"
	"github.com/fatal10110/acis_golang/internal/gameserver/model/zone"
)

// TestDecimalListValuesReadUnicodeDigits covers the data values the model
// packages split and parse themselves ("x;y;z", "id-level", "a,b,chance",
// "Nmin", ...). The reference parses each part with Integer.parseInt, so,
// per a Java probe (OpenJDK 21.0.11), fullwidth "１２" reads 12 and
// Arabic-Indic "٧" reads 7.
func TestDecimalListValuesReadUnicodeDigits(t *testing.T) {
	t.Parallel()
	const twelve, seven = "１２", "٧"
	attrs := func(kv ...string) *attrValues {
		vals := make(map[string]string, len(kv)/2)
		for i := 0; i < len(kv); i += 2 {
			vals[kv[i]] = kv[i+1]
		}
		return newAttrValues(vals, "test")
	}
	cases := []struct {
		name string
		got  func() (any, error)
		want any
	}{
		{"spawn position", func() (any, error) {
			p, err := spawn.ParsePositions(twelve + ";-" + seven + ";3;" + seven)
			if err != nil {
				return nil, err
			}
			return p[0], nil
		}, spawn.Position{Location: locationOf(12, -7, 3), Heading: 7}},
		{"weighted spawn position chance", func() (any, error) {
			p, err := spawn.ParsePositions("1;2;3;4;" + twelve + "%;1;2;3;4;" + seven + "%")
			if err != nil {
				return nil, err
			}
			return [2]int{p[0].Chance, p[1].Chance}, nil
		}, [2]int{12, 7}},
		{"castle artifact pos", func() (any, error) {
			a, err := castle.NewArtifact(1, twelve+";2;3;"+seven)
			return [2]int{a.Position.X, a.Heading}, err
		}, [2]int{12, 7}},
		{"effect zone skill", func() (any, error) {
			refs, err := zone.ParseSkillRefs(twelve + "-" + seven)
			if err != nil {
				return nil, err
			}
			return refs[0], nil
		}, zone.SkillRef{ID: 12, Level: 7}},
		{"boat messages", func() (any, error) {
			b, err := buildBoatLocation(attrs("x", "1", "y", "2", "z", "3", "arrival", twelve+";"+seven, "scheduled", twelve+"-"+seven))
			if err != nil {
				return nil, err
			}
			return fmt.Sprint(b.ArrivalMessages, b.Scheduled), nil
		}, fmt.Sprint([]int{12, 7}, []route.ScheduledMessage{{ID: 12, Delay: 7}})},
		{"recipe material", func() (any, error) {
			r, err := buildRecipe(attrs("id", "1", "material", twelve+"-"+seven, "product", "1-1", "itemId", "1",
				"level", "1", "mpConsume", "1", "successRate", "100", "isDwarven", "true", "alias", "a"))
			if err != nil {
				return nil, err
			}
			return r.Materials[0], nil
		}, recipe.Ingredient{ItemID: 12, Count: 7}},
		{"capsuled items", func() (any, error) {
			p := skill.ParseExtractableItems(twelve + "," + seven + ",50")
			if len(p) != 1 {
				return nil, fmt.Errorf("products = %v", p)
			}
			return p[0].Items[0], nil
		}, skill.ExtractableItem{ItemID: 12, Quantity: 7}},
		{"item skill ref", func() (any, error) { return item.ParseSkillRef(twelve + "-" + seven) }, item.SkillRef{ID: 12, Level: 7}},
		{"skill shared reuse ref", func() (any, error) { return skill.ParseRef(twelve + "-" + seven) }, skill.Ref{ID: 12, Level: 7}},
		{"observer spawn groups", func() (any, error) {
			s, err := buildObserverSpawn(attrs("id", "1", "x", "1", "y", "2", "z", "3", "groups", twelve+";"+seven))
			return fmt.Sprint(s.Groups), err
		}, fmt.Sprint([]int{12, 7})},
		{"restart location", func() (any, error) { return restart.ParseLocationValue(twelve + ";" + seven + ";3") }, locationOf(12, 7, 3)},
		{"restart point", func() (any, error) {
			p, err := restart.ParsePointValue(twelve + ";" + seven)
			return [2]int{p.X, p.Y}, err
		}, [2]int{12, 7}},
		{"respawn delay", func() (any, error) { return commons.ParseGameDuration(twelve + "min") }, 12 * time.Minute},
		{"residence int list", func() (any, error) {
			v, err := splitInts(twelve + ";" + seven)
			return fmt.Sprint(v), err
		}, fmt.Sprint([]int{12, 7})},
	}
	for _, tc := range cases {
		got, err := tc.got()
		if err != nil {
			t.Errorf("%s: %v", tc.name, err)
			continue
		}
		if got != tc.want {
			t.Errorf("%s: got %+v, want %+v", tc.name, got, tc.want)
		}
	}
}

// TestAttrValuesIntegersReadUnicodeDigits covers the attribute readers the
// loaders read <set>-style values through (getInteger, getLong and
// getIntegerArray in the reference, all Character.digit based).
func TestAttrValuesIntegersReadUnicodeDigits(t *testing.T) {
	t.Parallel()
	a := newAttrValues(map[string]string{
		"int":   "１２",
		"int32": "１２７",
		"long":  "-०१",
		"array": "１;٢",
	}, "unicode digits")
	if v := a.int("int"); v != 12 {
		t.Errorf("int = %d, want 12", v)
	}
	if v := a.int32("int32"); v != 127 {
		t.Errorf("int32 = %d, want 127", v)
	}
	if v := a.int64("long"); v != -1 {
		t.Errorf("int64 = %d, want -1", v)
	}
	if v := a.intArray("array"); fmt.Sprint(v) != "[1 2]" {
		t.Errorf("intArray = %v, want [1 2]", v)
	}
	if err := a.Err(); err != nil {
		t.Fatalf("Err() = %v", err)
	}
}

func locationOf(x, y, z int) location.Location { return location.Location{X: x, Y: y, Z: z} }

// TestShippedDataHasNoNonASCIIDigits: the shipped data files hold no
// character the integer readers newly accept, so every shipped integer
// reads exactly as it did when they took ASCII digits only.
func TestShippedDataHasNoNonASCIIDigits(t *testing.T) {
	t.Parallel()
	root := datapackPath(t, "data")
	files := 0
	err := filepath.WalkDir(root, func(path string, d fs.DirEntry, err error) error {
		if err != nil || d.IsDir() || !strings.HasSuffix(path, ".xml") {
			return err
		}
		files++
		data, err := os.ReadFile(path)
		if err != nil {
			return err
		}
		for i, r := range string(data) {
			if r < utf8.RuneSelf {
				continue
			}
			_, decimal := commons.Atoi(string(r))
			_, hex := commons.DecodeInt32("0x" + string(r))
			if decimal == nil || hex == nil {
				t.Errorf("%s: byte %d: %U reads as a digit", path, i, r)
			}
		}
		return nil
	})
	if err != nil {
		t.Fatal(err)
	}
	if files == 0 {
		t.Fatal("no shipped XML file scanned")
	}
}
