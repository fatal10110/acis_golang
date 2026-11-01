package scenario

import (
	"fmt"
	"slices"
	"strconv"
	"strings"
)

// Hall is one hall line: where a siegable clan hall's siege stands as the
// server boots, and the next siege date it stores.
type Hall struct {
	ID     int
	Status string
	// Date is the stored next siege date in Unix milliseconds, 0 for none
	// (the server then schedules the next siege itself).
	Date int64
}

// hallStatuses are the statuses a hall line and a hallstatus step name.
var hallStatuses = []string{"registering", "waiting", "siege"}

// Mate is one mate line: a second character of the clan the clan line
// seeds, of its own account.
type Mate struct {
	Name  string
	Level int
	// Leader: the mate leads the clan, so the character is a plain member
	// (the clan line's default leader is the character).
	Leader bool
	// Relation is "sponsor" when the mate is the character's academy
	// sponsor, "apprentice" when it is its apprentice, empty for neither.
	Relation string
	// Online: the mate enters the world where the character first does,
	// right after it; otherwise it stays out until an enter step names it.
	Online bool
	// DX, DY and DZ are where it stands once in the world, an offset from
	// where the character first entered it.
	DX, DY, DZ int
	// Holds are what it holds before it enters the world.
	Holds []Hold
}

// clanLine reads the clan line: clan <level> [hall <id>].
func (sc *Scenario) clanLine(args []string) error {
	usage := fmt.Errorf("one clan line: clan <level> [hall <id>]")
	if len(args) != 1 && len(args) != 3 || sc.Clan >= 0 {
		return usage
	}
	level, err := strconv.Atoi(args[0])
	if err != nil || level < 0 || level > maxClanLevel {
		return fmt.Errorf("clan level %q is not 0 to %d", args[0], maxClanLevel)
	}
	if len(args) == 3 {
		if args[1] != "hall" {
			return usage
		}
		id, err := strconv.Atoi(args[2])
		if err != nil || id < 1 {
			return fmt.Errorf("clan hall id %q is not a number of 1 or more", args[2])
		}
		sc.ClanHall = id
	}
	sc.Clan = level
	return nil
}

// hallLine reads a hall line: hall <id> <registering|waiting|siege> [date <unix ms>].
func (sc *Scenario) hallLine(args []string) error {
	usage := fmt.Errorf("hall <id> <%s> [date <unix ms>]", strings.Join(hallStatuses, "|"))
	if len(args) != 2 && len(args) != 4 {
		return usage
	}
	id, err := strconv.Atoi(args[0])
	if err != nil || id < 1 {
		return fmt.Errorf("hall id %q is not a number of 1 or more", args[0])
	}
	if !slices.Contains(hallStatuses, args[1]) {
		return fmt.Errorf("hall status %q is not %s", args[1], strings.Join(hallStatuses, ", "))
	}
	if slices.ContainsFunc(sc.Halls, func(h Hall) bool { return h.ID == id }) {
		return fmt.Errorf("hall %d listed twice", id)
	}
	h := Hall{ID: id, Status: args[1]}
	if len(args) == 4 {
		if args[2] != "date" {
			return usage
		}
		if h.Date, err = strconv.ParseInt(args[3], 10, 64); err != nil || h.Date < 1 {
			return fmt.Errorf("hall date %q is not a Unix time in milliseconds", args[3])
		}
	}
	sc.Halls = append(sc.Halls, h)
	return nil
}

// mateLine reads a mate line:
// mate <name> <level> [leader] [sponsor|apprentice] [online] [hold <item id> <count>] [<dx> <dy> <dz>].
func (sc *Scenario) mateLine(args []string, roles map[string]bool) error {
	usage := fmt.Errorf("mate <name> <level> [leader] [sponsor|apprentice] [online] [hold <item id> <count>]... [<dx> <dy> <dz>]")
	if len(args) < 2 {
		return usage
	}
	if sc.named(args[0], roles) || args[0] == sc.Character {
		return fmt.Errorf("mate %q named twice", args[0])
	}
	level, err := strconv.Atoi(args[1])
	if err != nil || level < 1 {
		return fmt.Errorf("mate level %q is not a number of 1 or more", args[1])
	}
	m := Mate{Name: args[0], Level: level}
	rest := args[2:]
flags:
	for len(rest) > 0 {
		switch rest[0] {
		case "leader":
			m.Leader = true
		case "sponsor", "apprentice":
			if m.Relation != "" {
				return fmt.Errorf("mate %s: one of sponsor and apprentice", m.Name)
			}
			m.Relation = rest[0]
		case "online":
			m.Online = true
		case "hold":
			if len(rest) < 3 {
				return usage
			}
			n, err := ints(rest[1:3])
			if err != nil || n[1] < 1 {
				return fmt.Errorf("mate hold %q %q is not <item id> <count of 1 or more>", rest[1], rest[2])
			}
			m.Holds = append(m.Holds, Hold{Item: int32(n[0]), Count: int32(n[1])})
			rest = rest[2:]
		default:
			break flags
		}
		rest = rest[1:]
	}
	switch len(rest) {
	case 0:
	case 3:
		n, err := ints(rest)
		if err != nil {
			return fmt.Errorf("mate: %w", err)
		}
		m.DX, m.DY, m.DZ = n[0], n[1], n[2]
	default:
		return usage
	}
	for _, o := range sc.Mates {
		switch {
		case m.Leader && o.Leader:
			return fmt.Errorf("mate %s: the clan has one leader, %s", m.Name, o.Name)
		case m.Relation != "" && o.Relation == m.Relation:
			return fmt.Errorf("mate %s: the character has one %s, %s", m.Name, m.Relation, o.Name)
		}
	}
	sc.Mates = append(sc.Mates, m)
	return nil
}

// mate returns the mate line of name.
func (sc *Scenario) mate(name string) (Mate, bool) {
	i := slices.IndexFunc(sc.Mates, func(m Mate) bool { return m.Name == name })
	if i < 0 {
		return Mate{}, false
	}
	return sc.Mates[i], true
}

// LeaderMate returns the mate that leads the clan, false when the
// character does.
func (sc *Scenario) LeaderMate() (Mate, bool) {
	i := slices.IndexFunc(sc.Mates, func(m Mate) bool { return m.Leader })
	if i < 0 {
		return Mate{}, false
	}
	return sc.Mates[i], true
}

// checkHalls requires the hall, mate and clan lines to fit together, and
// the steps that name them to name what exists.
func (sc *Scenario) checkHalls() error {
	if len(sc.Mates) > 0 && sc.Clan < 0 {
		return fmt.Errorf("a mate line needs a clan line")
	}
	if sc.ClanHall != 0 && sc.Clan < 1 {
		return fmt.Errorf("a clan line with a hall needs a clan of level 1 or more")
	}
	if sc.Trace != "" {
		for _, m := range sc.Mates {
			switch {
			case !m.Leader:
				return fmt.Errorf("a traced scenario takes only the mate that leads the clan (mate %s): the reference probe plays no other", m.Name)
			case !m.Online && len(m.Holds) > 0:
				return fmt.Errorf("a traced scenario takes a hold line of an online mate only (mate %s): the reference probe stores no items of a mate that enters later", m.Name)
			}
		}
	}
	for _, st := range sc.Steps {
		switch {
		case st.As != "":
			if _, ok := sc.mate(st.As); !ok {
				return fmt.Errorf("%d: as %s: no mate line names it", st.Line, st.As)
			}
		case st.Verb == "enter" && len(st.Args) == 1:
			if _, ok := sc.mate(st.Args[0]); !ok {
				return fmt.Errorf("%d: enter %s: no mate line names it", st.Line, st.Args[0])
			}
		case st.Verb == "hallstatus":
			if !slices.ContainsFunc(sc.Halls, func(h Hall) bool { return fmt.Sprint(h.ID) == st.Args[0] }) {
				return fmt.Errorf("%d: hallstatus %s: no hall line names it", st.Line, st.Args[0])
			}
			if !slices.Contains(hallStatuses, st.Args[1]) {
				return fmt.Errorf("%d: hall status %q is not %s", st.Line, st.Args[1], strings.Join(hallStatuses, ", "))
			}
		}
		for name := range st.To {
			if _, ok := sc.mate(name); !ok && name != "player" {
				return fmt.Errorf("%d: to %s: no mate line names it", st.Line, name)
			}
		}
		for _, ck := range st.Checks {
			if ck.Kind == "items" && !strings.Contains(ck.Args[0], "=") {
				if _, ok := sc.mate(ck.Args[0]); !ok && !sc.isMember(ck.Args[0]) {
					return fmt.Errorf("%d: items %s: no mate or member line names it", ck.Line, ck.Args[0])
				}
			}
		}
	}
	return nil
}
