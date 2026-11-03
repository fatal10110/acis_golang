package script

import (
	"fmt"

	"github.com/fatal10110/acis_golang/internal/gameserver/model/actor/npc"
)

// DBValue returns the script value the NPC's spawn keeps in its spawn_data
// row. It panics for an NPC whose spawn keeps no row, and for a handle on
// nothing.
func (n *NPC) DBValue() int { return n.spawnRow().DBValue() }

// SetDBValue stores v as the script value of the NPC's spawn_data row,
// saved with the row. It panics as DBValue does.
func (n *NPC) SetDBValue(v int) { n.spawnRow().SetDBValue(v) }

// DBLoaded reports whether the NPC's spawn last placed its NPC from a live
// spawn_data row, its saved HP, MP and position. It is false for an NPC
// whose spawn keeps no row; a handle on nothing panics.
func (n *NPC) DBLoaded() bool {
	row := n.rowOf()
	return row != nil && row.DBLoaded()
}

func (n *NPC) spawnRow() npc.SpawnRow {
	row := n.rowOf()
	if row == nil {
		panic(fmt.Sprintf("script: npc %d keeps no spawn_data row", n.ObjectID()))
	}
	return row
}

func (n *NPC) rowOf() npc.SpawnRow {
	switch o := n.combatant().(type) {
	case *npc.Hostile:
		return o.SpawnRow()
	case *npc.Folk:
		return o.SpawnRow()
	}
	panic("script: a handle on nothing has no spawn")
}
