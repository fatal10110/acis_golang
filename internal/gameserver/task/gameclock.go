package task

import (
	"fmt"
	"sync"
	"time"
)

const (
	// GameMinute is how much real time one in-game minute takes: one in-game
	// day lasts four real hours, so its 1440 minutes pass one per ten real
	// seconds. Advance a GameClock at this period, e.g.
	// scheduler.Start(task.GameMinute, clock.Tick, log).
	GameMinute = 10 * time.Second

	minutesPerDay  = 24 * 60 // in-game minutes in one in-game day
	nightEndMinute = 6 * 60  // day breaks at 06:00; night spans 00:00-05:59
)

// GameClock tracks in-game time, which runs six times faster than real time.
// The clock is aligned at construction so in-game midnight coincides with the
// local midnight preceding boot, then advances one in-game minute per Tick.
// Night spans 00:00-05:59 in-game.
//
// All methods are safe for concurrent use; mu guards minutes, night and
// dayNight.
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
	midnight := time.Date(n.Year(), n.Month(), n.Day(), 0, 0, 0, 0, n.Location())
	minutes := int(n.Sub(midnight) / GameMinute)
	return &GameClock{
		now:     now,
		start:   n,
		minutes: minutes,
		night:   minutes%minutesPerDay < nightEndMinute,
	}
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
// midnight alignment at construction. Use this rather than TimeOfDay when a
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
// alignment at construction.
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
