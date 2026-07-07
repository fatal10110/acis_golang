package datadiff

import "strconv"

// FormatFloat renders f in the canonical form a Record field uses for a
// floating-point value: the shortest fixed-point decimal that reads back
// to the exact same value, which is unambiguous for a given float64 and
// needs no rounding decision. A fixed-precision format (e.g. "%.6f") would
// instead force a rounding choice at whatever precision is picked, and two
// producers that happen to round a boundary value differently (say,
// round-half-up vs round-half-even) would report a spurious mismatch on a
// field that actually agrees. Every producer of Records for the same
// category must format floats this exact way.
func FormatFloat(f float64) string {
	if f == 0 {
		return "0" // avoids rendering negative zero as "-0"
	}
	return strconv.FormatFloat(f, 'f', -1, 64)
}
