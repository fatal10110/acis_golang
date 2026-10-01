package clan

import "context"

// The board texts are not part of the write-order fixture; its store takes
// them and keeps nothing.

func (f *fakeStore) UpdateNotice(context.Context, int32, bool, string) error { return nil }

func (f *fakeStore) UpdateIntroduction(context.Context, int32, string) error { return nil }
