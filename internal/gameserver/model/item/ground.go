package item

// GroundSnapshot is one persisted items_on_ground row.
type GroundSnapshot struct {
	Instance

	X, Y, Z        int
	TimeLeftMillis int64
}
