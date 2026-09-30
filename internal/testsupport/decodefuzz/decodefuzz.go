// Package decodefuzz holds the checks shared by the fuzz targets that feed
// malformed inbound bytes to the listeners' framing, decryption, and packet
// decoders.
package decodefuzz

import (
	"runtime"
	"runtime/metrics"
	"testing"
)

const (
	// perByteBudget bounds heap growth proportional to the input: decoding
	// a UTF-16 string grows it by at most 1.5x, and a step copies the
	// fields it keeps at most once or twice more.
	perByteBudget = 4
	// fixedBudget covers per-call constant work (RSA credential blocks,
	// error formatting) plus the runtime's heap accounting granularity,
	// which charges small objects a whole span at a time.
	fixedBudget = 64 << 10
	// attempts is how many times a step may run before an over-budget
	// reading counts. The heap counter is process-wide: while fuzzing, the
	// engine's own goroutines allocate alongside the step, and a garbage
	// collection cycle in flight charges whole spans as caches refill. Each
	// retry therefore starts from a completed collection. A step whose
	// allocation really scales with its input exceeds the budget on every
	// run; that noise does not.
	attempts = 3
)

// heapAllocsMetric is the cumulative count of bytes the program has
// allocated on the heap. Large objects are counted exactly when they are
// allocated.
const heapAllocsMetric = "/gc/heap/allocs:bytes"

// Bounded runs step, which consumes input and must be repeatable, and fails
// t when step allocates more heap than input's length justifies. A decoder
// that sizes a buffer from an attacker-controlled count before checking it
// against the bytes actually present fails here even when it then rejects
// the packet.
func Bounded(t testing.TB, name string, input []byte, step func()) {
	t.Helper()
	budget := uint64(fixedBudget + perByteBudget*len(input))
	sample := []metrics.Sample{{Name: heapAllocsMetric}}
	var grew uint64
	for attempt := range attempts {
		if attempt > 0 {
			runtime.GC()
		}
		metrics.Read(sample)
		before := sample[0].Value.Uint64()
		step()
		metrics.Read(sample)
		if grew = sample[0].Value.Uint64() - before; grew <= budget {
			return
		}
	}
	t.Fatalf("%s allocated %d bytes decoding a %d-byte input; budget %d", name, grew, len(input), budget)
}
