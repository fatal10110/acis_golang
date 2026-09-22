package gameservertest

import (
	"context"
	"errors"
	"sync/atomic"

	"github.com/fatal10110/acis_golang/internal/gameserver/model/item"
	"github.com/fatal10110/acis_golang/internal/gameserver/task"
)

// ErrItemFlushFault is what an armed ItemFlushFault fails a flush with.
var ErrItemFlushFault = errors.New("gameservertest: item flush fault")

// ItemFlushFault fails every item persistence flush while it is armed, so a
// suite can put the server on the path a refused or timed-out write takes —
// the detach flush that keeps its container pending instead of dropping it —
// without needing a database that actually misbehaves.
//
// Arm and Disarm are safe to call from the suite goroutine while the server
// writes on its own.
type ItemFlushFault struct {
	armed atomic.Bool
}

// Arm makes every later flush fail until Disarm.
func (f *ItemFlushFault) Arm() { f.armed.Store(true) }

// Disarm lets flushes reach the database again.
func (f *ItemFlushFault) Disarm() { f.armed.Store(false) }

// WithItemFlushFault routes item persistence through fault, which the suite
// arms and disarms around the writes it wants to fail.
func WithItemFlushFault(fault *ItemFlushFault) Option {
	return func(o *options) { o.itemFlushFault = fault }
}

// faultyItemFlusher is the flusher wrapper WithItemFlushFault installs.
type faultyItemFlusher struct {
	inner task.ItemFlusher
	fault *ItemFlushFault
}

func (f faultyItemFlusher) Flush(ctx context.Context, batch item.FlushBatch) error {
	if f.fault.armed.Load() {
		return ErrItemFlushFault
	}
	return f.inner.Flush(ctx, batch)
}
