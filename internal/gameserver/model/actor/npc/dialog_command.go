package npc

import (
	"strings"

	"github.com/fatal10110/acis_golang/internal/commons"
)

// DialogCommand is one of the generic dialog commands every NPC that talks
// answers, civilian or hostile.
type DialogCommand int

const (
	// DialogOther is any command but the generic ones: the NPC's own
	// dialog answers it, if any.
	DialogOther DialogCommand = iota
	// DialogTerritoryStatus shows the territory status of the NPC's
	// castle.
	DialogTerritoryStatus
	// DialogQuest opens the NPC's quest windows.
	DialogQuest
	// DialogChat opens chat page Val.
	DialogChat
	// DialogLink opens the page at Path.
	DialogLink
	// DialogRefused answers nothing of its own.
	DialogRefused
	// DialogAborted stops the handling: nothing more is sent.
	DialogAborted
)

// DialogReply is the reading of one generic dialog command.
type DialogReply struct {
	Command DialogCommand
	// Val is the chat page DialogChat opens.
	Val int
	// Path is the datapack page DialogLink opens.
	Path string
}

// ParseDialogCommand reads command, the part of an npc_<objectId>_<command>
// link after the object id, as the generic NPC dialog does: TerritoryStatus
// in any case; Quest[ <name>]; Chat <n>, page 0 when n is missing or does
// not parse; and Link <path>, data/html/<path> trimmed, refused when it
// climbs out with "..", aborting when the command is too short to hold a
// path. Arguments start at a character offset counted in UTF-16 units, as
// the client encodes them. Any other command is DialogOther.
func ParseDialogCommand(command string) DialogReply {
	switch {
	case strings.EqualFold(command, "TerritoryStatus"):
		return DialogReply{Command: DialogTerritoryStatus}
	case strings.HasPrefix(command, "Quest"):
		return DialogReply{Command: DialogQuest}
	case strings.HasPrefix(command, "Chat"):
		reply := DialogReply{Command: DialogChat}
		if arg, ok := commandChars(command, 5, -1); ok {
			if n, err := commons.ParseInt(arg, 32); err == nil {
				reply.Val = int(n)
			}
		}
		return reply
	case strings.HasPrefix(command, "Link"):
		arg, ok := commandChars(command, 5, -1)
		if !ok {
			return DialogReply{Command: DialogAborted}
		}
		path := strings.TrimFunc(arg, isCtrlOrSpace)
		if strings.Contains(path, "..") {
			return DialogReply{Command: DialogRefused}
		}
		return DialogReply{Command: DialogLink, Path: "data/html/" + path}
	}
	return DialogReply{Command: DialogOther}
}
