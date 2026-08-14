package skill

// fakeActor supplies the Actor surface every cast participant carries, so a
// test double only has to spell out the capability the case under test
// actually exercises. A double that models death or identity itself declares
// its own Dead or ObjectID, which shadows the one embedded here.
type fakeActor struct {
	objectID int32
}

func (f fakeActor) ObjectID() int32 { return f.objectID }

func (fakeActor) Dead() bool { return false }
