package task

import (
	"slices"
	"testing"
	"time"

	"github.com/fatal10110/acis_golang/internal/config"
)

type pvpFlagFakeActor struct {
	id     int32
	flag   PvPFlagState
	events []PvPFlagState
}

func (a *pvpFlagFakeActor) ObjectID() int32 { return a.id }

func (a *pvpFlagFakeActor) UpdatePvPFlag(flag PvPFlagState) {
	if a.flag == flag {
		return
	}
	a.flag = flag
	a.events = append(a.events, flag)
}

func TestPvPFlagsTickUpdatesBlinksAndExpires(t *testing.T) {
	now := time.UnixMilli(1_000)
	flags := NewPvPFlags(DefaultPvPFlagOptions(), func() time.Time { return now })
	actor := &pvpFlagFakeActor{id: 1}

	flags.Add(actor, 10*time.Second)
	flags.Tick()
	if got, want := actor.events, []PvPFlagState{PvPFlagOn}; !slices.Equal(got, want) {
		t.Fatalf("initial Tick events = %v, want %v", got, want)
	}

	now = now.Add(5 * time.Second)
	flags.Tick()
	if got, want := actor.events, []PvPFlagState{PvPFlagOn}; !slices.Equal(got, want) {
		t.Fatalf("Tick at exactly five seconds left = %v, want unchanged %v", got, want)
	}

	now = now.Add(time.Millisecond)
	flags.Tick()
	if got, want := actor.events, []PvPFlagState{PvPFlagOn, PvPFlagBlinking}; !slices.Equal(got, want) {
		t.Fatalf("Tick inside blink window = %v, want %v", got, want)
	}

	now = time.UnixMilli(11_000)
	flags.Tick()
	if got, want := actor.events, []PvPFlagState{PvPFlagOn, PvPFlagBlinking}; !slices.Equal(got, want) {
		t.Fatalf("Tick at exact deadline = %v, want unchanged %v", got, want)
	}

	now = now.Add(time.Millisecond)
	flags.Tick()
	if got, want := actor.events, []PvPFlagState{PvPFlagOn, PvPFlagBlinking, PvPFlagNone}; !slices.Equal(got, want) {
		t.Fatalf("Tick after deadline = %v, want %v", got, want)
	}
	if flags.Len() != 0 {
		t.Fatalf("Len() after expiry = %d, want 0", flags.Len())
	}
}

// TestPvPFlagsTickDueAllocationDoesNotScaleWithTrackedCount guards against
// tickExpiry pre-sizing its due partition to the full tracked count on
// every sweep (as opposed to allocating only when something is actually
// due). With every flag non-expiring, the sweep's own work — appending
// every entry to pending — necessarily costs more allocations at 128
// tracked flags than at 8, since growing that slice needs more
// reallocations; a pre-sized due slice would add a second, much larger
// N-proportional allocation on top of that on every single call. Comparing
// the two counts' ratio against the tracked-count ratio catches that
// second source without asserting an exact, Go-version-fragile count.
func TestPvPFlagsTickDueAllocationDoesNotScaleWithTrackedCount(t *testing.T) {
	newFlags := func(n int) *PvPFlags {
		now := time.UnixMilli(0)
		flags := NewPvPFlags(DefaultPvPFlagOptions(), func() time.Time { return now })
		for i := 0; i < n; i++ {
			flags.Add(&pvpFlagFakeActor{id: int32(i + 1)}, time.Hour)
		}
		return flags
	}

	small := newFlags(8)
	large := newFlags(128)

	smallAllocs := testing.AllocsPerRun(50, small.Tick)
	largeAllocs := testing.AllocsPerRun(50, large.Tick)

	if largeAllocs > smallAllocs*4 {
		t.Fatalf("allocs at 128 tracked flags (16x more than 8) = %v, want at most 4x the 8-flag allocs (%v); due partition may be pre-sized to the full tracked count even though nothing is due", largeAllocs, smallAllocs)
	}
}

func TestPvPFlagsRemoveCanLeaveCurrentFlag(t *testing.T) {
	now := time.UnixMilli(1_000)
	flags := NewPvPFlags(DefaultPvPFlagOptions(), func() time.Time { return now })
	actor := &pvpFlagFakeActor{id: 1}

	flags.Add(actor, 10*time.Second)
	flags.Tick()
	flags.Remove(actor, false)

	now = now.Add(11 * time.Second)
	flags.Tick()
	if got, want := actor.events, []PvPFlagState{PvPFlagOn}; !slices.Equal(got, want) {
		t.Fatalf("events after non-reset remove = %v, want %v", got, want)
	}
}

func TestPvPFlagsConfiguredDurations(t *testing.T) {
	now := time.UnixMilli(1_000)
	flags := NewPvPFlags(PvPFlagOptions{Normal: 10 * time.Second, Flagged: 2 * time.Second}, func() time.Time { return now })
	normal := &pvpFlagFakeActor{id: 1}
	flagged := &pvpFlagFakeActor{id: 2}

	flags.AddNormal(normal)
	flags.AddFlagged(flagged)
	if got, want := normal.events, []PvPFlagState{PvPFlagOn}; !slices.Equal(got, want) {
		t.Fatalf("normal add events = %v, want %v", got, want)
	}
	if got, want := flagged.events, []PvPFlagState{PvPFlagOn}; !slices.Equal(got, want) {
		t.Fatalf("flagged add events = %v, want %v", got, want)
	}

	now = now.Add(2*time.Second + time.Millisecond)
	flags.Tick()
	if got, want := flagged.events, []PvPFlagState{PvPFlagOn, PvPFlagNone}; !slices.Equal(got, want) {
		t.Fatalf("flagged timeout events = %v, want %v", got, want)
	}
	if got, want := normal.events, []PvPFlagState{PvPFlagOn}; !slices.Equal(got, want) {
		t.Fatalf("normal timeout early events = %v, want %v", got, want)
	}

	now = time.UnixMilli(11_001)
	flags.Tick()
	if got, want := normal.events, []PvPFlagState{PvPFlagOn, PvPFlagNone}; !slices.Equal(got, want) {
		t.Fatalf("normal timeout events = %v, want %v", got, want)
	}
}

func TestPvPFlagOptionsFromProperties(t *testing.T) {
	props, err := config.ParseString(`
PvPVsNormalTime = 40000
PvPVsPvPTime = 20000
KarmaPlayerCanShop = False
AwardPKKillPVPPoint = False
`)
	if err != nil {
		t.Fatalf("ParseString() error = %v", err)
	}

	opts, err := PvPFlagOptionsFromProperties(props)
	if err != nil {
		t.Fatalf("PvPFlagOptionsFromProperties() error = %v", err)
	}
	if opts.Normal != 40*time.Second || opts.Flagged != 20*time.Second {
		t.Fatalf("durations = normal %s flagged %s, want 40s/20s", opts.Normal, opts.Flagged)
	}
	if opts.AwardPKKillPVPPoint {
		t.Fatal("AwardPKKillPVPPoint = true, want false")
	}
	wantUnsupported := []string{"KarmaPlayerCanShop"}
	if !slices.Equal(opts.UnsupportedKeys, wantUnsupported) {
		t.Fatalf("UnsupportedKeys = %v, want %v", opts.UnsupportedKeys, wantUnsupported)
	}
}

func TestPvPFlagOptionsDefaultsAndInvalidValues(t *testing.T) {
	opts, err := PvPFlagOptionsFromProperties(nil)
	if err != nil {
		t.Fatalf("PvPFlagOptionsFromProperties(nil) error = %v", err)
	}
	if opts.Normal != 40*time.Second || opts.Flagged != 20*time.Second {
		t.Fatalf("default durations = normal %s flagged %s, want 40s/20s", opts.Normal, opts.Flagged)
	}
	if !opts.AwardPKKillPVPPoint {
		t.Fatal("default AwardPKKillPVPPoint = false, want true")
	}

	props, err := config.ParseString(`PvPVsNormalTime = nope`)
	if err != nil {
		t.Fatalf("ParseString() error = %v", err)
	}
	if _, err := PvPFlagOptionsFromProperties(props); err == nil {
		t.Fatal("PvPFlagOptionsFromProperties() with bad int: expected error")
	}
}

func BenchmarkPvPFlagsTickManyNonExpiringFlags(b *testing.B) {
	now := time.UnixMilli(0)
	flags := NewPvPFlags(DefaultPvPFlagOptions(), func() time.Time { return now })
	for i := 0; i < 128; i++ {
		flags.Add(&pvpFlagFakeActor{id: int32(i + 1)}, time.Hour)
	}

	b.ReportAllocs()
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		flags.Tick()
	}
}
