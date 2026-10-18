package script

import (
	"cmp"
	"slices"

	"github.com/fatal10110/acis_golang/internal/gameserver/model/actor/attackable"
)

// This file is what the //info NPC script pages read of the registry and
// the timers: the per-event lists of a template, the class chain of its
// first script that is not a real quest, and the timers bound to one NPC.

// Classes returns the class chain //info AI prints for the script: its own class
// (its Name) first, then each named class its hooks derive from, root
// last.
func (s *Script) Classes() []string {
	return append([]string{s.Name}, s.Hooks.ancestors...)
}

// EventScripts is one NPC event and the names of the scripts on its list,
// in dispatch order.
type EventScripts struct {
	Event   string
	Scripts []string
}

// NPCScripts returns, in event order, every event of the NPC template
// npcID that has scripts on its list, with their names; none for a nil
// registry.
func (r *Registry) NPCScripts(npcID int32) []EventScripts {
	var out []EventScripts
	for ev := range npcEventCount {
		list := r.scripts(npcID, ev)
		if len(list) == 0 {
			continue
		}
		names := make([]string, len(list))
		for i, s := range list {
			names[i] = s.Name
		}
		out = append(out, EventScripts{Event: ev.String(), Scripts: names})
	}
	return out
}

// AIClasses returns the class chain (see Script.Classes) of the first
// script, in event order and then dispatch order, on the lists of the NPC
// template npcID that is not a real quest; nil when there is none.
func (r *Registry) AIClasses(npcID int32) []string {
	for ev := range npcEventCount {
		for _, s := range r.scripts(npcID, ev) {
			if s.QuestID <= 0 {
				return s.Classes()
			}
		}
	}
	return nil
}

// NPCTimer is a pending timer as the //info script page shows it: its
// name and the name of the player it is bound to, "" for none.
type NPCTimer struct {
	Name   string
	Player string
}

// ScriptTimers are the pending timers of one script bound to an NPC.
type ScriptTimers struct {
	Script string
	Timers []NPCTimer
}

// NPCTimers returns, per registered script in list order, the pending
// timers bound to the live NPC n, that is to its spawn slot, whatever
// player they are also bound to; none for a nil registry or NPC. Timers
// have no natural order, so they are sorted by name, then player name.
func (r *Registry) NPCTimers(n attackable.Combatant) []ScriptTimers {
	if r == nil {
		return nil
	}
	if n == nil {
		return nil
	}
	id := npcIdentity(&NPC{self: n})
	if id == nil {
		return nil
	}
	type pending struct {
		s    *Script
		name string
		pl   *Player
	}
	var found []pending
	ts := r.timers
	ts.mu.Lock()
	for t := range ts.byNPC[id] {
		found = append(found, pending{t.s, t.key.name, t.pl})
	}
	ts.mu.Unlock()

	// The player names are read with the timers unlocked.
	byScript := map[*Script][]NPCTimer{}
	for _, p := range found {
		player := ""
		if c := p.pl.combatant(); c != nil {
			player = c.CharacterName()
		}
		byScript[p.s] = append(byScript[p.s], NPCTimer{Name: p.name, Player: player})
	}

	var out []ScriptTimers
	for _, e := range r.entries {
		list := byScript[e.script]
		if e.script == nil || len(list) == 0 {
			continue
		}
		slices.SortFunc(list, func(a, b NPCTimer) int {
			return cmp.Or(cmp.Compare(a.Name, b.Name), cmp.Compare(a.Player, b.Player))
		})
		out = append(out, ScriptTimers{Script: e.script.Name, Timers: list})
	}
	return out
}
