package perf

import (
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"
	"time"
)

// parkedForLeakCheck reports through started that it runs, then blocks until
// release closes; its name marks the stack the leak dump must show.
func parkedForLeakCheck(started chan<- struct{}, release <-chan struct{}, done chan<- struct{}) {
	close(started)
	<-release
	close(done)
}

func TestAwaitGoroutinesReportsLeakedStack(t *testing.T) {
	baseline := runtime.NumGoroutine()
	started, release, done := make(chan struct{}), make(chan struct{}), make(chan struct{})
	go parkedForLeakCheck(started, release, done)
	// A goroutine that has not been scheduled yet has no frames in the dump.
	<-started

	// Unrelated goroutines counted in baseline may end at any moment, so the
	// parked one alone does not keep the count above baseline. Only this
	// goroutine and the parked one are known to be alive, so a floor of one
	// is exceeded however other goroutines start or end; a zero grace takes
	// the dump on the first sample.
	const floor = 1
	n, dump := awaitGoroutines(floor, 0)
	if n <= floor || dump == "" {
		t.Fatalf("goroutines = %d with an empty dump, want above floor %d with a dump while one is parked", n, floor)
	}
	if !strings.Contains(dump, "parkedForLeakCheck") {
		t.Errorf("leak dump does not name the parked goroutine:\n%s", dump)
	}

	close(release)
	<-done
	if n, dump := awaitGoroutines(baseline, 5*time.Second); dump != "" {
		t.Errorf("goroutines = %d after release, want back at baseline %d:\n%s", n, baseline, dump)
	}
}

func TestProfileCaptureWritesProfiles(t *testing.T) {
	dir := filepath.Join(t.TempDir(), "prof")
	t.Setenv("ACIS_PERF_PROFILE_DIR", dir)

	p := startProfile(t)
	if p == nil {
		t.Fatal("startProfile returned nil with ACIS_PERF_PROFILE_DIR set")
	}
	p.stop(t)

	for _, name := range []string{"cpu.pprof", "heap.pprof", "goroutine.txt"} {
		info, err := os.Stat(filepath.Join(dir, name))
		if err != nil {
			t.Errorf("%s: %v", name, err)
			continue
		}
		if info.Size() == 0 {
			t.Errorf("%s is empty", name)
		}
	}
}

func TestProfileCaptureOffWithoutDir(t *testing.T) {
	t.Setenv("ACIS_PERF_PROFILE_DIR", "")
	p := startProfile(t)
	if p != nil {
		t.Fatal("startProfile started a capture without ACIS_PERF_PROFILE_DIR")
	}
	p.stop(t) // a nil capture records nothing
}
