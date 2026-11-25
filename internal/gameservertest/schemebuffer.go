package gameservertest

import (
	"context"
	"database/sql"
	"testing"

	gamesql "github.com/fatal10110/acis_golang/internal/gameserver/data/sql"
	"github.com/fatal10110/acis_golang/internal/gameserver/persist"
	"github.com/fatal10110/acis_golang/internal/gameserver/schemebuffer"
	"github.com/rs/zerolog"
)

// WithSchemeBuffer wires buffer as the scheme buffer's buffs and the
// players' schemes (default: none offered, none kept).
func WithSchemeBuffer(buffer *schemebuffer.Manager) Option {
	return func(o *options) { o.schemeBuffer = buffer }
}

// WithStoredSchemeBuffer wires the scheme buffer newBuffer returns when
// given store: the fixture database's buffer_schemes written on the
// server's persistence lanes. The buffer is restored from those rows as a
// server start restores it.
func WithStoredSchemeBuffer(newBuffer func(store schemebuffer.Option) *schemebuffer.Manager) Option {
	return func(o *options) { o.newSchemeBuffer = newBuffer }
}

// storedSchemeBuffer builds and restores the buffer WithStoredSchemeBuffer
// asked for, or returns buffer without it.
func storedSchemeBuffer(tb testing.TB, newBuffer func(schemebuffer.Option) *schemebuffer.Manager, buffer *schemebuffer.Manager, db *sql.DB, worker *persist.Worker, log zerolog.Logger) *schemebuffer.Manager {
	tb.Helper()
	if newBuffer == nil {
		return buffer
	}
	store := gamesql.NewBufferSchemeStore(db)
	buffer = newBuffer(schemebuffer.WithStore(store, worker, log))
	rows, err := store.Load(context.Background())
	if err != nil {
		tb.Fatalf("load buffer schemes: %v", err)
	}
	buffer.Restore(rows, log)
	return buffer
}
