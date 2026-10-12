package bad

import (
	"github.com/fatal10110/acis_golang/internal/gameserver/script"
)

// Alias writes the family's plain map through a hook-local copy of it.
func Alias(f *family) script.Script {
	return script.Script{Hooks: script.Hooks{OnTalk: func(_ *script.Script, e script.Talk) string {
		s := f.seen
		s[e.NPC.ObjectID()] = true
		return ""
	}}}
}

// LocalAlias writes a constructor-local map through a hook-local copy.
func LocalAlias() script.Script {
	seen := map[int32]bool{}
	return script.Script{Hooks: script.Hooks{OnTalk: func(_ *script.Script, e script.Talk) string {
		s := seen
		s[e.NPC.ObjectID()] = true
		return ""
	}}}
}

// Chain deletes from the family's map through an alias of an alias.
func Chain(f *family) script.Script {
	return script.Script{Hooks: script.Hooks{OnTalk: func(_ *script.Script, e script.Talk) string {
		var s map[int32]bool
		s = f.seen
		t := s
		delete(t, e.NPC.ObjectID())
		return ""
	}}}
}

// Deref stores through a copy of a constructor-local pointer.
func Deref() script.Script {
	total := new(int)
	return script.Script{Hooks: script.Hooks{OnTalk: func(*script.Script, script.Talk) string {
		p := total
		*p = 1
		return ""
	}}}
}

// Appended appends into a constructor-local slice's backing array through
// a copy and stores the result back.
func Appended() script.Script {
	order := make([]int32, 0, 8)
	return script.Script{Hooks: script.Hooks{OnTalk: func(_ *script.Script, e script.Talk) string {
		o := order[:0]
		o = append(o, e.NPC.ObjectID())
		return string(rune(len(o)))
	}}}
}

// rooms is a family whose plain field holds maps.
type rooms struct {
	byFloor []map[int32]bool
	byName  map[string]map[int32]bool
	last    *script.NPC
}

// Ranged writes each map of the family through the range variable.
func Ranged(r *rooms) script.Script {
	return script.Script{Hooks: script.Hooks{OnTalk: func(_ *script.Script, e script.Talk) string {
		for _, floor := range r.byFloor {
			floor[e.NPC.ObjectID()] = true
		}
		return ""
	}}}
}

// mark writes through its receiver by way of a local alias.
func (r *rooms) mark(name string, id int32) {
	if floor, ok := r.byName[name]; ok {
		floor[id] = true
	}
}

// keep stores into a parameter's pointee through a copy of its field.
func keep(r *rooms, npc *script.NPC) {
	last := r.last
	*last = *npc
}

// Wrapped writes the family's map through a local struct holding it.
func Wrapped(f *family) script.Script {
	return script.Script{Hooks: script.Hooks{OnTalk: func(_ *script.Script, e script.Talk) string {
		w := struct{ m map[int32]bool }{m: f.seen}
		w.m[e.NPC.ObjectID()] = true
		return ""
	}}}
}
