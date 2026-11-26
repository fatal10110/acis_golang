package persist

import (
	"bytes"
	"context"
	"errors"
	"strings"
	"testing"

	"github.com/rs/zerolog"
)

// laneRecorder keeps the jobs it is given per owner, or refuses them all.
type laneRecorder struct {
	refuse bool
	jobs   map[int32][]func()
}

func (r *laneRecorder) Enqueue(ownerID int32, job func()) bool {
	if r.refuse {
		return false
	}
	if r.jobs == nil {
		r.jobs = map[int32][]func(){}
	}
	r.jobs[ownerID] = append(r.jobs[ownerID], job)
	return true
}

func TestServiceWriteQueuesOnTheServiceLane(t *testing.T) {
	var logged bytes.Buffer
	writes := &laneRecorder{}
	ran := false
	if !ServiceWrite(writes, zerolog.New(&logged), "svc: save", func(ctx context.Context) error {
		if _, ok := ctx.Deadline(); !ok {
			t.Error("the write runs with no deadline")
		}
		ran = true
		return errors.New("boom")
	}) {
		t.Fatal("ServiceWrite reported a queued write dropped")
	}
	if ran || len(writes.jobs) != 1 || len(writes.jobs[ServiceOwner]) != 1 {
		t.Fatalf("ran=%v jobs=%v, want one job queued on ServiceOwner and not run", ran, writes.jobs)
	}
	writes.jobs[ServiceOwner][0]()
	if !ran || !strings.Contains(logged.String(), `"error":"boom","message":"svc: save"`) {
		t.Fatalf("ran=%v log=%q, want the job run and its error logged as svc: save", ran, logged.String())
	}
}

func TestServiceWriteRunsAtOnceWithoutAWriter(t *testing.T) {
	var logged bytes.Buffer
	ran := false
	if !ServiceWrite(nil, zerolog.New(&logged), "svc: save", func(context.Context) error { ran = true; return nil }) {
		t.Fatal("ServiceWrite without a writer reported the write dropped")
	}
	if !ran || logged.Len() != 0 {
		t.Fatalf("ran=%v log=%q, want the write run at once and nothing logged", ran, logged.String())
	}
}

func TestServiceWriteLogsADroppedWrite(t *testing.T) {
	var logged bytes.Buffer
	ran := false
	if ServiceWrite(&laneRecorder{refuse: true}, zerolog.New(&logged), "svc: save", func(context.Context) error { ran = true; return nil }) {
		t.Fatal("ServiceWrite reported a refused write queued")
	}
	if ran || !strings.Contains(logged.String(), `"message":"svc: save: write dropped"`) {
		t.Fatalf("ran=%v log=%q, want the write not run and logged as dropped", ran, logged.String())
	}
}
