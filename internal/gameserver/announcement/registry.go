// Package announcement owns the server announcements: the lines read to
// every player entering the world, and the automatic ones repeated to every
// player online on their own schedule. Game masters add and delete them at
// run time; Save rewrites the announcement file after each change.
package announcement

import (
	"errors"
	"maps"
	"math"
	"slices"
	"sync"
	"time"

	"github.com/fatal10110/acis_golang/internal/gameserver/model/admin"
	"github.com/fatal10110/acis_golang/internal/gameserver/sim"
	"github.com/rs/zerolog"
)

// Announcer delivers one automatic announcement to every player online.
type Announcer interface {
	Announce(message string, critical bool)
}

// Store reads and rewrites the announcement file.
type Store interface {
	Load() ([]admin.Announcement, error)
	Save([]admin.Announcement) error
}

// Reasons Add refuses an announcement.
var (
	ErrEmptyMessage = errors.New("announcement: empty message")
	// ErrDelay refuses an automatic announcement whose delay is under a
	// second, which would repeat with no pause (#5126).
	ErrDelay = errors.New("announcement: automatic delay under one second")
	// ErrLimit refuses an automatic announcement with a negative limit,
	// which would never count down to its end (#5126).
	ErrLimit = errors.New("announcement: negative automatic limit")
)

// Indexed is an announcement with the index a game master deletes it by.
type Indexed struct {
	Index int
	admin.Announcement
}

// Registry holds the announcements by index and runs the automatic ones'
// schedules. All methods are safe for concurrent use.
//
// Each announcement added takes the index after the highest one handed out
// since the file was last loaded, so an index is never reused after a
// deletion and no announcement is ever replaced (#4707).
type Registry struct {
	store Store
	out   Announcer
	log   zerolog.Logger
	// queue runs the schedules: their timers and announcements.
	queue *sim.Queue

	// file serializes the reads and writes of the announcement file, so
	// they keep the order of the changes. mu is never held across them.
	file sync.Mutex
	// saved is the change the file was last written at, or read at. file
	// guards it.
	saved uint64

	// mu guards entries, next, change, stopped and every entry's schedule.
	mu      sync.Mutex
	entries map[int]*entry
	// next is the index the next announcement held takes.
	next int
	// change counts the changes Add and Delete made.
	change  uint64
	stopped bool
}

// entry is one held announcement and its schedule.
type entry struct {
	admin.Announcement
	timer *sim.Timer
	// run identifies the schedule a fired timer belongs to; stopping or
	// restarting the schedule moves it on, so a timer of an earlier run
	// that fires anyway does nothing.
	run uint64
	// unlimited repeats at a fixed rate from next; otherwise left counts
	// the announcements still to make.
	unlimited bool
	left      int32
	next      time.Time
}

// NewRegistry returns an empty registry reading and writing store and
// delivering through out; a nil store holds the announcements in memory
// only, and a nil out delivers nothing. The schedules run on queue, which
// the registry owns from then on.
func NewRegistry(store Store, out Announcer, log zerolog.Logger, queue *sim.Queue) *Registry {
	return &Registry{store: store, out: out, log: log, queue: queue, entries: map[int]*entry{}}
}

// Load reads the announcement file anew, indexing the announcements from 0
// in file order, and starts the automatic announcements' schedules. The
// announcements held before are dropped and their schedules stopped first;
// a file that fails to load is an error and leaves them held and running.
// An automatic announcement Add would refuse is skipped with a warning, so
// no stored entry starts a schedule without pause (#5126). A change not yet
// saved is saved first, and one made while the file is read is saved and the
// file read again, so Load reads every change made before it returns.
func (r *Registry) Load() error {
	r.file.Lock()
	defer r.file.Unlock()
	for {
		r.saveFile()
		r.mu.Lock()
		change := r.change
		r.mu.Unlock()
		var list []admin.Announcement
		if r.store != nil {
			var err error
			if list, err = r.store.Load(); err != nil {
				return err
			}
		}
		r.mu.Lock()
		if r.change != change {
			r.mu.Unlock()
			continue
		}
		r.loadLocked(list)
		r.mu.Unlock()
		return nil
	}
}

// loadLocked holds list in place of the announcements held.
func (r *Registry) loadLocked(list []admin.Announcement) {
	for _, e := range r.entries {
		e.stop()
	}
	clear(r.entries)
	r.next = 0
	if r.store == nil {
		return
	}
	for _, a := range list {
		if err := check(a); err != nil {
			r.log.Warn().Err(err).Str("message", a.Message).Int("delay", a.Delay).Int("limit", a.Limit).
				Msg("Skipped an automatic announcement.")
			continue
		}
		r.putLocked(a)
	}
	r.log.Info().Msgf("Loaded %d announcements.", len(r.entries))
}

// Login returns the announcements read to a player entering the world: the
// ones that are not automatic, by index.
func (r *Registry) Login() []admin.Announcement {
	r.mu.Lock()
	defer r.mu.Unlock()
	var out []admin.Announcement
	for _, i := range r.indexesLocked() {
		if e := r.entries[i]; !e.Auto {
			out = append(out, e.Announcement)
		}
	}
	return out
}

// List returns every held announcement by index.
func (r *Registry) List() []Indexed {
	r.mu.Lock()
	defer r.mu.Unlock()
	out := make([]Indexed, 0, len(r.entries))
	for _, i := range r.indexesLocked() {
		out = append(out, Indexed{Index: i, Announcement: r.entries[i].Announcement})
	}
	return out
}

// RestartAuto starts every automatic announcement's schedule over, from its
// initial delay and with its full count.
func (r *Registry) RestartAuto() {
	r.mu.Lock()
	defer r.mu.Unlock()
	for _, i := range r.indexesLocked() {
		r.startLocked(r.entries[i])
	}
}

// Add holds a new announcement and starts its schedule when automatic; Save
// then rewrites the file. An announcement that is not automatic keeps no
// schedule values. It refuses, changing nothing, an empty message
// (ErrEmptyMessage), and an automatic announcement with a delay under a
// second (ErrDelay) or a negative limit (ErrLimit).
func (r *Registry) Add(a admin.Announcement) error {
	if err := check(a); err != nil {
		return err
	}
	if !a.Auto {
		a = admin.Announcement{Message: a.Message, Critical: a.Critical}
	}
	r.mu.Lock()
	defer r.mu.Unlock()
	r.putLocked(a)
	r.change++
	return nil
}

// check reports why a cannot be held, or nil when it can. Only an automatic
// announcement's schedule values count: the others keep none.
func check(a admin.Announcement) error {
	switch {
	case a.Message == "":
		return ErrEmptyMessage
	case !a.Auto:
		return nil
	case a.Delay < 1:
		return ErrDelay
	case a.Limit < 0:
		return ErrLimit
	}
	return nil
}

// Delete drops the announcement at index and stops its schedule; Save then
// rewrites the file. It reports false, changing nothing, when no announcement holds
// index.
func (r *Registry) Delete(index int) bool {
	r.mu.Lock()
	defer r.mu.Unlock()
	e, ok := r.entries[index]
	if !ok {
		return false
	}
	delete(r.entries, index)
	e.stop()
	r.change++
	return true
}

// Stop ends every schedule and closes the queue; a stopped registry starts
// none again.
func (r *Registry) Stop() {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.stopped = true
	defer r.queue.Close()
	for _, e := range r.entries {
		e.stop()
	}
}

// putLocked holds a at the next index, one no announcement has held since
// the last load (#4707), and starts its schedule.
func (r *Registry) putLocked(a admin.Announcement) {
	e := &entry{Announcement: a}
	r.entries[r.next] = e
	r.next++
	r.startLocked(e)
}

func (r *Registry) indexesLocked() []int {
	return slices.Sorted(maps.Keys(r.entries))
}

// Save rewrites the announcement file with the announcements held, unless
// it already holds every change Add and Delete made. It reads and writes
// the file outside mu, so the players entering the world and the schedules
// do not wait on it; saves running at once write in turn, the last change
// last. A write that fails is logged.
func (r *Registry) Save() {
	r.file.Lock()
	defer r.file.Unlock()
	r.saveFile()
}

// saveFile is Save with file held.
func (r *Registry) saveFile() {
	r.mu.Lock()
	change := r.change
	if change == r.saved || r.store == nil {
		r.mu.Unlock()
		return
	}
	list := make([]admin.Announcement, 0, len(r.entries))
	for _, i := range r.indexesLocked() {
		list = append(list, r.entries[i].Announcement)
	}
	r.mu.Unlock()
	r.saved = change
	if err := r.store.Save(list); err != nil {
		r.log.Error().Err(err).Msg("Error regenerating XML.")
	}
}

// startLocked stops e's schedule and, when e is automatic, starts it anew:
// with no limit the first announcement comes after the initial delay and
// the next ones at a fixed rate of one per delay; with a limit, that many
// announcements come, the first after the initial delay and each next one
// a delay after the last. A negative initial delay counts as none; check
// keeps every held delay at a second or more and every limit at zero or
// more.
func (r *Registry) startLocked(e *entry) {
	e.stop()
	if !e.Auto || r.stopped {
		return
	}
	initial := max(seconds(e.InitialDelay), 0)
	if e.Limit == 0 {
		e.unlimited = true
		e.next = r.queue.Now().Add(initial)
	} else {
		e.left = clampInt32(e.Limit)
	}
	run := e.run
	e.timer = r.queue.After(initial, func() { r.fire(e, run) })
}

// fire makes e's due announcement and schedules the next one.
func (r *Registry) fire(e *entry, run uint64) {
	r.mu.Lock()
	if e.run != run {
		r.mu.Unlock()
		return
	}
	e.timer = nil
	if e.unlimited {
		e.next = e.next.Add(seconds(e.Delay))
		e.timer = r.queue.After(max(e.next.Sub(r.queue.Now()), 0), func() { r.fire(e, run) })
	} else {
		if e.left == 0 {
			r.mu.Unlock()
			return
		}
		e.left--
		if e.left != 0 {
			e.timer = r.queue.After(seconds(e.Delay), func() { r.fire(e, run) })
		}
	}
	message, critical := e.Message, e.Critical
	r.mu.Unlock()
	if r.out != nil {
		r.out.Announce(message, critical)
	}
}

// stop cancels e's pending announcement.
func (e *entry) stop() {
	e.run++
	if e.timer != nil {
		e.timer.Stop()
		e.timer = nil
	}
}

// clampInt32 narrows n to a 32-bit count. Both sources of a limit,
// the file loader and //announce add_auto, refuse a value out of that range,
// so the bounds, clamping, only guard against a new caller.
func clampInt32(n int) int32 {
	switch {
	case n > math.MaxInt32:
		return math.MaxInt32
	case n < math.MinInt32:
		return math.MinInt32
	}
	return int32(n)
}

func seconds(n int) time.Duration {
	return time.Duration(n) * time.Second
}
