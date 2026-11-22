package sim

import (
	"context"
	"testing"

	"github.com/rs/zerolog"
)

// BenchmarkPoolPostDrain measures one post through a pool to its task
// running: on one queue, and spread over many queues so the run queue and
// per-queue buffers cycle under contention.
func BenchmarkPoolPostDrain(b *testing.B) {
	for _, bc := range []struct {
		name   string
		queues int
	}{{"one-queue", 1}, {"1024-queues", 1024}} {
		b.Run(bc.name, func(b *testing.B) {
			p := NewPool(0, zerolog.Nop())
			p.Start(context.Background())
			defer func() { _ = p.Stop(context.Background()) }()
			qs := make([]*Queue, bc.queues)
			for i := range qs {
				qs[i] = p.NewQueue("q")
			}
			done := make(chan struct{}, len(qs))
			task := func() { done <- struct{}{} }
			b.ReportAllocs()
			for b.Loop() {
				for _, q := range qs {
					q.Post(task)
				}
				for range qs {
					<-done
				}
			}
			b.ReportMetric(float64(b.Elapsed().Nanoseconds())/float64(b.N*len(qs)), "ns/task")
		})
	}
}
