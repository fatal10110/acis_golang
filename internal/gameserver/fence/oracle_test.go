package fence

import (
	"bufio"
	"os"
	"strconv"
	"strings"
	"testing"
)

// TestSizingMatchesReferenceProbe replays testdata/fence.golden, the output of
// testdata/oracle/run.sh: the reference's own size table and alignment
// statement. "size n offset cells" starts a run of fence lengths (up to the
// next size line) that place with that offset and cell count, "size n none"
// a run that the reference refuses; "align v offset want" is the reference
// aligning coordinate v for a placement with that offset; "outline type cx cy
// rows..." is the reference's inner description of a fence, one #/. string
// per x cell.
func TestSizingMatchesReferenceProbe(t *testing.T) {
	f, err := os.Open("testdata/fence.golden")
	if err != nil {
		t.Fatal(err)
	}
	defer f.Close()

	type step struct {
		start int
		s     size
		ok    bool
	}
	var steps []step
	aligned, outlines := 0, 0
	atoi := func(s string) int {
		n, err := strconv.Atoi(s)
		if err != nil {
			t.Fatal(err)
		}
		return n
	}
	sc := bufio.NewScanner(f)
	for sc.Scan() {
		fl := strings.Fields(sc.Text())
		switch {
		case fl[0] == "size" && len(fl) == 4:
			steps = append(steps, step{atoi(fl[1]), size{atoi(fl[2]), atoi(fl[3])}, true})
		case fl[0] == "size" && len(fl) == 3 && fl[2] == "none":
			steps = append(steps, step{start: atoi(fl[1])})
		case fl[0] == "align" && len(fl) == 4:
			v, offset, want := atoi(fl[1]), atoi(fl[2]), atoi(fl[3])
			if got := align(v, size{offset: offset}); got != want {
				t.Errorf("align(%d, offset %d) = %d, want %d", v, offset, got, want)
			}
			aligned++
		case fl[0] == "outline" && len(fl) >= 5:
			typ, cx, cy := atoi(fl[1]), atoi(fl[2]), atoi(fl[3])
			rows := fl[4:]
			got := outline(typ, cx, cy)
			if len(rows) != cx || len(got) != cx {
				t.Fatalf("outline(%d, %d, %d) has %d rows, want %d", typ, cx, cy, len(got), len(rows))
			}
			for ix, row := range rows {
				for iy, c := range row {
					if got[ix][iy] != (c == '#') {
						t.Errorf("outline(%d, %d, %d)[%d][%d] = %v, want %c", typ, cx, cy, ix, iy, got[ix][iy], c)
					}
				}
			}
			outlines++
		default:
			t.Fatalf("malformed golden line %q", sc.Text())
		}
	}
	if err := sc.Err(); err != nil {
		t.Fatal(err)
	}
	if len(steps) == 0 || aligned == 0 || outlines == 0 {
		t.Fatal("golden is missing a section")
	}
	first := steps[0].start
	for n := first; n <= first+1600; n++ {
		var want step
		for _, st := range steps {
			if st.start <= n {
				want = st
			}
		}
		got, ok := sizeOf(n)
		if ok != want.ok || got != want.s {
			t.Errorf("sizeOf(%d) = %v %v, want %v %v", n, got, ok, want.s, want.ok)
		}
	}
}
