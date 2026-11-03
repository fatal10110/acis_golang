package xml

import (
	"bufio"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"testing"

	"github.com/fatal10110/acis_golang/internal/gameserver/model/skill"
)

// TestHealSpsMatchesReferenceProbe replays the reference probe's spiritshot
// heal lookups (skill/formulas/testdata/formulas.golden, computed over the
// probe's copy of healSps.xml) on the shipped file as loaded, so the loader
// and the probe's copy agree with the datapack.
func TestHealSpsMatchesReferenceProbe(t *testing.T) {
	t.Parallel()
	table, err := LoadHealSps(datapackPath(t, filepath.Join("data", "xml", "healSps.xml")))
	if err != nil {
		t.Fatal(err)
	}

	f, err := os.Open("../../skill/formulas/testdata/formulas.golden")
	if err != nil {
		t.Fatal(err)
	}
	defer f.Close()

	entries, lookups := 0, 0
	sc := bufio.NewScanner(f)
	for line := 1; sc.Scan(); line++ {
		fields := strings.Fields(sc.Text())
		switch {
		case len(fields) > 0 && fields[0] == "healSpsEntry":
			entries++
		case len(fields) == 7 && fields[0] == "healSps":
			var in [4]int
			for i := range in {
				if in[i], err = strconv.Atoi(fields[1+i]); err != nil {
					t.Fatalf("line %d: %v", line, err)
				}
			}
			want, err := strconv.ParseFloat(fields[6], 64)
			if err != nil {
				t.Fatalf("line %d: %v", line, err)
			}
			if got := table.Calculate(skill.ID(in[0]), in[1], in[2], in[3]); got != want {
				t.Errorf("line %d: Calculate%v = %v, want %v", line, in, got, want)
			}
			lookups++
		}
	}
	if err := sc.Err(); err != nil {
		t.Fatal(err)
	}
	if entries != table.Count() || lookups == 0 {
		t.Fatalf("probe copies %d entries and checks %d lookups; healSps.xml has %d entries", entries, lookups, table.Count())
	}
}
