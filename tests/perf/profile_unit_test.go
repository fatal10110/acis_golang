package perf

import (
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"
	"time"
)

// parkedForLeakCheck blocks until release closes; its name marks the stack
// the leak dump must show.
func parkedForLeakCheck(release <-chan struct{}, done chan<- struct{}) {
	<-release
	close(done)
}

func TestAwaitGoroutinesReportsLeakedStack(t *testing.T) {
	baseline := runtime.NumGoroutine()
	release, done := make(chan struct{}), make(chan struct{})
	go parkedForLeakCheck(release, done)

	n, dump := awaitGoroutines(baseline, 100*time.Millisecond)
	if n <= baseline {
		t.Fatalf("goroutines = %d, want above baseline %d while one is parked", n, baseline)
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
