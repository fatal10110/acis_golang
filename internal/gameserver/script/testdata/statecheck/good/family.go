package good

import (
	"github.com/fatal10110/acis_golang/internal/gameserver/script"
)

// family is the state a family of scripts shares, as the third-class saga
// core and its sagas do: engine cells only. The catalog builds it once and
// hands it to the constructor of every member.
type family struct {
	members script.MapCell[int32, member]
	busy    script.NPCMapCell
}

// member is what a member registers about itself: plain values, read only.
type member struct {
	name string
	item int32
}

// newFamily builds the family; it is no constructor.
func newFamily() *family { return &family{} }

// Member registers itself at construction, and its hooks read and write the family's cells.
func Member(f *family, classID int32) script.Script {
	f.members.Set(classID, member{name: "good", item: 1})
	return script.Script{Hooks: script.Hooks{OnTalk: func(_ *script.Script, e script.Talk) string {
		if f.busy.Has(e.NPC) {
			f.busy.DeleteValue(e.NPC)
			return ""
		}
		f.busy.Set(e.NPC, e.NPC)
		m, _ := f.members.Get(classID)
		return m.name
	}}}
}
