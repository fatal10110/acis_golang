package move

import (
	"testing"

	"github.com/fatal10110/acis_golang/internal/gameserver/model/actor/event"
	"github.com/fatal10110/acis_golang/internal/gameserver/model/location"
)

// stepHookSink runs onStep when a debugged step is reported: between the
// step being computed and the controller syncing it, the window an
// off-queue teleport can land in.
type stepHookSink struct {
	eventLog
	onStep func()
}

func (s *stepHookSink) Emit(e event.Event) {
	s.eventLog.Emit(e)
	if _, ok := e.(event.MoveDebugStep); ok && s.onStep != nil {
		s.onStep()
	}
}

// A teleport that lands after a walk's step is computed but before it is
// synced wins: the stale step never moves the actor away from where the
// teleport put it, whether the teleport aborted the walk first or only
// reset the actor's position. Without one the step is synced as usual.
func TestPositionUpdateDropsStepOvertakenByTeleport(t *testing.T) {
	target := location.Location{X: 5000, Y: 5000}
	tests := []struct {
		name     string
		teleport func(c *Controller, m *CreatureMove, self Actor)
		want     func(step location.Location) location.Location
	}{
		{
			name: "teleport aborting the walk",
			teleport: func(c *Controller, m *CreatureMove, self Actor) {
				c.Stop()
				m.SetPosition(target)
				self.SyncPosition(target)
			},
			want: func(location.Location) location.Location { return target },
		},
		{
			name: "position reset during the walk",
			teleport: func(_ *Controller, m *CreatureMove, self Actor) {
				m.SetPosition(target)
				self.SyncPosition(target)
			},
			want: func(location.Location) location.Location { return target },
		},
		{
			name: "no teleport",
			want: func(step location.Location) location.Location { return step },
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			self := &chaseSelf{}
			mover, err := NewCreatureMove(location.Location{}, 100, staticGeo{canMove: true}, WithQueue(newMoveClock().q))
			if err != nil {
				t.Fatal(err)
			}
			sink := &stepHookSink{}
			controller, err := NewController(mover, self, sink, nil)
			if err != nil {
				t.Fatal(err)
			}
			if !mover.ToggleDebug(false) {
				t.Fatal("move debugging not on")
			}
			if ok, _ := controller.MoveToLocation(location.Location{X: 1000}); !ok {
				t.Fatal("walk not accepted")
			}
			var step location.Location
			sink.onStep = func() {
				step = mover.Position()
				if tt.teleport != nil {
					tt.teleport(controller, mover, self)
				}
			}
			controller.PositionUpdate()
			if step == (location.Location{}) {
				t.Fatal("the walk took no step")
			}
			x, y, z := self.Position()
			if got, want := (location.Location{X: x, Y: y, Z: z}), tt.want(step); got != want {
				t.Fatalf("presence at %+v after the update, want %+v (step %+v)", got, want, step)
			}
		})
	}
}
