package clan

// Sieges reads the castle sieges' registrations the clan rules check. A
// nil Sieges has no clan registered anywhere.
//
// It is never called with a clan's lock held: the sieges read the clans
// under their own lock (Unopposed's join excepted, see there).
type Sieges interface {
	// Registration reports whether clanID is registered on a castle's
	// siege and whether the first such siege, in castle order, is in
	// progress.
	Registration(clanID int32) (registered, inProgress bool)
	// Fighting reports whether clanID is registered on a siege in
	// progress.
	Fighting(clanID int32) bool
	// Opposed reports whether clans a and b stand on opposite sides of a
	// castle's siege.
	Opposed(a, b int32) bool
	// Unopposed runs join unless clans a and b stand on opposite sides of
	// a castle's siege, no registration changing until join returns, and
	// reports whether it ran. join is the one call made with the sieges'
	// lock held: it may take a clan's lock but must not call back into
	// the sieges.
	Unopposed(a, b int32, join func()) bool
}

func siegeRegistration(sv Sieges, clanID int32) (registered, inProgress bool) {
	if sv == nil {
		return false, false
	}
	return sv.Registration(clanID)
}

func siegeFighting(sv Sieges, clanID int32) bool {
	return sv != nil && sv.Fighting(clanID)
}

func siegeOpposed(sv Sieges, a, b int32) bool {
	return sv != nil && sv.Opposed(a, b)
}

func siegeUnopposed(sv Sieges, a, b int32, join func()) bool {
	if sv == nil {
		join()
		return true
	}
	return sv.Unopposed(a, b, join)
}
