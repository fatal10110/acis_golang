package task

import (
	"fmt"
	"sync"
	"time"

	"github.com/rs/zerolog"

	"github.com/fatal10110/acis_golang/internal/commons/scheduler"
)

const (
	// GameMinute is how much real time one in-game minute takes: one in-game
	// day lasts four real hours, so its 1440 minutes pass one per ten real
	// seconds. GameClock.Start advances the clock at this period.
	GameMinute = 10 * time.Second

	minutesPerDay  = 24 * 60 // in-game minutes in one in-game day
	nightEndMinute = 6 * 60  // day breaks at 06:00; night spans 00:00-05:59
)

// GameClock tracks in-game time, which runs six times faster than real time.
// The clock is aligned so in-game midnight coincides with the local midnight
// preceding the current time, at construction and again by Start, then
// advances one in-game minute per Tick.
// Night spans 00:00-05:59 in-game.
//
// All methods are safe for concurrent use; mu guards minutes, night and the
// listener slices. Tick runs on the game-minute ticker while every actor
// queue reads the time of day.
type GameClock struct {
	now   func() time.Time
	start time.Time

	mu       sync.RWMutex
	minutes  int  // in-game minutes elapsed since the midnight alignment
	night    bool // night state as of the last Tick, kept to detect boundary crossings
	minute   []func()
	dayNight []func(night bool)
}

// NewGameClock returns a clock aligned to the local midnight preceding the
// current time. now is the wall-clock source, also used by Uptime; pass
// time.Now in production. A nil now defaults to time.Now.
func NewGameClock(now func() time.Time) *GameClock {
	if now == nil {
		now = time.Now
	}
	n := now()
	c := &GameClock{now: now, start: n}
	c.align(n)
	return c
}

// align sets the clock to the in-game minutes elapsed between the local
// midnight preceding n and n. Callers hold mu or own c exclusively.
func (c *GameClock) align(n time.Time) {
	midnight := time.Date(n.Year(), n.Month(), n.Day(), 0, 0, 0, 0, n.Location())
	c.minutes = int(n.Sub(midnight) / GameMinute)
	c.night = c.minutes%minutesPerDay < nightEndMinute
}

// Start realigns the clock to the current time and starts the ticker that
// advances it one in-game minute per GameMinute from this instant on, so the
// time spent booting between construction and Start is not lost. The
// realignment fires no listener. The ticker runs at a fixed rate: ticks a
// stall delays all still run, so the clock stays locked to wall time.
func (c *GameClock) Start(log zerolog.Logger) *scheduler.Ticker {
	c.mu.Lock()
	c.align(c.now())
	c.mu.Unlock()
	return scheduler.Start(GameMinute, c.Tick, log)
}

// Tick advances the clock by one in-game minute and, when that minute crosses
// the day/night boundary, invokes the OnDayNight listeners. Call it once per
// GameMinute of real time for the clock to keep its intended pace.
func (c *GameClock) Tick() {
	c.mu.Lock()
	c.minutes++
	night := c.minutes%minutesPerDay < nightEndMinute
	crossed := night != c.night
	c.night = night
	minuteListeners := append([]func(){}, c.minute...)
	var listeners []func(night bool)
	if crossed {
		listeners = append(listeners, c.dayNight...)
	}
	c.mu.Unlock()

	// Listeners run outside the lock so they can read the clock.
	for _, fn := range listeners {
		fn(night)
	}
	for _, fn := range minuteListeners {
		fn()
	}
}

// OnDayNight registers fn to run at every day/night boundary crossing, with
// night true when night has just fallen. fn runs on the goroutine that called
// Tick and must not block it for long.
func (c *GameClock) OnDayNight(fn func(night bool)) {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.dayNight = append(c.dayNight, fn)
}

// OnMinute registers fn to run after every in-game minute advance. Day/night
// boundary listeners run before minute listeners on a crossing tick.
func (c *GameClock) OnMinute(fn func()) {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.minute = append(c.minute, fn)
}

// TimeOfDay returns the minute of the current in-game day, 0-1439.
func (c *GameClock) TimeOfDay() int {
	c.mu.RLock()
	defer c.mu.RUnlock()
	return c.minutes % minutesPerDay
}

// Minutes returns the absolute number of in-game minutes elapsed since the
// midnight alignment. Use this rather than TimeOfDay when a
// caller needs a monotonic counter that increases across in-game days
// (for example, to schedule periodic reminders that survive the wrap).
func (c *GameClock) Minutes() int {
	c.mu.RLock()
	defer c.mu.RUnlock()
	return c.minutes
}

// Hour returns the in-game hour, 0-23.
func (c *GameClock) Hour() int {
	c.mu.RLock()
	defer c.mu.RUnlock()
	return (c.minutes % minutesPerDay) / 60
}

// Minute returns the in-game minute of the hour, 0-59.
func (c *GameClock) Minute() int {
	c.mu.RLock()
	defer c.mu.RUnlock()
	return c.minutes % 60
}

// Day returns how many whole in-game days have elapsed since the midnight
// alignment.
func (c *GameClock) Day() int {
	c.mu.RLock()
	defer c.mu.RUnlock()
	return c.minutes / minutesPerDay
}

// IsNight reports whether it is currently night in-game (00:00-05:59).
func (c *GameClock) IsNight() bool {
	c.mu.RLock()
	defer c.mu.RUnlock()
	return c.minutes%minutesPerDay < nightEndMinute
}

// String returns the in-game time of day as zero-padded "HH:MM".
func (c *GameClock) String() string {
	c.mu.RLock()
	defer c.mu.RUnlock()
	t := c.minutes % minutesPerDay
	return fmt.Sprintf("%02d:%02d", t/60, t%60)
}

// Uptime returns the real time elapsed since the clock was constructed.
func (c *GameClock) Uptime() time.Duration {
	return c.now().Sub(c.start)
}
