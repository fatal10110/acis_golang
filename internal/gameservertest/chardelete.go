package gameservertest

import "time"

// WithCharacterDeleteAfter sets the server.properties DeleteCharAfterDays
// grace period a deleted character waits before it is purged (default 7
// days); zero purges it as soon as it is deleted.
func WithCharacterDeleteAfter(d time.Duration) Option {
	return func(o *options) { o.characterDeleteAfter = d }
}
