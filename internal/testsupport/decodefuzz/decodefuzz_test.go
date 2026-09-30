package decodefuzz

import (
	"fmt"
	"testing"
)

// fatalRecorder records Fatalf instead of stopping the test, so Bounded's
// verdict can be asserted.
type fatalRecorder struct {
	testing.TB
	failure string
}

func (r *fatalRecorder) Helper() {}

func (r *fatalRecorder) Fatalf(format string, args ...any) {
	r.failure = fmt.Sprintf(format, args...)
}

var sink []byte

func TestBoundedFlagsAllocationUnrelatedToInputSize(t *testing.T) {
	r := &fatalRecorder{TB: t}
	Bounded(r, "count-sized", make([]byte, 8), func() { sink = make([]byte, 1<<20) })
	if r.failure == "" {
		t.Fatal("Bounded accepted a 1 MiB allocation for an 8-byte input")
	}
}

func TestBoundedAcceptsAllocationProportionalToInput(t *testing.T) {
	r := &fatalRecorder{TB: t}
	input := make([]byte, 1<<20)
	Bounded(r, "copy", input, func() { sink = append([]byte(nil), input...) })
	if r.failure != "" {
		t.Fatalf("Bounded rejected copying its input: %s", r.failure)
	}
}
