package bad

import (
	"github.com/fatal10110/acis_golang/internal/gameserver/script"
)

// family keeps shared state outside the engine cells.
type family struct {
	count int
	seen  map[int32]bool
}

func (f *family) bump() { f.count++ }

// Member writes the family's plain fields from its constructor and its
// hooks.
func Member(f *family) script.Script {
	f.count = 1
	return script.Script{Hooks: script.Hooks{OnTalk: func(_ *script.Script, e script.Talk) string {
		f.seen[e.NPC.ObjectID()] = true
		return ""
	}}}
}
