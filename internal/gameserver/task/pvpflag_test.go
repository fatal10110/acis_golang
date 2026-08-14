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

// TestPvPFlagsTickDuePartitionIsNotPreSized guards against tickExpiry
// pre-sizing its due partition to the full tracked count on every sweep,
// instead of only allocating when something is actually due. A pre-sized
// due costs one constant extra allocation regardless of N, so an
// allocation-count (or count-ratio) assertion cannot see it — it shows up
// only as extra bytes. Measuring bytes/op via testing.Benchmark, not
// testing.AllocsPerRun, is what makes this test actually fail when that
// regression is reintroduced: with all 128 flags non-expiring, tickExpiry's
// own necessary work (appending every entry to pending) costs ~11.9 KB/op,
// while a due pre-sized to len(entries) would add ~5.1 KB more
// (128 * sizeof(deadlineEntry[PvPFlagActor])) on every single call.
func TestPvPFlagsTickDuePartitionIsNotPreSized(t *testing.T) {
	now := time.UnixMilli(0)
	flags := NewPvPFlags(DefaultPvPFlagOptions(), func() time.Time { return now })
	for i := 0; i < 128; i++ {
		flags.Add(&pvpFlagFakeActor{id: int32(i + 1)}, time.Hour)
	}

	res := testing.Benchmark(func(b *testing.B) {
		b.ReportAllocs()
		for i := 0; i < b.N; i++ {
			flags.Tick()
		}
	})

	if got := res.AllocedBytesPerOp(); got > 14_000 {
		t.Fatalf("PvPFlags.Tick = %d B/op at 128 non-expiring flags, want <= 14000; due partition may be pre-sized to the tracked count even though nothing is due", got)
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
