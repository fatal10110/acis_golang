package gameservertest

import (
	"testing"

	"github.com/fatal10110/acis_golang/internal/gameserver/persist"
)

// TestParkedOnHeldLaneNeedsTheWaitOnAHeldLane pins what excuses a frame the
// server has not finished: only its handler waiting for saves on a lane the
// test holds. A held lane alone excuses nothing, so a busy handler on
// another connection is still waited for.
func TestParkedOnHeldLaneNeedsTheWaitOnAHeldLane(t *testing.T) {
	const heldOwner = 1
	other := int32(heldOwner + 1)
	for persist.LaneIndex(other) == persist.LaneIndex(heldOwner) {
		other++
	}
	s := &Server{}
	s.heldLanes[persist.LaneIndex(heldOwner)].Add(1)

	cases := []struct {
		name   string
		parked *[]int32
		want   bool
	}{
		{"not waiting", nil, false},
		{"waiting on an unheld lane", &[]int32{other}, false},
		{"waiting on the held lane", &[]int32{other, heldOwner}, true},
		{"waiting on every lane", &[]int32{}, true},
	}
	for _, tc := range cases {
		ct := new(connTraffic)
		ct.parked.Store(tc.parked)
		if got := s.parkedOnHeldLane(ct); got != tc.want {
			t.Errorf("%s: parkedOnHeldLane = %v, want %v", tc.name, got, tc.want)
		}
	}

	s.heldLanes[persist.LaneIndex(heldOwner)].Add(-1)
	ct := new(connTraffic)
	ct.parked.Store(&[]int32{heldOwner})
	if s.parkedOnHeldLane(ct) {
		t.Error("a released lane still excuses the handler waiting on it")
	}
}
