package bad

import (
	"github.com/fatal10110/acis_golang/internal/gameserver/script"
	"sync"
)

var counter int

var _ = sync.Mutex{}

// New breaks the rule in every way the check knows.
func New(limit int) script.Script {
	count := 0
	seen := map[int32]bool{}
	ptr := new(int)
	other := 0
	delete(seen, 1)
	ptr = &other
	return script.Script{Hooks: script.Hooks{
		OnTalk: func(_ *script.Script, e script.Talk) string {
			count++
			if count > limit {
				return ""
			}
			seen[e.NPC.ObjectID()] = true
			counter++
			_ = other
			go func() {}()
			return ""
		},
		OnFirstTalk: func(_ *script.Script, e script.FirstTalk) string {
			n := 0
			_ = *ptr
			return string(rune(n))
		},
	}}
}
