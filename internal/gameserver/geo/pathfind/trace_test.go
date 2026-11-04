package pathfind

import (
	"slices"
	"strconv"
	"strings"
	"testing"

	"github.com/fatal10110/acis_golang/internal/gameserver/geo/engine"
)

func TestFindTracedMatchesFindAndDrawsTheRoute(t *testing.T) {
	traced := 0
	for seed := range int64(20) {
		e := newSeededMazeEngine(t, 24, 24, seed)
		finder := New(e, DefaultOptions())
		origin, target := at(1, 1, 0), at(22, 22, 0)
		want, _, wantOK := finder.Find(origin, target)
		tr := &engine.Trace{}
		got, ok := finder.FindTraced(origin, target, tr)
		if ok != wantOK || !slices.Equal(got, want) {
			t.Fatalf("seed %d: FindTraced() = %v, %v, want %v, %v", seed, got, ok, want, wantOK)
		}
		if !ok {
			if len(tr.Strokes) != 0 {
				t.Fatalf("seed %d: no route but trace has %d strokes", seed, len(tr.Strokes))
			}
			continue
		}
		timing := tr.Strokes[0]
		if timing.Line || timing.Color != engine.TraceRed || !strings.HasSuffix(timing.Label, "ms") {
			t.Fatalf("seed %d: first stroke = %+v, want the red search time point", seed, timing)
		}
		var segments []engine.Stroke
		for _, s := range tr.Strokes[1:] {
			if s.Line {
				segments = append(segments, s)
			}
		}
		if len(segments) == 0 {
			continue
		}
		traced++
		if len(segments) != len(got) {
			t.Fatalf("seed %d: %d segments for a %d point route", seed, len(segments), len(got))
		}
		prev := origin
		for i, s := range segments {
			if s.Label != "Segment #"+strconv.Itoa(i+1) || s.Color != engine.TraceGreen || s.X != prev.X || s.Y != prev.Y || s.X2 != got[i].X || s.Y2 != got[i].Y {
				t.Fatalf("seed %d: segment %d = %+v, want from %v to %v", seed, i, s, prev, got[i])
			}
			prev = got[i]
		}
	}
	if traced == 0 {
		t.Fatal("no seed produced a route of three points or more")
	}
}
