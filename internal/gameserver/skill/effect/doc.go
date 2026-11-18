// Package effect applies skill effects.
//
// # Tick scheduling
//
// A periodic effect does not own a timer. Its schedule is a due instant
// (anchored on the instant it started, advanced by exactly one period per
// tick) plus a remaining tick count. The server's fixed live-effect sweep
// (task.EffectTick, 1s) calls List.Tick on each active list's owner queue,
// and every effect whose due instant has passed runs one tick.
//
// Parity exception (owner decision, #4932): the first tick of an effect
// should run one period after it starts and later ticks at a fixed rate of
// the period, each at its exact due instant. Under the sweep a tick, and the
// expiry its last tick causes, may land up to one sweep interval after its
// due instant. The lateness is bounded and does not accumulate: the next due
// instant advances from the previous due instant, not from the late claim,
// and the tick count stays exact. Per-effect timers are out of scope.
package effect
