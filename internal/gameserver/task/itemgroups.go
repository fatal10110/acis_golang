package task

import "github.com/fatal10110/acis_golang/internal/gameserver/model/item"

// BoundRow is one items row of an operation whose rows must land together.
type BoundRow struct {
	ObjectID int32
	// OwnerID is the owner the row belongs to, which names the persistence
	// lane its writes are owed on. A destroyed instance no longer carries it.
	OwnerID int32
	// Inst is the instance whose state the row carries, or nil for a row the
	// operation deleted with no instance left to read.
	Inst *item.Instance
}

// rowGroup is a set of rows no single write has landed together yet. Its
// rows are guarded by ItemInstances.groupsMu.
type rowGroup struct {
	rows map[int32]BoundRow
}

// Bind records rows as one operation's: until one write lands them all, every
// write of any of them — a handler's write, the persistence tick's chunk, a
// container's last flush — takes the others along (Widen) and lands them in
// one transaction or not at all.
//
// The operation's own write is the one that normally lands them, a moment
// later. Binding matters when it does not: a failed write leaves the rows to
// the tick, which writes per owner lane, and to whatever handler writes one of
// them next. Written apart, a crash or a failure between the two writes of a
// trade leaves the traded items in both inventories or in neither.
//
// Bind has to run before the operation's write takes its places in the rows'
// order (persist.Order), so a write that takes a later place has already seen
// the group. Rows already bound to another group merge with it: a row cannot
// land with only some of the rows it depends on.
func (i *ItemInstances) Bind(rows []BoundRow) {
	if len(rows) < 2 {
		return
	}
	// A deleted row carries no instance, but its destroyed instance is
	// usually still pending, and writing that keeps its delete whole.
	rows = append([]BoundRow(nil), rows...)
	for n := range rows {
		if rows[n].Inst == nil {
			rows[n].Inst = i.pendingInstance(rows[n].ObjectID)
		}
	}

	i.groupsMu.Lock()
	defer i.groupsMu.Unlock()
	merged := &rowGroup{rows: make(map[int32]BoundRow, len(rows))}
	for _, row := range rows {
		if g := i.groups[row.ObjectID]; g != nil {
			for id, member := range g.rows {
				merged.rows[id] = member
			}
		}
	}
	for _, row := range rows {
		if row.Inst == nil {
			row.Inst = merged.rows[row.ObjectID].Inst
		}
		merged.rows[row.ObjectID] = row
	}
	for id := range merged.rows {
		i.groups[id] = merged
	}
}

// Widen returns the rows bound to any of ids (Bind) that ids does not
// already name. A write of ids writes these too, reading each instance's
// state where it takes the row's place, or deleting a row with no instance.
func (i *ItemInstances) Widen(ids []int32) []BoundRow {
	i.groupsMu.Lock()
	defer i.groupsMu.Unlock()
	if len(i.groups) == 0 {
		return nil
	}
	named := make(map[int32]struct{}, len(ids))
	for _, id := range ids {
		named[id] = struct{}{}
	}
	var out []BoundRow
	seen := make(map[*rowGroup]struct{})
	for _, id := range ids {
		g := i.groups[id]
		if g == nil {
			continue
		}
		if _, ok := seen[g]; ok {
			continue
		}
		seen[g] = struct{}{}
		for objectID, row := range g.rows {
			if _, ok := named[objectID]; !ok {
				out = append(out, row)
				named[objectID] = struct{}{}
			}
		}
	}
	return out
}

// Landed reports that one write has landed every row in ids. A group all of
// whose rows that write carried is settled and stops widening writes; a
// group the write covered only part of — it grew after the write took its
// places — stays.
func (i *ItemInstances) Landed(ids []int32) {
	i.groupsMu.Lock()
	defer i.groupsMu.Unlock()
	if len(i.groups) == 0 {
		return
	}
	written := make(map[int32]struct{}, len(ids))
	for _, id := range ids {
		written[id] = struct{}{}
	}
	for _, id := range ids {
		g := i.groups[id]
		if g == nil {
			continue
		}
		covered := true
		for member := range g.rows {
			if _, ok := written[member]; !ok {
				covered = false
				break
			}
		}
		if !covered {
			continue
		}
		for member := range g.rows {
			if i.groups[member] == g {
				delete(i.groups, member)
			}
		}
	}
}

// retargetGroups points ownerID's bound rows at the instances a restore just
// built for them (see retarget), so a later write that widens to one of them
// reads the live copy rather than the torn-down container's.
func (i *ItemInstances) retargetGroups(ownerID int32, claimed map[int32]*item.Instance) {
	i.groupsMu.Lock()
	defer i.groupsMu.Unlock()
	for objectID, inst := range claimed {
		g := i.groups[objectID]
		if g == nil {
			continue
		}
		g.rows[objectID] = BoundRow{ObjectID: objectID, OwnerID: ownerID, Inst: inst}
	}
}

// pendingInstance returns the instance behind objectID's outstanding write,
// or nil.
func (i *ItemInstances) pendingInstance(objectID int32) *item.Instance {
	if found := i.pendingInstances([]int32{objectID}); len(found) > 0 {
		return found[0]
	}
	return nil
}
