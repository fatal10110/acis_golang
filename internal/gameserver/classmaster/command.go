package classmaster

import (
	"strings"

	"github.com/fatal10110/acis_golang/internal/commons"
)

// CommandKind is a class manager dialog command.
type CommandKind int

const (
	// CommandMenu opens the menu of occupation change Tier.
	CommandMenu CommandKind = iota
	// CommandChangeClass changes the talker to ClassID.
	CommandChangeClass
	// CommandBecomeNoble grants noblesse status.
	CommandBecomeNoble
	// CommandLearnSkills grants every skill the talker can learn.
	CommandLearnSkills
)

// Command is one class manager dialog command.
type Command struct {
	Kind    CommandKind
	Tier    int
	ClassID int
	// Malformed is a change_class command whose class, read from its
	// fourteenth character on, is missing or does not parse: its handling
	// stops outright and nothing is sent.
	Malformed bool
}

// menuCommands are the menu commands, by the tier they open.
var menuCommands = [...]string{1: "1stClass", 2: "2ndClass", 3: "3rdClass"}

// changeClassArg is where a change_class command's class starts.
const changeClassArg = len("change_class ")

// ParseCommand reads a class manager's own dialog command, matched by
// prefix. ok is false for every other command, which the manager answers
// as any civilian NPC does.
func ParseCommand(command string) (Command, bool) {
	for tier, prefix := range menuCommands {
		if prefix != "" && strings.HasPrefix(command, prefix) {
			return Command{Kind: CommandMenu, Tier: tier}, true
		}
	}
	switch {
	case strings.HasPrefix(command, "change_class"):
		cmd := Command{Kind: CommandChangeClass, Malformed: true}
		if len(command) >= changeClassArg {
			if id, err := commons.ParseInt(command[changeClassArg:], 32); err == nil {
				cmd.ClassID, cmd.Malformed = int(id), false
			}
		}
		return cmd, true
	case strings.HasPrefix(command, "become_noble"):
		return Command{Kind: CommandBecomeNoble}, true
	case strings.HasPrefix(command, "learn_skills"):
		return Command{Kind: CommandLearnSkills}, true
	}
	return Command{}, false
}
