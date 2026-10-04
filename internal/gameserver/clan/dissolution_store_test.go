package clan

import "context"

// A dissolution is not part of the write-order fixture; its store takes it
// and keeps nothing.
func (f *fakeStore) DeleteClan(context.Context, int32, int32) error { return nil }
