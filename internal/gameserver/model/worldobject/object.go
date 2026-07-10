package worldobject

// Object is anything that can be tracked and looked up by id within the
// world grid.
type Object interface {
	ObjectID() int32
}
