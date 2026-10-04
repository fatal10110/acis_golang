package olympiad

import (
	"context"
	"slices"
)

// NoblesseGatePass is the item the Olympiad managers pay points out in.
const NoblesseGatePass int32 = 6651

// Applicant is what registration reads of the player asking to compete.
type Applicant struct {
	ObjectID int32
	Name     string
	// BaseClass is the class the player's classed registration and record
	// are kept under.
	BaseClass int
	Noble     bool
	// SubclassActive is set while the player plays one of its subclasses.
	SubclassActive bool
	// CursedWeaponID is the item id of the cursed weapon the player holds,
	// 0 for none.
	CursedWeaponID int32
	// Overweight is set when the player's load is in the third weight band
	// or above, or its inventory is at least 80% full.
	Overweight bool
}

// RegisterResult is the answer to a request to compete.
type RegisterResult int

const (
	// Registered: the player joined the waiting list it asked for.
	Registered RegisterResult = iota
	// RegisterNotInProgress: no competition window is open.
	RegisterNotInProgress
	// RegisterClosing: the window closes in less than ten minutes.
	RegisterClosing
	// RegisterNotNoble: only nobles compete.
	RegisterNotNoble
	// RegisterSubclass: the player plays a subclass.
	RegisterSubclass
	// RegisterCursedWeapon: the player holds the cursed weapon
	// Applicant.CursedWeaponID.
	RegisterCursedWeapon
	// RegisterOverweight: the player carries too much.
	RegisterOverweight
	// RegisterAlreadyNonClassed: the player already waits for a
	// non-classed match.
	RegisterAlreadyNonClassed
	// RegisterAlreadyClassed: the player already waits for a classed match.
	RegisterAlreadyClassed
	// RegisterInMatch: the player competes in a running match.
	RegisterInMatch
	// RegisterNoPoints: the player's record holds no points.
	RegisterNoPoints
)

// UnregisterResult is the answer to a request to leave the waiting list.
type UnregisterResult int

const (
	// Unregistered: the player left the waiting list.
	Unregistered UnregisterResult = iota
	// UnregisterNotInProgress: no competition window is open.
	UnregisterNotInProgress
	// UnregisterNotNoble: only nobles compete.
	UnregisterNotNoble
	// UnregisterNotRegistered: the player waits for no match.
	UnregisterNotRegistered
	// UnregisterInMatch: the player competes in a running match; nothing
	// is said.
	UnregisterInMatch
)

// openWindowMillis is how long a competition window must still run for a
// registration to be taken.
const openWindowMillis = 600000

// Register puts a onto the waiting list of kind: the one of its base class
// for Classed, the shared one for NonClassed. The window must be open and
// run for at least ten more minutes; then a must be a noble on its base
// class, holding no cursed weapon, not overweight, on no waiting list and
// in no match. A noble without a record for the running cycle is given one
// with the starting points, which it keeps even when it is then refused for
// having no points.
func (o *Olympiad) Register(a Applicant, kind GameType) RegisterResult {
	now := o.queue.Now().UnixMilli()
	o.mu.Lock()
	defer o.mu.Unlock()
	switch {
	case o.period != Competition:
		return RegisterNotInProgress
	case o.periodEnd-now < openWindowMillis:
		return RegisterClosing
	case !a.Noble:
		return RegisterNotNoble
	case a.SubclassActive:
		return RegisterSubclass
	case a.CursedWeaponID != 0:
		return RegisterCursedWeapon
	case a.Overweight:
		return RegisterOverweight
	}
	if r, ok := o.registeredLocked(a.ObjectID, a.BaseClass); ok {
		return r
	}
	if o.inMatchLocked(a.ObjectID) {
		return RegisterInMatch
	}
	n, ok := o.nobles[a.ObjectID]
	if !ok {
		n = Noble{ClassID: a.BaseClass, Name: a.Name, Points: o.cfg.StartPoints}
		o.nobles[a.ObjectID] = n
	}
	if n.Points <= 0 {
		return RegisterNoPoints
	}
	if kind == Classed {
		o.classed[a.BaseClass] = append(o.classed[a.BaseClass], a.ObjectID)
	} else {
		o.nonClassed = append(o.nonClassed, a.ObjectID)
	}
	return Registered
}

// registeredLocked reports which waiting list objectID is on, the
// non-classed one first, as the refusal a second registration gets.
func (o *Olympiad) registeredLocked(objectID int32, baseClass int) (RegisterResult, bool) {
	if slices.Contains(o.nonClassed, objectID) {
		return RegisterAlreadyNonClassed, true
	}
	if slices.Contains(o.classed[baseClass], objectID) {
		return RegisterAlreadyClassed, true
	}
	return 0, false
}

// inMatchLocked reports whether objectID competes in a running match.
// No match runs yet: the game manager that starts them is #3340, which
// answers this from its stadiums' games during the competition window.
func (o *Olympiad) inMatchLocked(int32) bool {
	return false
}

// Unregister takes the player objectID, of base class baseClass, off the
// waiting list it is on. The window must be open and the player a noble on
// a waiting list and in no match.
func (o *Olympiad) Unregister(objectID int32, baseClass int, noble bool) UnregisterResult {
	o.mu.Lock()
	defer o.mu.Unlock()
	switch {
	case o.period != Competition:
		return UnregisterNotInProgress
	case !noble:
		return UnregisterNotNoble
	}
	if _, ok := o.registeredLocked(objectID, baseClass); !ok {
		return UnregisterNotRegistered
	}
	if o.inMatchLocked(objectID) {
		return UnregisterInMatch
	}
	o.removeLocked(objectID, baseClass)
	return Unregistered
}

// IsRegistered reports whether the player objectID, of base class
// baseClass, is on a waiting list.
func (o *Olympiad) IsRegistered(objectID int32, baseClass int) bool {
	o.mu.Lock()
	defer o.mu.Unlock()
	_, ok := o.registeredLocked(objectID, baseClass)
	return ok
}

// IsRegisteredInComp reports whether the player objectID, of base class
// baseClass, is on a waiting list or competes in a running match.
func (o *Olympiad) IsRegisteredInComp(objectID int32, baseClass int) bool {
	o.mu.Lock()
	defer o.mu.Unlock()
	if _, ok := o.registeredLocked(objectID, baseClass); ok {
		return true
	}
	return o.period == Competition && o.inMatchLocked(objectID)
}

// RemoveDisconnectedCompetitor takes the player objectID, of base class
// baseClass, off the waiting list it is on, as it leaves the world or is
// jailed. Recording the departure in its running match is the game
// manager's (#3340).
func (o *Olympiad) RemoveDisconnectedCompetitor(objectID int32, baseClass int) {
	o.mu.Lock()
	defer o.mu.Unlock()
	o.removeLocked(objectID, baseClass)
}

// removeLocked takes objectID off the non-classed waiting list, or else
// off the one of baseClass. The class keeps its (possibly empty) list.
func (o *Olympiad) removeLocked(objectID int32, baseClass int) {
	if i := slices.Index(o.nonClassed, objectID); i >= 0 {
		o.nonClassed = slices.Delete(o.nonClassed, i, i+1)
		return
	}
	if list := o.classed[baseClass]; list != nil {
		if i := slices.Index(list, objectID); i >= 0 {
			o.classed[baseClass] = slices.Delete(list, i, i+1)
		}
	}
}

// WaitingList returns the sizes the Olympiad manager shows: classed is the
// number of classes anyone registered for since the lists were last
// cleared, whether or not they still hold a registration, and nonClassed
// the registrations for non-classed matches.
func (o *Olympiad) WaitingList() (classed, nonClassed int) {
	o.mu.Lock()
	defer o.mu.Unlock()
	return len(o.classed), len(o.nonClassed)
}

// Points returns objectID's points for the running cycle, 0 without a
// record.
func (o *Olympiad) Points(objectID int32) int {
	n, _ := o.Noble(objectID)
	return n.Points
}

// Passes trades objectID's points for Noblesse Gate Passes outside the
// competition window, once a cycle: its points, at most 1000 and none
// below 50, plus HeroPoints for a hero (hero set), times GPPerPoint. The
// record is then marked rewarded with no points, whatever it paid, and the
// change is queued for the database at once, so a second claim pays
// nothing even across a restart. Without a record, inside the window or
// once rewarded it pays 0 and changes nothing.
func (o *Olympiad) Passes(objectID int32, hero bool) int {
	o.order.Lock()
	defer o.order.Unlock()
	o.mu.Lock()
	n, ok := o.nobles[objectID]
	if o.period == Competition || !ok || n.Rewarded {
		o.mu.Unlock()
		return 0
	}
	points := min(1000, n.Points)
	if points < 50 {
		points = 0
	}
	if hero {
		points += o.cfg.HeroPoints
	}
	n.Rewarded, n.Points = true, 0
	o.nobles[objectID] = n
	o.mu.Unlock()
	saved := map[int32]Noble{objectID: n}
	o.write("save noblesse pass claim", func(ctx context.Context, st Store) error { return st.SaveNobles(ctx, saved) })
	return points * o.cfg.GPPerPoint
}

// ClassLeaders returns the names of the month's best ten of classID with at
// least MinMatches matches, by points, then matches, then wins, read from
// the month's standings.
func (o *Olympiad) ClassLeaders(ctx context.Context, classID int) ([]string, error) {
	return o.store.ClassLeaders(ctx, classID, o.cfg.MinMatches)
}
