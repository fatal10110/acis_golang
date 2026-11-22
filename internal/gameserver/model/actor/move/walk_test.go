package move

import (
	"reflect"
	"testing"
	"time"

	"github.com/fatal10110/acis_golang/internal/gameserver/model/location"
)

// WalkSteps is the UpdatePosition chain of a player mover on a queue clock:
// every position it lists is the one a hand-driven mover stands on after the
// same update, on open ground and against a wall.
func TestWalkStepsMatchesUpdatePositionChain(t *testing.T) {
	steps := []time.Duration{
		100 * time.Millisecond, 100 * time.Millisecond, 130 * time.Millisecond, 100 * time.Millisecond,
		100 * time.Millisecond, 100 * time.Millisecond, 250 * time.Millisecond, time.Millisecond, 100 * time.Millisecond,
	}
	target := location.Location{X: -1000, Y: 377}

	for name, geo := range map[string]Geo{
		"open": staticGeo{canMove: true},
		"wall": wallGeo(),
	} {
		t.Run(name, func(t *testing.T) {
			to, steps := target, steps
			if name == "wall" {
				to, steps = location.Location{X: 300, Y: 10}, []time.Duration{time.Second, time.Second, time.Second, time.Second, time.Second, time.Second, time.Second, time.Second}
			}
			mover, err := NewCreatureMove(location.Location{}, 115, geo, WithQueue(newMoveClock().q))
			if err != nil {
				t.Fatal(err)
			}
			mover.SetSpeeds(115, 60)
			if _, err := mover.MoveStraightToLocation(to); err != nil {
				t.Fatal(err)
			}
			var want []location.Location
			for _, step := range steps {
				_, moving := mover.UpdatePosition(step)
				want = append(want, mover.Position())
				if !moving {
					break
				}
			}

			got, err := WalkSteps(geo, location.Location{}, to, 115, 60, steps)
			if err != nil {
				t.Fatal(err)
			}
			if !reflect.DeepEqual(got.Points, want) {
				t.Fatalf("points = %v, want %v", got.Points, want)
			}
			if got.Stop != mover.Position() {
				t.Fatalf("stop = %v, want %v", got.Stop, mover.Position())
			}
			if wantBlocked := name == "wall"; got.Blocked != wantBlocked || got.Reached || len(got.Points) == 0 {
				t.Fatalf("blocked, reached = %v, %v; want blocked %v and not reached", got.Blocked, got.Reached, wantBlocked)
			}
		})
	}
}

func TestWalkStepsReachesDestination(t *testing.T) {
	to := location.Location{X: 100, Y: 0}
	got, err := WalkSteps(staticGeo{canMove: true}, location.Location{}, to, 200, 100, []time.Duration{
		time.Second, time.Second, time.Second, time.Second, time.Second, time.Second, time.Second, time.Second,
	})
	if err != nil {
		t.Fatal(err)
	}
	if !got.Reached || got.Blocked || got.Stop != to {
		t.Fatalf("outcome = %+v, want reached at %v", got, to)
	}
	if n := len(got.Points); n >= 8 || got.Points[n-1] != to {
		t.Fatalf("points = %v, want the chain to end on arrival", got.Points)
	}
}

func TestWalkStepsRejectsUnusableWalks(t *testing.T) {
	steps := []time.Duration{time.Second}
	geo := staticGeo{canMove: true}
	to := location.Location{X: 100}
	if _, err := WalkSteps(geo, location.Location{}, to, 0, 0, steps); err == nil {
		t.Error("zero speed: error = nil, want error")
	}
	if _, err := WalkSteps(geo, location.Location{}, to, -1, 0, steps); err == nil {
		t.Error("negative speed: error = nil, want error")
	}
}
