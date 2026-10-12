package good

import (
	"github.com/fatal10110/acis_golang/internal/gameserver/script"
)

// table holds what its hooks only read.
type table struct {
	names   map[int32]string
	base    int
	pos     [2]int
	masters []master
}

// master is a plain value a table lists.
type master struct {
	npc    int32
	arcana int32
}

// Lookup copies the table's plain fields into hook-locals and changes
// only the copies.
func Lookup(t *table) script.Script {
	return script.Script{Hooks: script.Hooks{OnTalk: func(_ *script.Script, e script.Talk) string {
		names := t.names
		n := t.base
		n++
		p := t.pos
		p[0] = n
		var out []int32
		for _, m := range t.masters {
			m.npc = e.NPC.ObjectID()
			out = append(out, m.arcana)
		}
		out[0] = 0
		name := names[e.NPC.ObjectID()]
		name += "!"
		return name + string(rune(p[0]))
	}}}
}
