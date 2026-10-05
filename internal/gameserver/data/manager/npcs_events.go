package manager

import (
	"slices"

	"github.com/fatal10110/acis_golang/internal/gameserver/model/spawn"
)

// DefaultSpawnEvents is the npcs.properties SpawnEvents default: the extra
// and unique monsters, the lottery ticket sellers and the Miss Queen weapon
// dealers.
func DefaultSpawnEvents() []string {
	return []string{"extra_mob", "18age", "start_weapon"}
}

// eventMakerType is the maker AI type whose spawns follow the SpawnEvents
// entry its EventName memo names.
const eventMakerType = "event_maker"

// spawnEvents is the SpawnEvents list in configured order, each name once
// and no empty name: an empty name never matches a maker's event, and a
// repeated one would spawn its makers twice.
type spawnEvents []string

func newSpawnEvents(names []string) spawnEvents {
	var out spawnEvents
	for _, name := range names {
		if name != "" && !slices.Contains(out, name) {
			out = append(out, name)
		}
	}
	return out
}

func (e spawnEvents) has(name string) bool {
	return name != "" && slices.Contains(e, name)
}

// sevenSignsEvent reports whether event is one of the Seven Signs groups.
// The Seven Signs period and seal owners decide those, never SpawnEvents,
// so they never spawn through the SpawnEvents list.
func sevenSignsEvent(event string) bool {
	switch event {
	case "ssq_event",
		"ssq_seal1_none", "ssq_seal1_dawn", "ssq_seal1_twilight",
		"ssq_seal2_none", "ssq_seal2_dawn", "ssq_seal2_twilight":
		return true
	}
	return false
}

// allows reports whether maker's spawn condition lets its NPCs spawn and
// respawn: a maker with an event attribute needs that event listed, an
// event_maker needs its EventName memo listed. Any other maker, and a slot
// no maker declared, is allowed.
func (e spawnEvents) allows(maker *spawn.Maker) bool {
	switch {
	case maker == nil:
		return true
	case maker.Event != "":
		// #171: replace the flat Seven Signs refusal with the period and seal
		// owner check before any ssq_* maker spawns, or its NPCs never respawn.
		return !sevenSignsEvent(maker.Event) && e.has(maker.Event)
	case maker.AIType == eventMakerType:
		return e.has(maker.AIParams["EventName"])
	}
	return true
}

// spawnEventMakers spawns every maker of the listed events, event by event
// in configured order, after the on-start makers. Its slots are keyed for
// RespawnAll run gen.
func (n *Npcs) spawnEventMakers(makers []*spawn.Maker, gen int) {
	for _, event := range n.events {
		if sevenSignsEvent(event) {
			continue
		}
		before := n.LiveCount()
		for _, maker := range makers {
			if maker.Event == event {
				n.spawnMaker(maker, gen)
			}
		}
		n.log.Info().Str("event", event).Int("npcs", n.LiveCount()-before).Msg("spawned event npcs")
	}
}
