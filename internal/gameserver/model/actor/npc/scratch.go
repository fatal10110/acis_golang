package npc

import "sync"

// IntSlot names one integer of an NPC's script memory.
type IntSlot uint8

// The integer slots. AVQuest0 and AVQuest1 are the two that scripts also
// update with CompareAndExchangeInt.
const (
	IntAI0 IntSlot = iota
	IntAI1
	IntAI2
	IntAI3
	IntAI4
	IntQuest0
	IntQuest1
	IntQuest2
	IntQuest3
	IntQuest4
	IntParam1
	IntParam2
	IntParam3
	IntFlag
	IntRespawnTime
	IntWeightPoint
	IntAVQuest0
	IntAVQuest1
	intSlotCount
)

// CreatureSlot names one creature reference of an NPC's script memory.
type CreatureSlot uint8

// The creature slots.
const (
	CreatureAI0 CreatureSlot = iota
	CreatureAI1
	CreatureAI2
	CreatureAI3
	CreatureAI4
	CreatureQuest0
	CreatureQuest1
	CreatureQuest2
	CreatureQuest3
	CreatureQuest4
	creatureSlotCount
)

// ScratchCreature is a creature a script keeps in a creature slot. The
// slot holds the value it was given, whatever happens to the creature
// since.
type ScratchCreature interface {
	ObjectID() int32
}

// Scratch is the memory scripts keep on an NPC: integer and creature slots
// and the script value. A spawn slot owns one and hands it to every NPC it
// spawns, so the slots carry over from one life to the next; Respawned
// clears the script value for each new life. A Scratch is safe for
// concurrent use; each call is atomic on its own.
type Scratch struct {
	mu          sync.Mutex
	ints        [intSlotCount]int32
	creatures   [creatureSlotCount]ScratchCreature
	scriptValue int32
}

// NewScratch returns the memory of an NPC that has never lived: every slot
// zero or empty, except the weight point, which starts at 1.
func NewScratch() *Scratch {
	s := &Scratch{}
	s.ints[IntWeightPoint] = 1
	return s
}

// Int returns the value of slot.
func (s *Scratch) Int(slot IntSlot) int32 {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.ints[slot]
}

// SetInt stores v in slot.
func (s *Scratch) SetInt(slot IntSlot, v int32) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.ints[slot] = v
}

// CompareAndExchangeInt stores v in slot when it holds expected, and
// returns the value slot held before the call either way.
func (s *Scratch) CompareAndExchangeInt(slot IntSlot, expected, v int32) int32 {
	s.mu.Lock()
	defer s.mu.Unlock()
	old := s.ints[slot]
	if old == expected {
		s.ints[slot] = v
	}
	return old
}

// Creature returns the creature in slot, nil when it holds none.
func (s *Scratch) Creature(slot CreatureSlot) ScratchCreature {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.creatures[slot]
}

// SetCreature stores c in slot; nil empties it.
func (s *Scratch) SetCreature(slot CreatureSlot, c ScratchCreature) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.creatures[slot] = c
}

// ScriptValue returns the script value.
func (s *Scratch) ScriptValue() int32 {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.scriptValue
}

// SetScriptValue stores v as the script value.
func (s *Scratch) SetScriptValue(v int32) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.scriptValue = v
}

// Respawned starts a new life of the slot: the script value goes back to
// zero and every other slot keeps its value.
func (s *Scratch) Respawned() {
	s.SetScriptValue(0)
}
