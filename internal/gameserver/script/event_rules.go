package script

import "sync/atomic"

// EventRules are the events.properties settings the event quests read.
type EventRules struct {
	// FourGobletsParty is the party size, 2 to 9, a Four Sepulchers entry
	// needs.
	FourGobletsParty int
	// Rift are the Dimensional Rift settings.
	Rift RiftRules
}

// RiftRules are the Dimensional Rift settings, taken as written.
type RiftRules struct {
	// MinParty is the party size an entry needs and a party in the rift
	// must keep.
	MinParty int
	// Costs are the dimension fragments each member pays to enter, by
	// rank: recruit, soldier, officer, captain, commander and hero.
	Costs [6]int
	// JumpDelayMin is the base of the time between two automatic jumps, in
	// minutes; JumpDelayRnd bounds the random part added to it, counted in
	// 30 second steps.
	JumpDelayMin, JumpDelayRnd int
	// AnakazelPortChance is the percent chance of a teleport to Anakazel.
	AnakazelPortChance int
}

// DefaultEventRules returns the shipped event settings, the ones a missing
// key reads.
func DefaultEventRules() EventRules {
	return EventRules{
		FourGobletsParty: 4,
		Rift: RiftRules{
			MinParty:           2,
			Costs:              [6]int{21, 24, 27, 30, 33, 36},
			JumpDelayMin:       8,
			JumpDelayRnd:       5,
			AnakazelPortChance: 15,
		},
	}
}

// EventBook holds the EventRules in force. //reload config puts new ones
// in force whole, so a reader that takes them once per action sees one set
// from before or after, never a mix.
type EventBook struct {
	rules atomic.Pointer[EventRules]
}

// NewEventBook returns an EventBook holding r.
func NewEventBook(r EventRules) *EventBook {
	b := &EventBook{}
	b.Set(r)
	return b
}

// Set puts r in force in place of the rules before.
func (b *EventBook) Set(r EventRules) { b.rules.Store(&r) }

// Rules returns the rules in force; a nil book reads the shipped ones.
func (b *EventBook) Rules() EventRules {
	if b == nil {
		return DefaultEventRules()
	}
	return *b.rules.Load()
}

// EventRules returns the event settings in force, read at each call, so a
// //reload config between two calls is seen by the second. A script takes
// them once per action to read one set throughout.
func (s *Script) EventRules() EventRules { return s.env.Events.Rules() }
