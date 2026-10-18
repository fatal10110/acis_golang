package door

// WatchKind orders the watchers of a door's state changes: on each change
// every script watching the door runs first, then every maker.
type WatchKind uint8

// The watcher kinds, in the order a state change calls them.
const (
	WatchScript WatchKind = iota
	WatchMaker
	watchKinds
)

// WatchKinds is the number of watcher kinds.
const WatchKinds = int(watchKinds)
