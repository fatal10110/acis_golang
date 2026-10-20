package scenario

import (
	"fmt"
	"slices"
	"strconv"
	"strings"

	"github.com/fatal10110/acis_golang/internal/gameserver/model/actor/npc"
	modelskill "github.com/fatal10110/acis_golang/internal/gameserver/model/skill"
)

// Door is one door line: a fixture door of id standing at an offset from
// where the character first enters the world, named Role.
type Door struct {
	Role       string
	ID         int
	DX, DY, DZ int
}

// Summon is the summon line: the character's siege summon of a fixture
// template of id, named Role.
type Summon struct {
	Role string
	ID   int32
}

// fight reads the skill, door and summon lines.
func (sc *Scenario) fight(key string, args []string, roles map[string]bool) error {
	switch key {
	case "skill":
		if len(args) != 2 {
			return fmt.Errorf("skill <skill id> <level>")
		}
		ref, err := skillRef(args[0], args[1])
		if err != nil {
			return err
		}
		if slices.ContainsFunc(sc.Skills, func(r modelskill.Ref) bool { return r.ID == ref.ID }) {
			return fmt.Errorf("skill %d listed twice", ref.ID)
		}
		sc.Skills = append(sc.Skills, ref)
	case "door":
		if len(args) != 5 {
			return fmt.Errorf("door <role> <door id> <dx> <dy> <dz>")
		}
		if sc.named(args[0], roles) {
			return fmt.Errorf("role %q named twice", args[0])
		}
		n, err := ints(args[1:])
		if err != nil {
			return fmt.Errorf("door: %w", err)
		}
		if n[0] < 1 || slices.ContainsFunc(sc.Doors, func(d Door) bool { return d.ID == n[0] }) {
			return fmt.Errorf("door id %d is not 1 or more, or listed twice", n[0])
		}
		sc.Doors = append(sc.Doors, Door{Role: args[0], ID: n[0], DX: n[1], DY: n[2], DZ: n[3]})
	case "summon":
		if len(args) != 2 || sc.Summon != nil {
			return fmt.Errorf("one summon line: summon <role> <npc id>")
		}
		if sc.named(args[0], roles) {
			return fmt.Errorf("role %q named twice", args[0])
		}
		id, err := strconv.ParseInt(args[1], 10, 32)
		if err != nil || id < 1 {
			return fmt.Errorf("summon npc id %q is not a number of 1 or more", args[1])
		}
		sc.Summon = &Summon{Role: args[0], ID: int32(id)}
	}
	return nil
}

// skillRef reads a skill id and level of 1 or more.
func skillRef(id, level string) (modelskill.Ref, error) {
	n, err := strconv.ParseInt(id, 10, 32)
	if err != nil || n < 1 {
		return modelskill.Ref{}, fmt.Errorf("skill id %q is not a number of 1 or more", id)
	}
	lv, err := strconv.Atoi(level)
	if err != nil || lv < 1 {
		return modelskill.Ref{}, fmt.Errorf("skill level %q is not a number of 1 or more", level)
	}
	return modelskill.Ref{ID: modelskill.ID(n), Level: lv}, nil
}

// npcFightOption reads an npc line's clan or skill option at the head of
// args, returning what it left; ok is false when args starts with neither.
func npcFightOption(n *NPC, args []string) (rest []string, ok bool, err error) {
	switch args[0] {
	case "clan":
		if n.Clans != nil || len(args) < 3 {
			return nil, true, fmt.Errorf("npc option clan <name>[,<name>...] <range>, once")
		}
		names := strings.Split(args[1], ",")
		if slices.Contains(names, "") {
			return nil, true, fmt.Errorf("clan names %q: an empty name", args[1])
		}
		r, err := strconv.Atoi(args[2])
		if err != nil || r < 0 {
			return nil, true, fmt.Errorf("clan range %q is not a number of 0 or more", args[2])
		}
		n.Clans, n.ClanRange = names, r
		return args[3:], true, nil
	case "skill":
		if len(args) < 4 {
			return nil, true, fmt.Errorf("npc option skill <type> <skill id> <level>")
		}
		typ, known := npc.ParseSkillType(args[1])
		if !known || typ == npc.SkillTypePassive {
			return nil, true, fmt.Errorf("npc skill type %q is no type a behavior casts", args[1])
		}
		if _, dup := n.Skills[typ]; dup {
			return nil, true, fmt.Errorf("npc skill type %s listed twice", args[1])
		}
		ref, err := skillRef(args[2], args[3])
		if err != nil {
			return nil, true, err
		}
		if n.Skills == nil {
			n.Skills = map[npc.SkillType]modelskill.Ref{}
		}
		n.Skills[typ] = ref
		return args[4:], true, nil
	}
	return args, false, nil
}

// door returns the door line of role.
func (sc *Scenario) door(role string) (Door, bool) {
	i := slices.IndexFunc(sc.Doors, func(d Door) bool { return d.Role == role })
	if i < 0 {
		return Door{}, false
	}
	return sc.Doors[i], true
}

// summoned reports whether role names the summon line's summon.
func (sc *Scenario) summoned(role string) bool { return sc.Summon != nil && sc.Summon.Role == role }

// checkFightStep validates the hit, cast, jump and approach steps' roles
// and numbers: a hit lands on an NPC line's NPC or a door line's door,
// from the character, another NPC line's NPC or the summon line's summon.
func (sc *Scenario) checkFightStep(verb string, args []string, roles map[string]bool) error {
	switch verb {
	case "hit":
		if _, isDoor := sc.door(args[0]); !roles[args[0]] && !isDoor {
			return fmt.Errorf("unknown role %q: no npc or door line names it", args[0])
		}
		switch len(args) {
		case 2:
		case 4:
			if args[2] != "by" {
				return fmt.Errorf("hit <role> <damage> [by <role>]")
			}
			if by := args[3]; by == args[0] || (!roles[by] && !sc.summoned(by)) {
				return fmt.Errorf("hit by %q: no other npc line nor the summon line names it", by)
			}
		default:
			return fmt.Errorf("hit <role> <damage> [by <role>]")
		}
	case "cast":
		if !roles[args[0]] {
			return fmt.Errorf("unknown role %q", args[0])
		}
		id, err := strconv.ParseInt(args[1], 10, 32)
		if err != nil || !slices.ContainsFunc(sc.Skills, func(r modelskill.Ref) bool { return int64(r.ID) == id }) {
			return fmt.Errorf("cast skill %q: no skill line lists it", args[1])
		}
	case "jump":
		if !roles[args[0]] {
			return fmt.Errorf("unknown role %q", args[0])
		}
		if _, err := ints(args[1:]); err != nil {
			return fmt.Errorf("jump: %w", err)
		}
	case "approach":
		if len(args) == 2 {
			if d, err := strconv.Atoi(args[1]); err != nil || d < 1 {
				return fmt.Errorf("approach distance %q is not a number of 1 or more", args[1])
			}
		}
	}
	return nil
}

// checkFightChecks validates the hate, desires and at checks' roles: a
// hate or desires check reads an NPC line's NPC, an at check one of them
// or the character; a hate check's creature is the character (default),
// an NPC line's NPC or the summon line's summon.
func (sc *Scenario) checkFightChecks(roles map[string]bool) error {
	for _, st := range sc.Steps {
		for _, ck := range st.Checks {
			var bad string
			switch ck.Kind {
			case "hate":
				if !roles[ck.Args[0]] {
					bad = ck.Args[0]
				} else if len(ck.Args) == 3 && ck.Args[2] != "player" && !roles[ck.Args[2]] && !sc.summoned(ck.Args[2]) {
					bad = ck.Args[2]
				}
			case "desires":
				if !roles[ck.Args[0]] {
					bad = ck.Args[0]
				}
			case "at":
				if ck.Args[0] != "player" && !roles[ck.Args[0]] {
					bad = ck.Args[0]
				}
			}
			if bad != "" {
				return fmt.Errorf("%d: %s check: unknown role %q", ck.Line, ck.Kind, bad)
			}
		}
	}
	return nil
}

// checkFightCheck validates a hate, desires or at line's numbers.
func checkFightCheck(kind string, args []string) error {
	switch kind {
	case "hate":
		if len(args) < 2 || len(args) > 3 {
			return fmt.Errorf("hate <role> <weight> [<creature>]")
		}
		if _, err := strconv.ParseFloat(args[1], 64); err != nil {
			return fmt.Errorf("hate weight %q is not a number", args[1])
		}
	case "desires":
		if len(args) < 2 {
			return fmt.Errorf("desires <role> <desire> ... | -")
		}
		if len(args) == 2 && args[1] == "-" {
			return nil
		}
		for _, d := range args[1:] {
			if !validDesire(d) {
				return fmt.Errorf("desire %q is not <kind>(<args>)=<weight>", d)
			}
		}
	case "at":
		if len(args) != 4 {
			return fmt.Errorf("at <role> <dx> <dy> <dz>")
		}
		if _, err := ints(args[1:]); err != nil {
			return fmt.Errorf("at: %w", err)
		}
	}
	return nil
}

// validDesire reports whether d reads <kind>(<args>)=<weight>.
func validDesire(d string) bool {
	head, weight, ok := strings.Cut(d, ")=")
	if !ok {
		return false
	}
	kind, _, ok := strings.Cut(head, "(")
	if _, err := strconv.ParseFloat(weight, 64); !ok || kind == "" || err != nil {
		return false
	}
	return true
}

// tracedFight reports what of the fight lines a traced scenario uses, ""
// for none: the reference probe plays none of them.
func (sc *Scenario) tracedFight() string {
	switch {
	case len(sc.Skills) > 0 || len(sc.Doors) > 0 || sc.Summon != nil:
		return "skill, door or summon line"
	case slices.ContainsFunc(sc.NPCs, func(n NPC) bool { return n.Clans != nil || n.Skills != nil }):
		return "clan or skill option on an npc line"
	case slices.ContainsFunc(sc.Steps, func(st Step) bool {
		return (st.Verb == "hit" && len(st.Args) == 4) || (st.Verb == "approach" && len(st.Args) == 2)
	}):
		return "hit by another creature or approach to a distance"
	}
	return ""
}
