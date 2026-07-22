package location

import "math"

const headingToDegrees = 182.04444444444444

// OrientedLocation is a world location with a client heading.
type OrientedLocation struct {
	Location
	Heading int
}

// IsBehind reports whether other is behind l, using l's heading.
func (l OrientedLocation) IsBehind(other Location) bool {
	angleOther := angleFrom(other.X, other.Y, l.X, l.Y)
	angleHeading := headingDegrees(l.Heading)
	return angleClose(angleOther-angleHeading, 60)
}

// IsInFrontOf reports whether other is in front of l, using l's heading.
func (l OrientedLocation) IsInFrontOf(other Location) bool {
	angleOther := angleFrom(l.X, l.Y, other.X, other.Y)
	angleHeading := headingDegrees(l.Heading)
	return angleClose(angleHeading-angleOther, 60)
}

// IsFacing reports whether other is inside l's forward-facing angle.
func (l OrientedLocation) IsFacing(other Location, degrees int) bool {
	if degrees >= 360 {
		return true
	}
	if degrees <= 0 {
		return false
	}
	angleOther := angleFrom(l.X, l.Y, other.X, other.Y)
	angleHeading := headingDegrees(l.Heading)
	return angleClose(angleHeading-angleOther, float64(degrees)/2)
}

func angleFrom(x1, y1, x2, y2 int) float64 {
	angle := math.Atan2(float64(y2-y1), float64(x2-x1)) * 180 / math.Pi
	if angle < 0 {
		angle += 360
	}
	return angle
}

func headingDegrees(heading int) float64 {
	return float64(heading) / headingToDegrees
}

// HeadingDegrees converts a client heading unit (0-65535 per circle) into
// degrees.
func HeadingDegrees(heading int) float64 {
	return headingDegrees(heading)
}

func angleClose(diff, max float64) bool {
	if diff <= -360+max {
		diff += 360
	}
	if diff >= 360-max {
		diff -= 360
	}
	return math.Abs(diff) <= max
}
