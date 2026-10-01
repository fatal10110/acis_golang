package move

import (
	"testing"

	"github.com/fatal10110/acis_golang/internal/gameserver/model/actor/event"
	"github.com/fatal10110/acis_golang/internal/gameserver/model/location"
)

// headingSelf is a non-player walker that records the heading it faces at
// each movement broadcast.
type headingSelf struct {
	summonChaseSelf
	heading     int
	atBroadcast []int
}

func (s *headingSelf) SetHeading(h int) { s.heading = h }

func (s *headingSelf) BroadcastMove(ev event.Move) {
	s.atBroadcast = append(s.atBroadcast, s.heading)
	s.summonChaseSelf.BroadcastMove(ev)
}

// Every walk start faces the leg it starts before the movement is broadcast
// (CreatureMove.moveToLocation: setHeadingTo(destination) ahead of the
// MoveToLocation broadcast; PlayerMove.maybeMoveToPawn: setHeadingTo(tx, ty)
// ahead of MoveToPawn), whatever heading the walker had before. A routed
// walk faces its first waypoint, and a same-cell request faces heading 0
// (MathUtil.calculateHeadingFrom of a zero vector).
func TestControllerMoveStartFacesDestination(t *testing.T) {
	const staleHeading = 12345
	east := location.Location{}.HeadingTo(location.Location{X: 300})
	north := location.Location{}.HeadingTo(location.Location{Y: 300})
	south := location.Location{}.HeadingTo(location.Location{Y: -300})
	west := location.Location{}.HeadingTo(location.Location{X: -300})
	northEast := location.Location{}.HeadingTo(location.Location{X: 100, Y: 100})
	southWest := location.Location{}.HeadingTo(location.Location{X: -300, Y: -300})

	tests := []struct {
		name  string
		geo   Geo
		start func(*Controller) bool
		want  int
	}{
		{
			name: "MoveToLocation",
			start: func(c *Controller) bool {
				ok, err := c.MoveToLocation(location.Location{Y: 300})
				return ok && err == nil
			},
			want: north,
		},
		{
			name: "MoveToLocationEvent",
			start: func(c *Controller) bool {
				_, err := c.MoveToLocationEvent(location.Location{Y: 300})
				return err == nil
			},
			want: north,
		},
		{
			name:  "MoveHome",
			start: func(c *Controller) bool { return c.MoveHome(location.Location{Y: -300}) == nil },
			want:  south,
		},
		{
			name: "routed walk faces its first waypoint",
			geo: &recordingGeo{
				findPath:   []location.Location{{X: 100, Y: 100}, {Y: 300}},
				findPathOK: true,
			},
			start: func(c *Controller) bool {
				ok, err := c.MoveToLocation(location.Location{Y: 300})
				return ok && err == nil
			},
			want: northEast,
		},
		{
			name: "same-cell request",
			start: func(c *Controller) bool {
				ok, err := c.MoveToLocation(location.Location{})
				return ok && err == nil
			},
			want: 0,
		},
		{
			name: "chase",
			start: func(c *Controller) bool {
				following, err := c.MaybeStartOffensiveFollow(&followTarget{x: -300}, 40)
				return following && err == nil
			},
			want: west,
		},
		{
			name: "friendly follow",
			start: func(c *Controller) bool {
				following, err := c.MaybeStartFriendlyFollow(&followTarget{x: -300, y: -300}, 40)
				return following && err == nil
			},
			want: southWest,
		},
		{
			name:  "MoveToPawn",
			start: func(c *Controller) bool { return c.MoveToPawn(&followTarget{x: 300}, 40) },
			want:  east,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			geo := tt.geo
			if geo == nil {
				geo = staticGeo{canMove: true}
			}
			self := &headingSelf{heading: staleHeading}
			controller, _, _, _ := newChaseController(t, self, geo)
			if !tt.start(controller) {
				t.Fatal("walk not accepted")
			}
			if len(self.atBroadcast) != 1 || self.atBroadcast[0] != tt.want {
				t.Fatalf("heading at the move broadcast = %v, want [%d] (stale heading %d)", self.atBroadcast, tt.want, staleHeading)
			}
			if self.heading != tt.want {
				t.Fatalf("heading after the walk start = %d, want %d", self.heading, tt.want)
			}
		})
	}
}

// A walk refused at the start leaves the heading alone: only an accepted
// walk turns the walker.
func TestControllerRefusedMoveKeepsHeading(t *testing.T) {
	const staleHeading = 12345
	self := &headingSelf{heading: staleHeading}
	mover, err := NewCreatureMove(location.Location{}, 0, staticGeo{canMove: true})
	if err != nil {
		t.Fatal(err)
	}
	mover.SetQueue(newMoveClock().q)
	controller, err := NewController(mover, self, &eventLog{})
	if err != nil {
		t.Fatal(err)
	}
	if ok, _ := controller.MoveToLocation(location.Location{Y: 300}); ok {
		t.Fatal("MoveToLocation() at zero speed accepted, want refused")
	}
	if self.heading != staleHeading || len(self.atBroadcast) != 0 {
		t.Fatalf("heading = %d with broadcasts %v, want %d and none", self.heading, self.atBroadcast, staleHeading)
	}
}
