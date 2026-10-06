package npc

import (
	"math/rand/v2"
	"strconv"
	"time"
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

// ChatPage returns the datapack page of this talking NPC's chat window: a
// guard's own page, else the NPC's default page, which exists reports on,
// or the generic one.
func (h *Hostile) ChatPage(exists func(file string) bool) string {
	id := strconv.Itoa(h.NpcID())
	if hostileKind(h.Instance) == "Guard" {
		return "data/html/guard/" + id + ".htm"
	}
	if file := "data/html/default/" + id + ".htm"; exists(file) {
		return file
	}
	return "data/html/npcdefault.htm"
}

// TalkAnimation claims the talk animation an interact at now plays: a
// random social action id in [0, 8), at most one per socialInterval, the
// first one always, and none while the NPC cannot act. ok is false when
// none plays.
func (h *Hostile) TalkAnimation(now time.Time) (id int32, ok bool) {
	if h.DenyAIAction() {
		return 0, false
	}
	ms := now.UnixMilli()
	for {
		last := h.lastSocial.Load()
		if last != 0 && ms-last <= socialInterval.Milliseconds() {
			return 0, false
		}
		if h.lastSocial.CompareAndSwap(last, ms) {
			return int32(rand.IntN(8)), true
		}
	}
}
