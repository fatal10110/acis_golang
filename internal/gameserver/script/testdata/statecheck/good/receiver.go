package good

import (
	"github.com/fatal10110/acis_golang/internal/gameserver/script"
)

// quest holds what its hooks only read, as q350 does.
type quest struct{ name string }

// Value binds read-only value-receiver method values.
func Value() script.Script {
	q := quest{name: "good"}
	return script.Script{Hooks: script.Hooks{OnTalk: q.onTalk}}
}

func (q quest) onTalk(_ *script.Script, _ script.Talk) string {
	name := q.name
	name += "!"
	return name
}

// Factory returns a Catalog constructor that keeps its state in cells.
func Factory() func() script.Script {
	return func() script.Script {
		var talks script.IntCell
		return script.Script{Hooks: script.Hooks{OnTalk: func(*script.Script, script.Talk) string {
			talks.Add(1)
			return ""
		}}}
	}
}
