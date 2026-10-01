package clan

import "context"

// The sub-unit, mentor and war writes are not inspected by the write-order
// tests; the fake store accepts them.

func (f *fakeStore) SetPledgeType(context.Context, int32, int) error      { return nil }
func (f *fakeStore) SetMentor(context.Context, int32, int32, int32) error { return nil }
func (f *fakeStore) InsertSubunit(context.Context, SubunitRow) error      { return nil }
func (f *fakeStore) UpdateSubunit(context.Context, SubunitRow) error      { return nil }
func (f *fakeStore) InsertWar(context.Context, int32, int32) error        { return nil }
func (f *fakeStore) EndWar(context.Context, int32, int32, int64) error    { return nil }
