package henna

import "sync"

const (
	// MaxAmount is the hard array size for equipped dyes.
	MaxAmount = 3
	// FirstSlotID is the 1-based DB slot written for array index 0.
	FirstSlotID = 1
	// MaxStatValue caps the summed bonus for one base attribute.
	MaxStatValue = 5
)

// Stat indexes match Java HennaType ordinal order used by HennaInfo and
// the cached _stats array: INT, STR, CON, MEN, DEX, WIT.
type Stat int

const (
	StatINT Stat = iota
	StatSTR
	StatCON
	StatMEN
	StatDEX
	StatWIT
	statCount
)

// Equipped is one non-empty dye slot for client packets.
type Equipped struct {
	SymbolID       int
	ActiveSymbolID int // 0 when the dye is not usable by the current class
}

// Snapshot is the client-visible henna surface for HennaInfo.
type Snapshot struct {
	INT, STR, CON, MEN, DEX, WIT int
	MaxSlots                     int
	Equipped                     []Equipped
}

// Row is one character_hennas persistence row (DB slot is 1-based).
type Row struct {
	Slot     int
	SymbolID int
}

// List is the equipped-dye container for one class_index slot.
type List struct {
	mu    sync.Mutex
	slots [MaxAmount]*Henna
	stats [statCount]int
}

// MaxSlots returns how many dyes classLevel may equip: 0 / 2 / 3.
func MaxSlots(classLevel int) int {
	if classLevel < 1 {
		return 0
	}
	if classLevel == 1 {
		return 2
	}
	return MaxAmount
}

// Restore replaces slots from rows using lookup. Invalid slots and unknown
// symbols are skipped. Stats are not recalculated; call Recalculate after.
func (l *List) Restore(rows []Row, lookup func(symbolID int) (Henna, bool)) {
	l.mu.Lock()
	defer l.mu.Unlock()
	var next [MaxAmount]*Henna
	for _, row := range rows {
		if row.Slot < FirstSlotID || row.Slot > FirstSlotID+MaxAmount-1 {
			continue
		}
		h, ok := lookup(row.SymbolID)
		if !ok {
			continue
		}
		cp := h
		next[row.Slot-FirstSlotID] = &cp
	}
	l.slots = next
}

// Recalculate rebuilds cached stats for classID, skipping dyes the class
// cannot use, then capping each attribute at MaxStatValue.
func (l *List) Recalculate(classID int) {
	l.mu.Lock()
	defer l.mu.Unlock()
	l.recalculateLocked(classID)
}

func (l *List) recalculateLocked(classID int) {
	var stats [statCount]int
	for _, h := range l.slots {
		if h == nil || !h.UsableByClass(classID) {
			continue
		}
		stats[StatINT] += h.INT
		stats[StatSTR] += h.STR
		stats[StatCON] += h.CON
		stats[StatMEN] += h.MEN
		stats[StatDEX] += h.DEX
		stats[StatWIT] += h.WIT
	}
	for i := range stats {
		if stats[i] > MaxStatValue {
			stats[i] = MaxStatValue
		}
	}
	l.stats = stats
}

// Stat returns the cached bonus for s.
func (l *List) Stat(s Stat) int {
	if l == nil || s < 0 || s >= statCount {
		return 0
	}
	l.mu.Lock()
	defer l.mu.Unlock()
	return l.stats[s]
}

// Add places h in the first empty slot allowed by classLevel. On success it
// returns the 1-based DB slot and recalculates stats for classID.
func (l *List) Add(h Henna, classID, classLevel int) (dbSlot int, ok bool) {
	l.mu.Lock()
	defer l.mu.Unlock()
	max := MaxSlots(classLevel)
	idx := -1
	for i := 0; i < max; i++ {
		if l.slots[i] == nil {
			idx = i
			break
		}
	}
	if idx < 0 {
		return 0, false
	}
	cp := h
	l.slots[idx] = &cp
	l.recalculateLocked(classID)
	return idx + FirstSlotID, true
}

// Remove clears the slot holding symbolID. On success it returns the 1-based
// DB slot and recalculates stats for classID.
func (l *List) Remove(symbolID, classID int) (dbSlot int, ok bool) {
	l.mu.Lock()
	defer l.mu.Unlock()
	for i, h := range l.slots {
		if h == nil || h.SymbolID != symbolID {
			continue
		}
		l.slots[i] = nil
		l.recalculateLocked(classID)
		return i + FirstSlotID, true
	}
	return 0, false
}

// BySymbolID returns the equipped template for symbolID, if any.
func (l *List) BySymbolID(symbolID int) (Henna, bool) {
	if l == nil {
		return Henna{}, false
	}
	l.mu.Lock()
	defer l.mu.Unlock()
	for _, h := range l.slots {
		if h != nil && h.SymbolID == symbolID {
			return *h, true
		}
	}
	return Henna{}, false
}

// IsFull reports whether classLevel leaves no empty slot.
func (l *List) IsFull(classLevel int) bool {
	if l == nil {
		return MaxSlots(classLevel) <= 0
	}
	l.mu.Lock()
	defer l.mu.Unlock()
	used := 0
	for _, h := range l.slots {
		if h != nil {
			used++
		}
	}
	empty := MaxSlots(classLevel) - used
	return empty <= 0
}

// Snapshot builds the HennaInfo payload for classID.
func (l *List) Snapshot(classID, classLevel int) Snapshot {
	snap := Snapshot{MaxSlots: MaxSlots(classLevel)}
	if l == nil {
		return snap
	}
	l.mu.Lock()
	defer l.mu.Unlock()
	snap.INT = l.stats[StatINT]
	snap.STR = l.stats[StatSTR]
	snap.CON = l.stats[StatCON]
	snap.MEN = l.stats[StatMEN]
	snap.DEX = l.stats[StatDEX]
	snap.WIT = l.stats[StatWIT]
	for _, h := range l.slots {
		if h == nil {
			continue
		}
		active := 0
		if h.UsableByClass(classID) {
			active = h.SymbolID
		}
		snap.Equipped = append(snap.Equipped, Equipped{
			SymbolID:       h.SymbolID,
			ActiveSymbolID: active,
		})
	}
	return snap
}
