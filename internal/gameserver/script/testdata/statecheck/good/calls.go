package good

import (
	"fmt"
	"maps"
	"slices"
	"strings"

	"github.com/fatal10110/acis_golang/internal/gameserver/script"
)

// roster holds what its hooks only read, through read-only calls.
type roster struct {
	order []int32
	names []string
	seen  map[int32]bool
}

// count reads the roster's map; it writes nothing.
func (r *roster) count() int { return len(r.seen) }

// fresh builds a new slice of the roster's numbers; it shares nothing
// with its receiver.
func (r *roster) fresh() []int32 {
	var out []int32
	for _, id := range r.order {
		out = append(out, id+1)
	}
	return out
}

// Read hands the roster's plain fields to read-only library calls, sorts
// and writes copies of them only, and passes engine values to the engine.
func Read(r *roster) script.Script {
	return script.Script{Hooks: script.Hooks{OnTalk: func(s *script.Script, e script.Talk) string {
		id := e.NPC.ObjectID()
		if !slices.Contains(r.order, id) || slices.Index(r.order, id) < 0 {
			return ""
		}
		keys := slices.Sorted(maps.Keys(r.seen))
		keys[0] = id
		c := slices.Clone(r.order)
		slices.Sort(c)
		c[0] = id
		m := maps.Clone(r.seen)
		maps.Copy(m, r.seen)
		m[id] = true
		local := []int32{id}
		slices.Sort(local)
		f := r.fresh()
		f[0] = id
		slices.Sort(r.fresh())
		s.Say(e.NPC, strings.Join(r.names, ","), fmt.Sprint(r.count()))
		return ""
	}}}
}

// tally is a hook-local record of an engine value.
type tally struct {
	ghost *script.NPC
	talks int
	marks map[int32]bool
}

// record stores an engine value past a local pointer and then writes
// other fields of it: the stored value is not reached again. The result
// of an engine function or method is an engine value of its own.
func record(e script.Talk) int {
	t := &tally{}
	t.ghost = e.NPC
	t.talks++
	t.marks = map[int32]bool{}
	t.marks[1] = true
	at := script.LocOf(e.NPC)
	at.X++
	here := e.NPC.Loc()
	here.X++
	return t.talks + at.X + here.X
}
