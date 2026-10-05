package perf

import (
	"os"
	"path/filepath"
	"runtime"
	"runtime/metrics"
	"runtime/pprof"
	"strings"
	"testing"
	"time"
)

// leakGrace bounds how long the server may take to reap the goroutines of
// the connections the load run closed. A character whose connection drops in
// attack stance stays in the world for the 15 s combat linger
// (network.disconnectCombatDelay, Java's in-combat logout delay) and every
// load client disconnects mid-fight, so the grace covers that plus slack.
const leakGrace = 25 * time.Second

// profileCapture records the load run's profiles into ACIS_PERF_PROFILE_DIR:
// cpu.pprof over the measured window, then heap.pprof and goroutine.txt at
// its end, while every client is still connected. A nil capture records
// nothing.
type profileCapture struct {
	dir string
	cpu *os.File
}

// startProfile starts the CPU profile when ACIS_PERF_PROFILE_DIR is set.
func startProfile(t *testing.T) *profileCapture {
	t.Helper()
	dir := os.Getenv("ACIS_PERF_PROFILE_DIR")
	if dir == "" {
		return nil
	}
	if err := os.MkdirAll(dir, 0o755); err != nil {
		t.Fatalf("ACIS_PERF_PROFILE_DIR=%q: %v", dir, err)
	}
	f, err := os.Create(filepath.Join(dir, "cpu.pprof"))
	if err != nil {
		t.Fatal(err)
	}
	if err := pprof.StartCPUProfile(f); err != nil {
		_ = f.Close()
		t.Fatalf("start CPU profile (drop -cpuprofile when ACIS_PERF_PROFILE_DIR is set): %v", err)
	}
	return &profileCapture{dir: dir, cpu: f}
}

// stop ends the CPU profile and writes the heap and goroutine snapshots.
func (p *profileCapture) stop(t *testing.T) {
	t.Helper()
	if p == nil {
		return
	}
	pprof.StopCPUProfile()
	if err := p.cpu.Close(); err != nil {
		t.Fatal(err)
	}
	// The heap profile reports the live heap as of the last collection.
	runtime.GC()
	writeProfile(t, filepath.Join(p.dir, "heap.pprof"), "heap", 0)
	writeProfile(t, filepath.Join(p.dir, "goroutine.txt"), "goroutine", 1)
	t.Logf("  profiles written to %s (cpu.pprof, heap.pprof, goroutine.txt)", p.dir)
}

func writeProfile(t *testing.T, path, name string, debug int) {
	t.Helper()
	f, err := os.Create(path)
	if err != nil {
		t.Fatal(err)
	}
	defer f.Close()
	if err := pprof.Lookup(name).WriteTo(f, debug); err != nil {
		t.Fatalf("write %s profile: %v", name, err)
	}
}

// checkGoroutineLeak fails the run when, after every client connection was
// closed, the process does not get back to baseline goroutines within
// leakGrace. baseline is the count taken right after boot, before the load
// run opened its extra connections, so each goroutine still alive past it
// was started for a connection or a character that is gone. The failure
// carries the grouped goroutine dump, written to ACIS_PERF_PROFILE_DIR as
// goroutine-leak.txt when that is set.
func checkGoroutineLeak(t *testing.T, baseline int) {
	t.Helper()
	n, dump := awaitGoroutines(baseline, leakGrace)
	t.Logf("  goroutines after disconnect=%d (baseline %d)", n, baseline)
	if dump == "" {
		return
	}
	if dir := os.Getenv("ACIS_PERF_PROFILE_DIR"); dir != "" {
		path := filepath.Join(dir, "goroutine-leak.txt")
		if err := os.WriteFile(path, []byte(dump), 0o644); err != nil {
			t.Fatal(err)
		}
		t.Errorf("goroutine leak: %d goroutines %s after every client disconnected, baseline %d; dump in %s", n, leakGrace, baseline, path)
		return
	}
	t.Errorf("goroutine leak: %d goroutines %s after every client disconnected, baseline %d:\n%s", n, leakGrace, baseline, dump)
}

// awaitGoroutines polls until at most baseline goroutines are alive or grace
// runs out. It returns the last count and, when that is still above
// baseline, the grouped goroutine dump (debug=1).
func awaitGoroutines(baseline int, grace time.Duration) (int, string) {
	deadline := time.Now().Add(grace)
	n := runtime.NumGoroutine()
	for n > baseline && time.Now().Before(deadline) {
		time.Sleep(20 * time.Millisecond)
		n = runtime.NumGoroutine()
	}
	if n <= baseline {
		return n, ""
	}
	var dump strings.Builder
	// WriteTo on a strings.Builder cannot fail.
	_ = pprof.Lookup("goroutine").WriteTo(&dump, 1)
	return n, dump.String()
}

// readGCCPU returns the runtime's estimate of the CPU time spent on garbage
// collection since process start (mark assists, background and idle marking,
// pauses).
func readGCCPU() time.Duration {
	sample := []metrics.Sample{{Name: "/cpu/classes/gc/total:cpu-seconds"}}
	metrics.Read(sample)
	return time.Duration(sample[0].Value.Float64() * float64(time.Second))
}
