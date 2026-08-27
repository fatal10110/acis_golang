package world

import "testing"

// spawnOrderObject is a minimal Tracked+Observer double for asserting the
// order of State.Spawn's registry write against its Discover callbacks.
type spawnOrderObject struct {
	Presence
	id int32
	on func(obj Tracked)
}

func (o *spawnOrderObject) ObjectID() int32   { return o.id }
func (o *spawnOrderObject) Discover(t Tracked) {
	if o.on != nil {
		o.on(t)
	}
}
func (o *spawnOrderObject) Forget(Tracked) {}

// TestSpawn_NotResolvableDuringOwnDiscoverCallbacks matches the Java oracle
// (WorldObject.spawnMe: setRegion before World.addObject): an object being
// spawned must not be resolvable via State.Object from inside the Discover
// callbacks its own arrival triggers, since it is not yet registered.
func TestSpawn_NotResolvableDuringOwnDiscoverCallbacks(t *testing.T) {
	s := New()

	observer := &spawnOrderObject{id: 1}
	s.Spawn(observer, 0, 0, 0, 0)

	var foundDuringDiscover bool
	arriving := &spawnOrderObject{id: 2}
	observer.on = func(obj Tracked) {
		if obj.ObjectID() != arriving.id {
			return
		}
		_, foundDuringDiscover = s.Object(arriving.id)
	}

	s.Spawn(arriving, 0, 0, 0, 0)

	if foundDuringDiscover {
		t.Error("arriving object was resolvable via State.Object during its own Discover callbacks; Java resolves it only after setRegion returns")
	}
	if _, ok := s.Object(arriving.id); !ok {
		t.Error("arriving object not registered after Spawn returns")
	}
}
