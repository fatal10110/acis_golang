package persist

import (
	"context"
	"time"

	"github.com/rs/zerolog"
)

// ServiceTaskTimeout bounds one database job a game-wide service runs.
const ServiceTaskTimeout = 10 * time.Second

// Enqueuer queues a job on an owner's lane, reporting false when it refused
// the job.
type Enqueuer interface {
	Enqueue(ownerID int32, job func()) bool
}

// ServiceJob returns a job that runs fn under ServiceTaskTimeout and logs
// fn's error as msg.
func ServiceJob(log zerolog.Logger, msg string, fn func(context.Context) error) func() {
	return func() {
		ctx, cancel := context.WithTimeout(context.Background(), ServiceTaskTimeout)
		defer cancel()
		if err := fn(ctx); err != nil {
			log.Error().Err(err).Msg(msg)
		}
	}
}

// ServiceWrite queues fn as a ServiceJob on the ServiceOwner lane of
// writes, or runs it at once when writes is nil. One lane keeps a
// service's writes in the order they were made, and no player's saves wait
// behind them. It reports false, after logging msg + ": write dropped",
// when writes refused the job.
func ServiceWrite(writes Enqueuer, log zerolog.Logger, msg string, fn func(context.Context) error) bool {
	job := ServiceJob(log, msg, fn)
	if writes == nil {
		job()
		return true
	}
	if !writes.Enqueue(ServiceOwner, job) {
		log.Error().Msg(msg + ": write dropped")
		return false
	}
	return true
}
