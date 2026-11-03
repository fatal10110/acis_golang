package npc

import (
	"math/rand/v2"
	"strconv"
)

// silentGuards are the guards of the starting villages and the patrols:
// an interact with one does nothing.
var silentGuards = map[int]bool{
	30733: true, 31032: true, 31033: true, 31034: true, 31035: true, 31036: true,
	31671: true, 31672: true, 31673: true, 31674: true,
}

// Talks reports whether a player's interact makes this NPC talk: a town
// guard, other than the starting villages' and patrols' guards, or a
// friendly monster. Any other hostile NPC answers an interact with nothing.
func (h *Hostile) Talks() bool {
	switch hostileKind(h.Instance) {
	case "Guard":
		return !silentGuards[h.NpcID()]
	case "FriendlyMonster":
		return true
	}
	return false
}

// ChatPage returns the datapack page of chat page val of this NPC: a
// guard's own page, <npcId>.htm for 0 and <npcId>-<val>.htm otherwise,
// whether or not it exists; else that page of the NPC's default pages,
// which exists reports on, or the generic one.
func (h *Hostile) ChatPage(val int, exists func(file string) bool) string {
	name := strconv.Itoa(h.NpcID())
	if val != 0 {
		name += "-" + strconv.Itoa(val)
	}
	if hostileKind(h.Instance) == "Guard" {
		return "data/html/guard/" + name + ".htm"
	}
	if file := "data/html/default/" + name + ".htm"; exists(file) {
		return file
	}
	return "data/html/npcdefault.htm"
}

// TalkAnimation claims the talk animation an interact plays: a random
// social action id in [0, 8), none while the NPC cannot act, and otherwise
// as the AI's social clock allows (see ai.Attackable.ClaimTalkSocial). ok
// is false when none plays.
func (h *Hostile) TalkAnimation() (id int32, ok bool) {
	if h.DenyAIAction() || !h.brain.ClaimTalkSocial() {
		return 0, false
	}
	return int32(rand.IntN(8)), true
}
