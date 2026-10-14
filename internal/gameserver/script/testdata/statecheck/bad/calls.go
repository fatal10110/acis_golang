package bad

import (
	"maps"
	"slices"
	"sort"

	"github.com/fatal10110/acis_golang/internal/gameserver/script"
)

// ordered is a family whose plain fields a call can write.
type ordered struct {
	order []int32
	seen  map[int32]bool
	last  *int32
}

// seenMap hands out the family's plain map; it writes nothing itself.
func (o *ordered) seenMap() map[int32]bool { return o.seen }

// Sorted sorts the family's slice in place with a library mutator.
func Sorted(o *ordered) script.Script {
	return script.Script{Hooks: script.Hooks{OnTalk: func(*script.Script, script.Talk) string {
		slices.Sort(o.order)
		return ""
	}}}
}

// Copied copies into the family's map with a library mutator.
func Copied(o *ordered) script.Script {
	return script.Script{Hooks: script.Hooks{OnTalk: func(_ *script.Script, e script.Talk) string {
		other := map[int32]bool{e.NPC.ObjectID(): true}
		maps.Copy(o.seen, other)
		return ""
	}}}
}

// Resorted sorts a slice of the family's field with another package's
// mutator.
func Resorted(o *ordered) script.Script {
	return script.Script{Hooks: script.Hooks{OnTalk: func(*script.Script, script.Talk) string {
		sort.Slice(o.order[1:], func(i, j int) bool { return i < j })
		return ""
	}}}
}

// MethodResult writes the family's map through a method's result.
func MethodResult(o *ordered) script.Script {
	return script.Script{Hooks: script.Hooks{OnTalk: func(_ *script.Script, e script.Talk) string {
		s := o.seenMap()
		s[e.NPC.ObjectID()] = true
		return ""
	}}}
}

// DirectResult writes the family's map through a method's result in place.
func DirectResult(o *ordered) script.Script {
	return script.Script{Hooks: script.Hooks{OnTalk: func(_ *script.Script, e script.Talk) string {
		o.seenMap()[e.NPC.ObjectID()] = true
		return ""
	}}}
}

// Callback hands the family's map to a function value that writes it.
func Callback(o *ordered) script.Script {
	return script.Script{Hooks: script.Hooks{OnTalk: func(_ *script.Script, e script.Talk) string {
		put := func(m map[int32]bool) { m[e.NPC.ObjectID()] = true }
		put(o.seen)
		return ""
	}}}
}

// Engine hands the family's pointer to an engine call, which may keep or
// write it.
func Engine(o *ordered) script.Script {
	return script.Script{Hooks: script.Hooks{OnTalk: func(s *script.Script, _ script.Talk) string {
		s.Remember(o.last)
		return ""
	}}}
}

// Nested writes the family's map through a local map it was stored in.
func Nested(o *ordered) script.Script {
	return script.Script{Hooks: script.Hooks{OnTalk: func(_ *script.Script, e script.Talk) string {
		m := map[int]map[int32]bool{}
		m[0] = o.seen
		m[0][e.NPC.ObjectID()] = true
		return ""
	}}}
}

// sortAll is a helper whose library mutator writes through its parameter.
func sortAll(o *ordered) {
	slices.Reverse(o.order)
}

// stash writes through its parameter's map by way of a local pointer the
// map was stored behind.
func stash(o *ordered) {
	h := &holder{}
	h.m = o.seen
	h.m[1] = true
}

// Unpacked writes the family's map through a local read back out of the
// local map it was stored in.
func Unpacked(o *ordered) script.Script {
	return script.Script{Hooks: script.Hooks{OnTalk: func(_ *script.Script, e script.Talk) string {
		m := map[int]map[int32]bool{}
		m[0] = o.seen
		x := m[0]
		x[e.NPC.ObjectID()] = true
		return ""
	}}}
}
