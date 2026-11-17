package location

import "testing"

// IsFacing measures the angle between the heading and the bearing to the
// other point against half the cone; headings are client units (16384 is a
// quarter turn).
func TestIsFacing(t *testing.T) {
	const (
		east  = 0
		north = 16384
		west  = 32768
		south = 49152
	)
	type probe struct {
		name    string
		heading int
		dx, dy  int
		degrees int
		want    bool
	}
	for _, p := range []probe{
		{"dead ahead east", east, 100, 0, 120, true},
		{"directly behind", east, -100, 0, 120, false},
		{"just inside the half cone", east, 100, 173, 120, true},   // atan2(173,100) = 59.97 deg
		{"just outside the half cone", east, 100, 174, 120, false}, // 60.1 deg
		{"inside, clockwise side", east, 100, -173, 120, true},
		{"outside, clockwise side", east, 100, -174, 120, false},
		{"north heading, ahead", north, 0, 100, 90, true},
		{"north heading, 44 deg off", north, 96, 100, 90, true}, // 43.8 deg off north
		{"north heading, 46 deg off", north, 104, 100, 90, false},
		{"west heading, ahead", west, -100, 0, 60, true},
		{"south heading, ahead", south, 0, -100, 60, true},
		// Bearing 359.4 deg against heading 0: the 0/360 seam must wrap.
		{"seam, bearing just below 360", east, 1000, -10, 2, true},
		{"seam, narrower cone excludes it", east, 1000, -10, 1, false},
		// Bearing 0.57 deg against heading 65535 (359.99 deg): same seam, other side.
		{"seam, heading just below 360", 65535, 1000, 10, 2, true},
		{"seam, heading just below 360 narrow", 65535, 1000, 10, 1, false},
		{"full circle sees behind", east, -100, 0, 360, true},
		{"wider than full circle sees behind", east, -100, 0, 720, true},
		{"zero cone sees nothing", east, 100, 0, 0, false},
		{"negative cone sees nothing", east, 100, 0, -10, false},
		{"180 cone includes the side", east, 0, 100, 180, true},
		{"180 cone excludes behind", east, -100, 1, 180, false},
		{"359 cone still excludes the exact back", east, -100, 0, 359, false},
	} {
		self := OrientedLocation{Location: Location{X: 5000, Y: -7000, Z: 40}, Heading: p.heading}
		other := Location{X: self.X + p.dx, Y: self.Y + p.dy, Z: 9000}
		if got := self.IsFacing(other, p.degrees); got != p.want {
			t.Errorf("%s: IsFacing(dx=%d dy=%d, %d deg) at heading %d = %v, want %v",
				p.name, p.dx, p.dy, p.degrees, p.heading, got, p.want)
		}
	}
}
