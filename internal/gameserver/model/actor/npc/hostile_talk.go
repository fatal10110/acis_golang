package npc

import (
	"math/rand/v2"
	"strconv"
	"strings"

	"github.com/fatal10110/acis_golang/internal/commons"
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

// HostileCommand is a dialog command a hostile NPC answers.
type HostileCommand int

const (
	// HostileUnported is a command of a system not in place for hostile
	// NPCs.
	HostileUnported HostileCommand = iota
	// HostileTerritoryStatus shows the territory status of the NPC's
	// castle.
	HostileTerritoryStatus
	// HostileQuest opens the NPC's quest windows.
	HostileQuest
	// HostileChat opens chat page Val.
	HostileChat
	// HostileLink opens the page at Path.
	HostileLink
	// HostileRefused answers nothing of its own.
	HostileRefused
	// HostileAborted stops the handling: nothing more is sent.
	HostileAborted
)

// HostileReply is a hostile NPC's reading of one dialog command.
type HostileReply struct {
	Command HostileCommand
	// Val is the chat page HostileChat opens.
	Val int
	// Path is the datapack page HostileLink opens.
	Path string
}

// ParseHostileCommand reads command, the part of an npc_<objectId>_<command>
// link after the object id, as the generic NPC dialog does: TerritoryStatus
// in any case; Quest[ <name>]; Chat <n>, page 0 when n is missing or does
// not parse; and Link <path>, data/html/<path> trimmed, refused when it
// climbs out with "..", aborting when the command is too short to hold a
// path. Any other command is HostileUnported.
func ParseHostileCommand(command string) HostileReply {
	switch {
	case strings.EqualFold(command, "TerritoryStatus"):
		return HostileReply{Command: HostileTerritoryStatus}
	case strings.HasPrefix(command, "Quest"):
		return HostileReply{Command: HostileQuest}
	case strings.HasPrefix(command, "Chat"):
		reply := HostileReply{Command: HostileChat}
		if arg, ok := commandChars(command, 5, -1); ok {
			if n, err := commons.ParseInt(arg, 32); err == nil {
				reply.Val = int(n)
			}
		}
		return reply
	case strings.HasPrefix(command, "Link"):
		arg, ok := commandChars(command, 5, -1)
		if !ok {
			return HostileReply{Command: HostileAborted}
		}
		path := strings.TrimFunc(arg, javaSpace)
		if strings.Contains(path, "..") {
			return HostileReply{Command: HostileRefused}
		}
		return HostileReply{Command: HostileLink, Path: "data/html/" + path}
	}
	return HostileReply{Command: HostileUnported}
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
