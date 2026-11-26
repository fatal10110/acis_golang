package itemcontainer

// ReservedIDs holds the object ids a change takes before it takes its
// price, so that running out of ids refuses the change instead of taking a
// payment for items it cannot create. Object ids are not reused within a
// session: an id reserved and never handed out stays spent.
type ReservedIDs struct {
	ids  []int32
	late func() (int32, error)
}

// ReserveIDs takes n object ids from next up front. It returns next's error
// when any of them cannot be taken.
func ReserveIDs(n int, next func() (int32, error)) (*ReservedIDs, error) {
	r := &ReservedIDs{ids: make([]int32, 0, max(n, 0)), late: next}
	for range n {
		id, err := r.take()
		if err != nil {
			return nil, err
		}
		r.ids = append(r.ids, id)
	}
	return r, nil
}

// take is a fresh id from the allocator.
func (r *ReservedIDs) take() (int32, error) {
	if r.late == nil {
		return 0, errNoObjectID
	}
	id, err := r.late()
	if err == nil && id == 0 {
		err = errNoObjectID
	}
	return id, err
}

// Next is the nextID of the change's AddCreated calls: the reserved ids in
// the order taken, then late ones from next for an instance the reservation
// did not count on, as when a held stack it counted on has gone since.
func (r *ReservedIDs) Next() (int32, error) {
	if len(r.ids) == 0 {
		return r.take()
	}
	id := r.ids[0]
	r.ids = r.ids[1:]
	return id, nil
}
