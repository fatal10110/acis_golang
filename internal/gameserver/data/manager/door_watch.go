package manager

import (
	"slices"

	"github.com/fatal10110/acis_golang/internal/gameserver/model/door"
)

// doorWatcher is one watcher of a door's state changes: key names who
// registered it, fn is called with the door after each change.
type doorWatcher struct {
	key any
	fn  func(*door.Object)
}

// doorWatchers are the watchers of one door, by kind, each kind in
// registration order.
type doorWatchers [door.WatchKinds][]doorWatcher

// WatchDoor has fn called with id's standing door after each later change
// of its open state, after the watchers of kinds ordered before kind and
// after the ones of kind registered before it. A key already watching that
// door under kind is moved last, with fn. The registration belongs to the
// standing door: a door a reload builds has none. It reports whether the
// door stands in the world.
//
// fn runs on the goroutine that changed the door, with no door lock held,
// at the change's place: after the door's status broadcast and before the
// door it triggers changes.
func (w *WorldObjects) WatchDoor(id int, kind door.WatchKind, key any, fn func(*door.Object)) bool {
	if w == nil {
		return false
	}
	w.doorMu.Lock()
	defer w.doorMu.Unlock()
	obj, ok := w.Door(id)
	if !ok {
		return false
	}
	if w.watchers == nil {
		w.watchers = make(map[*door.Object]*doorWatchers)
	}
	ws := w.watchers[obj]
	if ws == nil {
		ws = &doorWatchers{}
		w.watchers[obj] = ws
	}
	list := slices.DeleteFunc(ws[kind], func(o doorWatcher) bool { return o.key == key })
	ws[kind] = append(list, doorWatcher{key: key, fn: fn})
	return true
}

// doorRun says where a door change's watcher calls run: in place, with
// doorMu released around them, or queued for the caller to run once it
// releases doorMu.
type doorRun struct {
	inline bool
	queued []func()
}

// inlineDoorRun runs watcher calls in place.
func inlineDoorRun() *doorRun { return &doorRun{inline: true} }

// fire runs the queued watcher calls; the caller does not hold doorMu.
func (r *doorRun) fire() {
	for _, fn := range r.queued {
		fn()
	}
}

// watchDoorChange calls obj's watchers, which its state change just
// broadcast, as run says; the caller holds doorMu.
func (w *WorldObjects) watchDoorChange(obj *door.Object, run *doorRun) {
	ws := w.watchers[obj]
	if ws == nil {
		return
	}
	var calls []func(*door.Object)
	for _, list := range ws {
		for _, o := range list {
			calls = append(calls, o.fn)
		}
	}
	if len(calls) == 0 {
		return
	}
	call := func() {
		for _, fn := range calls {
			fn(obj)
		}
	}
	if !run.inline {
		run.queued = append(run.queued, call)
		return
	}
	w.doorMu.Unlock()
	defer w.doorMu.Lock()
	call()
}
