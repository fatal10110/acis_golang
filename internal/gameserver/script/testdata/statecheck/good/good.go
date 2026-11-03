package good

import (
	"github.com/fatal10110/acis_golang/internal/gameserver/script"
)

const limit = 3

// New keeps its shared state in cells, and mutates only per-invocation
// locals.
func New(npcIDs []int32) script.Script {
	ghost := &script.NPCCell{}
	var talks script.IntCell
	name := "good"
	return script.Script{Bind: script.Bindings{script.EventTalk: npcIDs}, Hooks: script.Hooks{
		OnTalk: func(_ *script.Script, e script.Talk) string {
			n := 0
			for range limit {
				func() { n++ }()
			}
			if talks.Add(1) > n || !ghost.Empty() {
				return name
			}
			return ""
		},
	}}
}

// helper is no constructor: its locals are its own.
func helper() int {
	total := 0
	add := func(v int) { total += v }
	add(1)
	return total
}
