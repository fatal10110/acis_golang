package gameservertest

import "github.com/fatal10110/acis_golang/internal/gameserver/model/observer"

// WithObserverGroups supplies the viewpoints broadcasting towers offer
// (default: none).
func WithObserverGroups(t *observer.Table) Option {
	return func(o *options) { o.observers = t }
}
