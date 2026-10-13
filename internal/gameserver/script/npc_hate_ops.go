package script

import (
	"github.com/fatal10110/acis_golang/internal/gameserver/model/actor/attackable"
	"github.com/fatal10110/acis_golang/internal/gameserver/model/actor/npc"
	"github.com/fatal10110/acis_golang/internal/gameserver/model/actor/player"
)

// The desire, hate and AI-state operations the boss, raid and faction
// behaviors make on an NPC (#3748).

// desireRemover is an NPC that can drop every desire and hate it holds.
type desireRemover interface{ RemoveAllDesire() }

// RemoveAllDesire drops every desire the NPC holds, zeroes its hate of
// every creature on its attack hate and on its hate list, keeping them
// listed, and stops its movement. The NPC is not sent back to peace. A
// handle on nothing panics.
func (n *NPC) RemoveAllDesire() {
	r, ok := n.combatant().(desireRemover)
	if !ok {
		panic("script: remove every desire of a handle on nothing")
	}
	r.RemoveAllDesire()
}

// RemoveDesires drops every desire of kind the NPC holds. Only a hostile
// NPC does it; any other NPC panics.
func (n *NPC) RemoveDesires(kind Intention) {
	n.hostile("remove desires").AI().Desires().RemoveKind(kind)
}

// CleanAllHate zeroes the NPC's attack hate of every creature, keeping them
// listed with their damage, and drops its attack desires. The NPC is not
// sent back to peace and its movement goes on. Only a hostile NPC does it;
// any other NPC panics.
func (n *NPC) CleanAllHate() { n.hostile("clean its hate").AI().CleanAllHate() }

// RemoveAggro drops c from the NPC's attack hate, its damage and hate with
// it, and changes nothing else: no desire is dropped and the NPC is not
// sent back to peace. Only a hostile NPC does it; any other NPC panics, and
// so does a nil c.
func (n *NPC) RemoveAggro(c Creature) {
	h := n.hostile("remove an attacker")
	h.AI().RemoveThreat(mustCombatant(c, "remove a missing attacker"))
}

// CleanAll zeroes the hate of every creature on the list, keeping them
// listed, and drops the NPC's cast desires aimed at each of them.
func (l HateList) CleanAll() { l.h.AI().CleanAllHates() }

// StopHate drops c from the list. Its desires stay. A nil c panics.
func (l HateList) StopHate(c Creature) {
	l.h.AI().Hates().StopHate(mustCombatant(c, "stop the hate of a missing creature"))
}

// AddDefault adds the hate an NPC gives a creature it notices: 300 when the
// list is empty and the NPC stands in its territory, 100 otherwise. Nil
// adds nothing, and so does one siege guard's hate of another.
func (l HateList) AddDefault(c Creature) { l.h.AddDefaultHate(combatantOf(c)) }

// DisableCoreAI turns off the NPC's own attacks when disabled is true, and
// back on when it is false; its scripts still make it act. Only a hostile
// NPC does it; any other NPC panics.
func (n *NPC) DisableCoreAI(disabled bool) { n.hostile("disable its core AI").DisableCoreAI(disabled) }

// aiSleeper is an NPC whose AI task can leave it alone.
type aiSleeper interface{ AISleeping() bool }

// AISleeping reports whether the AI task leaves the NPC alone: it is dead,
// out of the world, or in a region no player keeps active that it sleeps
// in. A handle on nothing panics.
func (n *NPC) AISleeping() bool {
	s, ok := n.combatant().(aiSleeper)
	if !ok {
		panic("script: the AI task of a handle on nothing")
	}
	return s.AISleeping()
}

// screenBroadcaster is an NPC that can show a text on its observers'
// screens.
type screenBroadcaster interface {
	BroadcastOnScreen(durationMs int32, text string)
}

// BroadcastOnScreen shows text in the middle of the screens of the players
// around the NPC for durationMs milliseconds. A handle on nothing panics.
func (n *NPC) BroadcastOnScreen(durationMs int32, text string) {
	b, ok := n.combatant().(screenBroadcaster)
	if !ok {
		panic("script: a screen message of a handle on nothing")
	}
	b.BroadcastOnScreen(durationMs, text)
}

// NeighborHero returns a hero picked at random, through the script random
// source, among the players within radius of the NPC, body to body in 3D;
// nil when there is none, and then nothing is drawn. A handle on nothing
// panics.
func (s *Script) NeighborHero(n *NPC, radius int) *Player {
	scan, ok := n.combatant().(knownScanner)
	if !ok {
		panic("script: the neighbors of a handle on nothing")
	}
	var heroes []attackable.Combatant
	scan.ForEachKnownCombatantInRadius(radius, func(c attackable.Combatant) {
		if h, ok := c.(player.CharacterHolder); ok && h.PlayerCharacter().IsHero() {
			heroes = append(heroes, c)
		}
	})
	if len(heroes) == 0 {
		return nil
	}
	return PlayerOf(heroes[s.env.Rand(len(heroes))])
}

// mustCombatant resolves c, panicking with what when c is nil or a handle
// on nothing.
func mustCombatant(c Creature, what string) attackable.Combatant {
	t := combatantOf(c)
	if t == nil {
		panic("script: " + what)
	}
	return t
}

var (
	_ aiSleeper         = (*npc.Folk)(nil)
	_ desireRemover     = (*npc.Hostile)(nil)
	_ desireRemover     = (*npc.Folk)(nil)
	_ screenBroadcaster = (*npc.Hostile)(nil)
	_ screenBroadcaster = (*npc.Folk)(nil)
)
