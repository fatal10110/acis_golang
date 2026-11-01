package scenario

import (
	"fmt"
	"regexp"
	"slices"
	"strconv"
	"strings"
)

// repeatWord is a bypass step's xN.
var repeatWord = regexp.MustCompile(`^x[0-9]+$`)

// bypassArgs reads the rest of a bypass line: the command, which keeps its
// own spacing, and how many times to send it (0 without a trailing xN, which
// is no part of the command).
func bypassArgs(rest string) (args []string, repeat int, err error) {
	if i := strings.LastIndexAny(rest, " \t"); i > 0 && repeatWord.MatchString(rest[i+1:]) {
		n, err := killCount(rest[i+1:])
		if err != nil {
			return nil, 0, fmt.Errorf("bypass count %q is not xN of 1 or more", rest[i+1:])
		}
		return []string{strings.TrimSpace(rest[:i])}, n, nil
	}
	return []string{rest}, 0, nil
}

// checkSeedCheck validates the open, dungeon and standing checks' arguments.
func checkSeedCheck(kind string, args []string) error {
	switch kind {
	case "open":
		if len(args) != 2 || (args[1] != "yes" && args[1] != "no") {
			return fmt.Errorf("open <door role> <yes|no>")
		}
	case "dungeon":
		if len(args) != 1 || (args[0] != "yes" && args[0] != "no") {
			return fmt.Errorf("dungeon <yes|no>")
		}
	case "standing":
		if len(args) != 2 {
			return fmt.Errorf("standing <player|member> <n>")
		}
		if v, err := strconv.Atoi(args[1]); err != nil || v > maxStanding || v < -maxStanding {
			return fmt.Errorf("standing %q is not -%d to %d", args[1], maxStanding, maxStanding)
		}
	}
	return nil
}

// checkSeedRoles validates the roles the open and standing checks name: a
// door line's door, the character or a member line's character.
func (sc *Scenario) checkSeedRoles() error {
	for _, st := range sc.Steps {
		for _, ck := range st.Checks {
			switch ck.Kind {
			case "open":
				if _, ok := sc.door(ck.Args[0]); !ok {
					return fmt.Errorf("%d: open check: no door line names %q", ck.Line, ck.Args[0])
				}
			case "standing":
				if ck.Args[0] != "player" && !sc.isMember(ck.Args[0]) {
					return fmt.Errorf("%d: standing check: %q is not player or a member line's character", ck.Line, ck.Args[0])
				}
			}
		}
	}
	return nil
}

// seedChecks are the checks of a step the reference probe reads, as the trace
// writes them: a door's open state, the dungeon flag, the character's
// standing and attack stance.
func seedChecks(st Step) []string {
	var out []string
	for _, ck := range st.Checks {
		switch {
		case ck.Kind == "open", ck.Kind == "dungeon", (ck.Kind == "standing" || ck.Kind == "stance") && ck.Args[0] == "player":
			out = append(out, ck.Kind+" "+strings.Join(ck.Args, " "))
		}
	}
	return out
}

// tracedSeeds reports what of the world seeds a traced scenario uses, "" for
// none: the reference probe plays none of them.
func (sc *Scenario) tracedSeeds() string {
	switch {
	case slices.ContainsFunc(sc.Steps, func(st Step) bool {
		return st.Verb == "appear" || st.Repeat > 1 || slices.ContainsFunc(st.Checks, func(ck Check) bool {
			return ck.Kind == "standing" && ck.Args[0] != "player"
		})
	}):
		return "appear step, bypass count or a member's standing check"
	}
	return ""
}
