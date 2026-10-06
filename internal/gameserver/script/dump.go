package script

import (
	"bufio"
	"fmt"
	"io"
	"slices"
	"strconv"
	"strings"
)

// Dump writes the registry in the layout of the registration manifest
// (testdata/oracle/README.md), so the two compare line by line: per listed
// script its kind, own hooks and bindings, then the folded per-(NPC, event)
// lists.
//
//	script <path>                    one block per listed script, in list order
//	  kind behavior|quest|scheduled|script
//	  hooks <method>,...             the hooks the script's own constructor sets
//	  bind <EVENT>,... <npc ids>     events with identical id sets share a line
//	  scheduled                      a scheduled task its entry schedules
//	script <path> missing|refused    not registered
//
//	# folded npc events
//	fold npc <EVENT>,... <path>,... <npc ids>
func (r *Registry) Dump(w io.Writer) error {
	bw := bufio.NewWriter(w)
	for _, e := range r.entries {
		switch e.state {
		case entryMissing:
			fmt.Fprintf(bw, "script %s missing\n", e.path)
			continue
		case entryRefused:
			fmt.Fprintf(bw, "script %s refused\n", e.path)
			continue
		}
		fmt.Fprintf(bw, "script %s\n  kind %s\n", e.path, kindOf(e.script))
		if own := hookNames(e.script.Hooks.ownSet()); len(own) > 0 {
			fmt.Fprintf(bw, "  hooks %s\n", strings.Join(own, ","))
		}
		for _, g := range byIDs(e.bound) {
			fmt.Fprintf(bw, "  bind %s %s\n", g.events, g.ids)
		}
		if e.sched != nil {
			fmt.Fprint(bw, "  scheduled\n")
		}
	}

	fmt.Fprint(bw, "\n# folded npc events\n")
	folded := map[string]Bindings{}
	for k, list := range r.npc {
		if len(list) == 0 {
			continue
		}
		paths := make([]string, len(list))
		for i, s := range list {
			paths[i] = s.path
		}
		key := strings.Join(paths, ",")
		if folded[key] == nil {
			folded[key] = Bindings{}
		}
		folded[key][k.event] = append(folded[key][k.event], k.npc)
	}
	keys := make([]string, 0, len(folded))
	for k := range folded {
		keys = append(keys, k)
	}
	slices.Sort(keys)
	for _, key := range keys {
		b := folded[key]
		for ev := range b {
			slices.Sort(b[ev])
		}
		for _, g := range byIDs(b) {
			fmt.Fprintf(bw, "fold npc %s %s %s\n", g.events, key, g.ids)
		}
	}
	return bw.Flush()
}

// kindOf names s's kind as the manifest does.
func kindOf(s *Script) string {
	switch {
	case s.Behavior:
		return "behavior"
	case s.QuestID > 0:
		return "quest"
	case s.Hooks.OnStart != nil:
		return "scheduled"
	default:
		return "script"
	}
}

// hookNames returns the manifest names of the hooks in set, sorted.
func hookNames(set hookSet) []string {
	var out []string
	for h := range hookCount {
		if set.has(h) {
			out = append(out, h.String())
		}
	}
	slices.Sort(out)
	return out
}

// idGroup is a run of events sharing one id set, written out.
type idGroup struct{ events, ids string }

// byIDs groups b's events by identical id sets, in the order of each
// group's first event, each group's events in event order. Id lists must be
// sorted.
func byIDs(b Bindings) []idGroup {
	var out []idGroup
	index := map[string]int{}
	for ev := range npcEventCount {
		ids := b[ev]
		if len(ids) == 0 {
			continue
		}
		key := ranges(ids)
		i, ok := index[key]
		if !ok {
			i = len(out)
			index[key] = i
			out = append(out, idGroup{ids: key})
		}
		if out[i].events != "" {
			out[i].events += ","
		}
		out[i].events += ev.String()
	}
	return out
}

// ranges writes sorted ids with runs of three or more consecutive ids as
// a-b.
func ranges(ids []int32) string {
	var sb strings.Builder
	for i := 0; i < len(ids); {
		j := i
		for j+1 < len(ids) && ids[j+1] == ids[j]+1 {
			j++
		}
		if sb.Len() > 0 {
			sb.WriteByte(',')
		}
		sb.WriteString(strconv.Itoa(int(ids[i])))
		switch {
		case j-i >= 2:
			sb.WriteByte('-')
			sb.WriteString(strconv.Itoa(int(ids[j])))
		case j > i:
			sb.WriteByte(',')
			sb.WriteString(strconv.Itoa(int(ids[j])))
		}
		i = j + 1
	}
	return sb.String()
}
