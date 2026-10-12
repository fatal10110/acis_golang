package task

import (
	"slices"

	"github.com/fatal10110/acis_golang/internal/gameserver/model/item"
)

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
// rows are guarded by ItemInstances.groupsMu. A group's membership never
// changes once Bind made it: binding one of its rows again builds a new group
// holding both, so a write that saw a group knows every row it stands for.
type rowGroup struct {
	rows map[int32]BoundRow
}

// BoundGroups names the groups a write carried, which only that write's
// landing may settle (Landed).
type BoundGroups []*rowGroup

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
// land with only some of the rows it depends on. It runs after the
// operation's mutation, which is why the operation is opened before it
// mutates (BeginOperation) and ended only after its places are taken: no
// write reads one of its rows while it is changed but not yet bound.
//
// Bind returns the group the operation's write carries, for its Landed.
func (i *ItemInstances) Bind(rows []BoundRow) BoundGroups {
	if len(rows) < 2 {
		return nil
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
	return BoundGroups{merged}
}

// Widen returns the rows bound to any of ids (Bind) that ids does not
// already name, and the groups they came from. A write of ids writes these
// too, reading each instance's state where it takes the row's place, or
// deleting a row with no instance, and passes the groups to its Landed.
func (i *ItemInstances) Widen(ids []int32) ([]BoundRow, BoundGroups) {
	i.groupsMu.Lock()
	defer i.groupsMu.Unlock()
	if len(i.groups) == 0 {
		return nil, nil
	}
	named := make(map[int32]struct{}, len(ids))
	for _, id := range ids {
		named[id] = struct{}{}
	}
	var out []BoundRow
	var carried BoundGroups
	for _, id := range ids {
		g := i.groups[id]
		if g == nil || slices.Contains(carried, g) {
			continue
		}
		carried = append(carried, g)
		for objectID, row := range g.rows {
			if _, ok := named[objectID]; !ok {
				out = append(out, row)
				named[objectID] = struct{}{}
			}
		}
	}
	return out, carried
}

// Landed reports that one write, which carried groups (Bind, Widen), has
// landed the rows in written — the rows it really wrote, not those a later
// write took over. Each carried group whose every row is in written is
// settled and stops widening writes.
//
// Only the carried groups are settled, and only rows still bound to them: a
// row bound again since (Bind merges it into a newer group) belongs to an
// operation whose state this write read too early to carry. Settling by row
// would let an older write landing late clear that newer group before its own
// write landed, and a failure of that write would then leave its rows to be
// written apart.
func (i *ItemInstances) Landed(groups BoundGroups, written []int32) {
	if len(groups) == 0 {
		return
	}
	i.groupsMu.Lock()
	defer i.groupsMu.Unlock()
	for _, g := range groups {
		if !groupCovered(g, written) {
			continue
		}
		for member := range g.rows {
			if i.groups[member] == g {
				delete(i.groups, member)
			}
		}
	}
}

// groupCovered reports whether written names every row of g.
func groupCovered(g *rowGroup, written []int32) bool {
	for member := range g.rows {
		if !slices.Contains(written, member) {
			return false
		}
	}
	return true
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

// markOrphans takes the rows the pending cap just dropped (finishOwner) and
// records each of their groups whose every row is dead — deleted, or an
// instance used up or destroyed — for the next Save to release
// (releaseOrphans).
//
// A dropped row's group otherwise stays until a write lands every one of its
// rows (Landed), and the tick no longer writes a dropped row. A row whose
// item still exists may yet be written by its owner — a handler's write, the
// container's last flush at logout — and that write must carry the dropped
// rows along (Widen), or one leg of the operation would land without the
// other. So only a group with no such row left is released.
//
// A dead row's last write can still be on its way into pending: an instance
// changes first and reports the change after (item.Persister), so a row just
// used up may not be pending yet. Releasing at once would let that write land
// alone; marking here and releasing on the next Save, which first checks that
// no row of the group is pending or in flight, leaves a whole Save for it to
// arrive and keep the group.
func (i *ItemInstances) markOrphans(dropped []int32) {
	if len(dropped) == 0 {
		return
	}
	var dead []*rowGroup
	for g, rows := range i.groupsOf(dropped) {
		if allDead(rows) {
			dead = append(dead, g)
		}
	}
	if len(dead) == 0 {
		return
	}
	i.groupsMu.Lock()
	defer i.groupsMu.Unlock()
	for _, g := range dead {
		i.orphans[g] = struct{}{}
	}
}

// releaseOrphans ends every group markOrphans recorded whose rows are all
// still dead and none of which is pending or in flight: nothing is left to
// write any of them, so the group would only hold its instances for the life
// of the process. A group that has a row written again keeps it, and is
// marked again if that write is dropped too.
//
// Only the marked groups are released, and only rows still bound to them: a
// row bound again since (Bind merges it into a newer group) belongs to an
// operation none of whose writes the cap dropped, so that newer group is left
// to its own writes.
func (i *ItemInstances) releaseOrphans() {
	i.groupsMu.Lock()
	if len(i.orphans) == 0 {
		i.groupsMu.Unlock()
		return
	}
	marked := make(map[*rowGroup][]BoundRow, len(i.orphans))
	for g := range i.orphans {
		if !i.stillBound(g) {
			continue
		}
		rows := make([]BoundRow, 0, len(g.rows))
		for _, row := range g.rows {
			rows = append(rows, row)
		}
		marked[g] = rows
	}
	clear(i.orphans)
	i.groupsMu.Unlock()

	var release []*rowGroup
	for g, rows := range marked {
		if allDead(rows) && !i.anyOutstanding(rows) {
			release = append(release, g)
		}
	}
	if len(release) == 0 {
		return
	}
	i.groupsMu.Lock()
	defer i.groupsMu.Unlock()
	for _, g := range release {
		for member := range g.rows {
			if i.groups[member] == g {
				delete(i.groups, member)
			}
		}
	}
}

// stillBound reports whether any row is still bound to g rather than to a
// newer group. The caller holds groupsMu.
func (i *ItemInstances) stillBound(g *rowGroup) bool {
	for member := range g.rows {
		if i.groups[member] == g {
			return true
		}
	}
	return false
}

// groupsOf returns a copy of the rows of each group bound to one of ids.
func (i *ItemInstances) groupsOf(ids []int32) map[*rowGroup][]BoundRow {
	i.groupsMu.Lock()
	defer i.groupsMu.Unlock()
	out := make(map[*rowGroup][]BoundRow)
	for _, id := range ids {
		g := i.groups[id]
		if g == nil {
			continue
		}
		if _, ok := out[g]; ok {
			continue
		}
		rows := make([]BoundRow, 0, len(g.rows))
		for _, row := range g.rows {
			rows = append(rows, row)
		}
		out[g] = rows
	}
	return out
}

// allDead reports whether every row is a delete or an instance whose count is
// gone, which no later change brings back.
func allDead(rows []BoundRow) bool {
	for _, row := range rows {
		if row.Inst != nil && row.Inst.Snapshot().Count > 0 {
			return false
		}
	}
	return true
}

// anyOutstanding reports whether a write of any row is pending or in flight.
func (i *ItemInstances) anyOutstanding(rows []BoundRow) bool {
	for _, row := range rows {
		if i.ContainsID(row.ObjectID) {
			return true
		}
	}
	return false
}
