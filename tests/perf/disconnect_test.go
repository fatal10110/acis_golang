package perf

import (
	"encoding/json"
	"fmt"
	"slices"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/rs/zerolog"

	"github.com/fatal10110/acis_golang/internal/gameservertest"
	"github.com/fatal10110/acis_golang/internal/testsupport"
)

// TestMassDisconnect puts ACIS_PERF_CLIENTS characters at one spot, so each
// sees every other, then drops every connection at once and measures the
// detach burst: its wall time and the sim tasks the watchdog reported over
// its budget. Skipped unless ACIS_PERF_CLIENTS is set:
//
//	ACIS_PERF_CLIENTS=300 go test ./tests/perf/ -run TestMassDisconnect -v -count=1
//
// ACIS_PERF_LEAVERS drops only that many, while the rest stay connected and
// watch them leave. ACIS_PERF_PROFILE_DIR records the CPU profile of the burst.
func TestMassDisconnect(t *testing.T) {
	clients := envInt(t, "ACIS_PERF_CLIENTS", 0)
	if clients <= 0 {
		t.Skip("set ACIS_PERF_CLIENTS to run the mass disconnect")
	}
	slow := &slowTasks{}
	srv := gameservertest.Boot(t,
		gameservertest.WithCharacter("Perf0", 5, 0),
		gameservertest.WithWantChars(1),
		gameservertest.WithProductionTickers(),
		gameservertest.WithRealPool(),
		gameservertest.WithLog(zerolog.New(slow)),
	)
	conns := []*testsupport.ScriptedClient{srv.Client}
	ids := []int32{srv.SoleObjectID(t)}
	for i := 1; i < clients; i++ {
		account := fmt.Sprintf("perf%d", i)
		ids = append(ids, srv.SeedCharacterFor(t, account, fmt.Sprintf("Perf%d", i), 5, 0).ID)
		conns = append(conns, srv.DialClient(t, account, 1))
	}
	readers := make([]*frameReader, clients)
	for i, c := range conns {
		enterWorld(t, c)
		readers[i] = startReader(c, ids[i])
	}

	leavers := min(envInt(t, "ACIS_PERF_LEAVERS", clients), clients)
	slow.reset()
	profile := startProfile(t)
	start := time.Now()
	for _, r := range readers[:leavers] {
		r.done.Store(true)
		_ = r.client.Close()
	}
	for i, r := range readers[:leavers] {
		<-r.stopped
		srv.AwaitConnectionEnded(t, conns[i])
	}
	burst := time.Since(start)
	profile.stop(t)
	for _, r := range readers[leavers:] {
		r.stop()
	}

	detaches := slow.detaches()
	t.Logf("clients=%d leavers=%d burst=%v slow-tasks=%d slow-detaches=%d", clients, leavers, burst, slow.count(), len(detaches))
	if len(detaches) > 0 {
		var sum time.Duration
		for _, d := range detaches {
			sum += d
		}
		t.Logf("slow detaches: min=%v median=%v max=%v sum=%v",
			detaches[0], detaches[len(detaches)/2], detaches[len(detaches)-1], sum)
	}
}

// slowTasks collects the watchdog's slow-task reports from the server log.
type slowTasks struct {
	mu    sync.Mutex
	tasks []slowTask
}

type slowTask struct {
	task    string
	elapsed time.Duration
}

func (s *slowTasks) Write(p []byte) (int, error) {
	var line struct {
		Message string  `json:"message"`
		Task    string  `json:"task"`
		Elapsed float64 `json:"elapsed"`
	}
	if json.Unmarshal(p, &line) == nil && line.Message == "sim: slow task" {
		s.mu.Lock()
		s.tasks = append(s.tasks, slowTask{line.Task, time.Duration(line.Elapsed * float64(time.Millisecond))})
		s.mu.Unlock()
	}
	return len(p), nil
}

func (s *slowTasks) reset() {
	s.mu.Lock()
	s.tasks = nil
	s.mu.Unlock()
}

func (s *slowTasks) count() int {
	s.mu.Lock()
	defer s.mu.Unlock()
	return len(s.tasks)
}

// detaches returns the elapsed times of the slow connection-end tasks, sorted.
func (s *slowTasks) detaches() []time.Duration {
	s.mu.Lock()
	defer s.mu.Unlock()
	var out []time.Duration
	for _, task := range s.tasks {
		if strings.Contains(task.task, "endConnection") {
			out = append(out, task.elapsed)
		}
	}
	slices.Sort(out)
	return out
}
