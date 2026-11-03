package bad

import (
	"github.com/fatal10110/acis_golang/internal/gameserver/script"
)

// quest holds a ghost every player's hook writes.
type quest struct {
	ghost *script.NPC
	talks int
	seen  map[int32]bool
}

// Ghost binds its hook as a method value on a mutable receiver.
func Ghost() script.Script {
	q := &quest{seen: map[int32]bool{}}
	return script.Script{Hooks: script.Hooks{OnTalk: q.onTalk, OnFirstTalk: q.onFirstTalk}}
}

func (q *quest) onTalk(_ *script.Script, e script.Talk) string {
	q.talks++
	if q.ghost == nil {
		q.ghost = e.NPC
	}
	return ""
}

// A value receiver still shares the map it holds.
func (q quest) onFirstTalk(_ *script.Script, e script.FirstTalk) string {
	delete(q.seen, e.NPC.ObjectID())
	return ""
}

// remember is a helper handed the shared quest.
func remember(q *quest, id int32) {
	q.seen[id] = true
}

// Factory returns a Catalog constructor whose hooks share a counter.
func Factory() func() script.Script {
	count := 0
	return func() script.Script {
		return script.Script{Hooks: script.Hooks{OnTalk: func(*script.Script, script.Talk) string {
			count++
			return ""
		}}}
	}
}

// Pointer returns a pointer to a Script whose hook shares a counter.
func Pointer() *script.Script {
	count := 0
	return &script.Script{Hooks: script.Hooks{OnTalk: func(*script.Script, script.Talk) string {
		count++
		return ""
	}}}
}
