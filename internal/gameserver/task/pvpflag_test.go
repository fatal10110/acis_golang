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
	wantUnsupported := []string{"AwardPKKillPVPPoint", "KarmaPlayerCanShop"}
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

	props, err := config.ParseString(`PvPVsNormalTime = nope`)
	if err != nil {
		t.Fatalf("ParseString() error = %v", err)
	}
	if _, err := PvPFlagOptionsFromProperties(props); err == nil {
		t.Fatal("PvPFlagOptionsFromProperties() with bad int: expected error")
	}
}
