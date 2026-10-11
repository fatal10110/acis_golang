package location

// Jump is where an instant teleport aims: To, or, when Scattered, Toward, a
// point drawn around To on the ground plane, as far as a straight walk from
// To at To's height gets toward it.
type Jump struct {
	To        Location
	Toward    Location
	Scattered bool
}
