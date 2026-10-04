package gameservertest

import "github.com/fatal10110/acis_golang/internal/gameserver/schemebuffer"

// WithSchemeBuffer wires buffer as the scheme buffer's buffs and the
// players' schemes (default: none offered, none kept).
func WithSchemeBuffer(buffer *schemebuffer.Manager) Option {
	return func(o *options) { o.schemeBuffer = buffer }
}
