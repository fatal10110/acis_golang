// Package announcement owns the server announcements: the lines read to
// every player entering the world, and the automatic ones repeated to every
// player online on their own schedule. Game masters add and delete them at
// run time; every change rewrites the announcement file.
package announcement

import (
	"maps"
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

// Indexed is an announcement with the index a game master deletes it by.
type Indexed struct {
	Index int
	admin.Announcement
}

// Registry holds the announcements by index and runs the automatic ones'
// schedules. All methods are safe for concurrent use.
//
// An announcement takes the index equal to the number held when it is
// added, so after a deletion a new one can take an index still in use and
// replace that announcement; a replaced automatic announcement keeps
// repeating until the server stops.
type Registry struct {
	store Store
	out   Announcer
	log   zerolog.Logger
	// queue runs the schedules: their timers and announcements.
	queue *sim.Queue

	// mu guards entries, orphans, stopped and every entry's schedule.
	mu      sync.Mutex
	entries map[int]*entry
	orphans []*entry
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
	// the announcements still to make, a negative count never reaching
	// zero before it wraps.
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

// Load reads the announcement file anew and starts the automatic
// announcements' schedules. The announcements held before are dropped and
// their schedules stopped first; a file that fails to load leaves none.
func (r *Registry) Load() error {
	r.mu.Lock()
	defer r.mu.Unlock()
	for _, e := range r.entries {
		e.stop()
	}
	clear(r.entries)
	if r.store == nil {
		return nil
	}
	list, err := r.store.Load()
	if err != nil {
		return err
	}
	for _, a := range list {
		r.putLocked(a)
	}
	r.log.Info().Msgf("Loaded %d announcements.", len(r.entries))
	return nil
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

// Add holds a new announcement, starts its schedule when automatic and
// rewrites the file. An announcement that is not automatic keeps no
// schedule values. An empty message is refused.
func (r *Registry) Add(a admin.Announcement) bool {
	if a.Message == "" {
		return false
	}
	if !a.Auto {
		a = admin.Announcement{Message: a.Message, Critical: a.Critical}
	}
	r.mu.Lock()
	defer r.mu.Unlock()
	r.putLocked(a)
	r.saveLocked()
	return true
}

// Delete drops the announcement at index, stops its schedule and rewrites
// the file. It reports false, changing nothing, when no announcement holds
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
	r.saveLocked()
	return true
}

// Stop ends every schedule, replaced announcements' too, and closes the
// queue; a stopped registry starts none again.
func (r *Registry) Stop() {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.stopped = true
	defer r.queue.Close()
	for _, e := range r.entries {
		e.stop()
	}
	for _, e := range r.orphans {
		e.stop()
	}
	r.orphans = nil
}

// putLocked holds a at the next index and starts its schedule. An
// announcement already at that index is replaced, its schedule left
// running.
func (r *Registry) putLocked(a admin.Announcement) {
	index := len(r.entries)
	if old, ok := r.entries[index]; ok && old.timer != nil {
		r.orphans = append(r.orphans, old)
	}
	e := &entry{Announcement: a}
	r.entries[index] = e
	r.startLocked(e)
}

func (r *Registry) indexesLocked() []int {
	return slices.Sorted(maps.Keys(r.entries))
}

func (r *Registry) saveLocked() {
	if r.store == nil {
		return
	}
	list := make([]admin.Announcement, 0, len(r.entries))
	for _, i := range r.indexesLocked() {
		list = append(list, r.entries[i].Announcement)
	}
	if err := r.store.Save(list); err != nil {
		r.log.Error().Err(err).Msg("Error regenerating XML.")
	}
}

// startLocked stops e's schedule and, when e is automatic, starts it anew:
// with no limit the first announcement comes after the initial delay and
// the next ones at a fixed rate of one per delay, a delay that is not
// positive starting nothing; with a limit, that many announcements come,
// the first after the initial delay and each next one a delay after the
// last. A negative delay counts as none.
func (r *Registry) startLocked(e *entry) {
	e.stop()
	if !e.Auto || r.stopped {
		return
	}
	initial := max(seconds(e.InitialDelay), 0)
	if e.Limit == 0 {
		e.unlimited = true
		if seconds(e.Delay) <= 0 {
			return
		}
		e.next = r.queue.Now().Add(initial)
	} else {
		e.left = int32(e.Limit)
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
			e.timer = r.queue.After(max(seconds(e.Delay), 0), func() { r.fire(e, run) })
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

func seconds(n int) time.Duration {
	return time.Duration(n) * time.Second
}
