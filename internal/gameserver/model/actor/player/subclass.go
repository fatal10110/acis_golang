package player

import (
	"sort"
	"sync/atomic"
)

// MaxSubclasses is how many subclasses one character may hold.
const MaxSubclasses = 3

// SubclassStartLevel is the level a new subclass starts at and the lowest
// level a subclass ever drops to.
const SubclassStartLevel = 40

// SubClass is one subclass slot: the class it plays, its slot index (1 to
// MaxSubclasses) and its own progression.
type SubClass struct {
	ClassID int
	Index   int
	Exp     int64
	SP      int
	Level   int
}

// NewSubClass returns a fresh subclass of classID in slot index, at
// SubclassStartLevel with that level's starting experience and no SP.
func NewSubClass(classID, index int, levels *LevelTable) SubClass {
	var exp int64
	if levels != nil {
		exp = levels.RequiredExpForLevel(SubclassStartLevel)
	}
	return SubClass{ClassID: classID, Index: index, Exp: exp, Level: SubclassStartLevel}
}

// subclassState is a character's subclass slots. slots, base and the active
// index are guarded by progressionMu, as the progression they trade places
// with is.
type subclassState struct {
	slots map[int]SubClass
	// base holds the base class's level, experience and SP while a
	// subclass is active; the live progression fields then hold the
	// subclass's own.
	baseLevel int
	baseExp   int64
	baseSP    int
	// index is the active class index: 0 for the base class, a slot index
	// while a subclass is active. Atomic for the readers outside
	// progressionMu (persistence slots, packets).
	index atomic.Int32
	// locked is held while a class change is in progress, from its request
	// until the new class is in place.
	locked atomic.Bool
}

// ClassID returns the class c currently plays.
func (c *Character) ClassID() int { return int(c.activeClassID.Load()) }

// SetClassID records the class c plays. Only restore and character
// creation call it; a live class change goes through SwitchClass.
func (c *Character) SetClassID(id int) { c.activeClassID.Store(int32(id)) }

// ClassIndex returns the active class index: 0 for the base class, the
// active subclass's slot otherwise.
func (c *Character) ClassIndex() int { return int(c.subclasses.index.Load()) }

// SubclassActive reports whether c plays one of its subclasses.
func (c *Character) SubclassActive() bool { return c.ClassIndex() > 0 }

// TryLockClassChange claims c's class-change lock, reporting false when a
// change is already in progress.
func (c *Character) TryLockClassChange() bool { return c.subclasses.locked.CompareAndSwap(false, true) }

// UnlockClassChange releases the class-change lock.
func (c *Character) UnlockClassChange() { c.subclasses.locked.Store(false) }

// ClassChangeLocked reports whether a class change is in progress.
func (c *Character) ClassChangeLocked() bool { return c.subclasses.locked.Load() }

// Subclasses returns c's subclass slots by ascending index. The active
// subclass carries its live progression.
func (c *Character) Subclasses() []SubClass {
	c.progressionMu.RLock()
	defer c.progressionMu.RUnlock()
	return c.subclassesLocked()
}

func (c *Character) subclassesLocked() []SubClass {
	out := make([]SubClass, 0, len(c.subclasses.slots))
	active := c.ClassIndex()
	for _, sub := range c.subclasses.slots {
		if sub.Index == active {
			sub.Exp, sub.SP, sub.Level = c.Exp, c.SP, c.CharLevel
		}
		out = append(out, sub)
	}
	sort.Slice(out, func(i, j int) bool { return out[i].Index < out[j].Index })
	return out
}

// Subclass returns the subclass in slot index.
func (c *Character) Subclass(index int) (SubClass, bool) {
	for _, sub := range c.Subclasses() {
		if sub.Index == index {
			return sub, true
		}
	}
	return SubClass{}, false
}

// RestoreSubclasses loads c's stored subclass slots, before c is live. The
// progression fields hold the characters row, which is the base class's;
// when the stored active class is one of the subclasses, that subclass
// becomes active and its progression takes the fields' place. It reports
// whether the stored active class is the base class or one of the
// subclasses: false means the row names a class c does not hold.
func (c *Character) RestoreSubclasses(subs []SubClass) bool {
	c.progressionMu.Lock()
	defer c.progressionMu.Unlock()
	c.subclasses.slots = make(map[int]SubClass, len(subs))
	for _, sub := range subs {
		c.subclasses.slots[sub.Index] = sub
	}
	active := c.ClassID()
	if active == c.BaseClassID {
		return true
	}
	for _, sub := range subs {
		if sub.ClassID != active {
			continue
		}
		c.subclasses.baseLevel, c.subclasses.baseExp, c.subclasses.baseSP = c.CharLevel, c.Exp, c.SP
		c.CharLevel, c.Exp, c.SP = sub.Level, sub.Exp, sub.SP
		c.subclasses.index.Store(int32(sub.Index))
		return true
	}
	return false
}

// AddSubclass puts sub in its slot. It reports false when c already holds
// MaxSubclasses subclasses, the slot is 0 or the slot is taken.
func (c *Character) AddSubclass(sub SubClass) bool {
	c.progressionMu.Lock()
	defer c.progressionMu.Unlock()
	if len(c.subclasses.slots) >= MaxSubclasses || sub.Index == 0 {
		return false
	}
	if _, taken := c.subclasses.slots[sub.Index]; taken {
		return false
	}
	if c.subclasses.slots == nil {
		c.subclasses.slots = make(map[int]SubClass)
	}
	c.subclasses.slots[sub.Index] = sub
	return true
}

// RemoveSubclass empties slot index. Removing the active subclass leaves
// the live progression in place until the next SwitchClass.
func (c *Character) RemoveSubclass(index int) {
	c.progressionMu.Lock()
	defer c.progressionMu.Unlock()
	delete(c.subclasses.slots, index)
}

// ReplaceActiveProgression sets the live level, experience and SP to sub's.
// A subclass replaced in its own active slot is played from its fresh
// values from then on.
func (c *Character) ReplaceActiveProgression(sub SubClass) {
	c.progressionMu.Lock()
	defer c.progressionMu.Unlock()
	c.CharLevel, c.Exp, c.SP = sub.Level, sub.Exp, sub.SP
}

// SwitchClass makes slot index the active class (0 for the base class),
// played with tmpl. The live progression goes back to the slot it belongs
// to, unless that slot was emptied, and the new class's takes its place.
// The experience held before a death is dropped. It reports false when
// index names no subclass.
func (c *Character) SwitchClass(index int, tmpl *Template) bool {
	c.progressionMu.Lock()
	defer c.progressionMu.Unlock()
	var next SubClass
	if index != 0 {
		sub, ok := c.subclasses.slots[index]
		if !ok {
			return false
		}
		next = sub
	}
	current := c.ClassIndex()
	switch {
	case current == 0:
		c.subclasses.baseLevel, c.subclasses.baseExp, c.subclasses.baseSP = c.CharLevel, c.Exp, c.SP
	default:
		if sub, ok := c.subclasses.slots[current]; ok {
			sub.Level, sub.Exp, sub.SP = c.CharLevel, c.Exp, c.SP
			c.subclasses.slots[current] = sub
		}
	}
	if index == 0 {
		c.CharLevel, c.Exp, c.SP = c.subclasses.baseLevel, c.subclasses.baseExp, c.subclasses.baseSP
		c.SetClassID(c.BaseClassID)
	} else {
		c.CharLevel, c.Exp, c.SP = next.Level, next.Exp, next.SP
		c.SetClassID(next.ClassID)
	}
	c.ExpBeforeDeath = 0
	c.subclasses.index.Store(int32(index))
	c.runtimeTemplate.Store(tmpl)
	return true
}

// baseProgressionLocked returns the base class's level, experience and SP.
// The caller holds progressionMu.
func (c *Character) baseProgressionLocked() (level int, exp int64, sp int) {
	if c.ClassIndex() == 0 {
		return c.CharLevel, c.Exp, c.SP
	}
	return c.subclasses.baseLevel, c.subclasses.baseExp, c.subclasses.baseSP
}

// VisibleBaseClassID is the class the character model and the status
// window's base class show: the class played on the base class, the base
// class while a subclass is active.
func (c *Character) VisibleBaseClassID() int {
	if c.ClassIndex() == 0 {
		return c.ClassID()
	}
	return c.BaseClassID
}

// StartingSubclassSkills returns the skills a new subclass of t's class
// starts with: every grant learnable at SubclassStartLevel, each at its
// highest such level.
func (t *Template) StartingSubclassSkills() SkillLevels {
	out := make(SkillLevels)
	for _, g := range t.Skills {
		if g.MinLevel <= SubclassStartLevel && g.Level > out[g.SkillID] {
			out[g.SkillID] = g.Level
		}
	}
	return out
}

// ClampResources lowers current HP, MP and CP to their maxima, as a class
// change does once the new class's maxima apply.
func (c *Character) ClampResources() {
	res := c.ResourceValues()
	c.vitalsMu.Lock()
	defer c.vitalsMu.Unlock()
	c.curHP = min(c.curHP, res.MaxHP)
	c.curMP = min(c.curMP, res.MaxMP)
	c.curCP = min(c.curCP, res.MaxCP)
}

// Overweight reports whether c carries too much to change class: a load in
// the third weight band or above, or an inventory at least 80% full.
func (c *Character) Overweight() bool {
	if c.WeightPenalty() > 2 {
		return true
	}
	inv, limit := c.Inventory(), c.InventoryLimit()
	return inv != nil && limit > 0 && float64(inv.Size())/float64(limit) >= 0.8
}
