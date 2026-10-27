package scenario

import (
	"fmt"
	"slices"
	"strconv"
	"strings"
	"time"

	"github.com/fatal10110/acis_golang/internal/gameserver/model/actor/npc"
	modelskill "github.com/fatal10110/acis_golang/internal/gameserver/model/skill"
)

// MemSet is one slot of an npc line's mem option: the script memory slot an
// NPC starts each life with, and the value.
type MemSet struct {
	Slot  string
	Value string
}

// memSlot is where a script memory slot lives in the NPC's scratch: an
// integer slot, a creature slot, or the script value.
type memSlot struct {
	integer  npc.IntSlot
	creature npc.CreatureSlot
	isCrea   bool
	isValue  bool
}

// slotCount is how many numbered slots each script memory family has.
const slotCount = 5

// memSlots are the script memory slots a scenario names: i_ai0 to i_ai4 and
// i_quest0 to i_quest4 (integers), c_ai0 to c_ai4 and c_quest0 to c_quest4
// (creatures), flag, weight (the weight point) and value (the script value).
var memSlots = func() map[string]memSlot {
	m := map[string]memSlot{
		"flag":   {integer: npc.IntFlag},
		"weight": {integer: npc.IntWeightPoint},
		"value":  {isValue: true},
	}
	for i := range slotCount {
		n := strconv.Itoa(i)
		m["i_ai"+n] = memSlot{integer: npc.IntAI0 + npc.IntSlot(i)}
		m["i_quest"+n] = memSlot{integer: npc.IntQuest0 + npc.IntSlot(i)}
		m["c_ai"+n] = memSlot{creature: npc.CreatureAI0 + npc.CreatureSlot(i), isCrea: true}
		m["c_quest"+n] = memSlot{creature: npc.CreatureQuest0 + npc.CreatureSlot(i), isCrea: true}
	}
	return m
}()

// noCreature is the value of an empty creature slot.
const noCreature = "-"

// parseMem reads <slot>=<value>[,<slot>=<value>...]: an integer slot takes a
// number, a creature slot the character (player), the role of an NPC line or
// - for none.
func parseMem(arg string) ([]MemSet, error) {
	var out []MemSet
	for _, kv := range strings.Split(arg, ",") {
		slot, value, ok := strings.Cut(kv, "=")
		def, known := memSlots[slot]
		switch {
		case !ok || !known:
			return nil, fmt.Errorf("memory %q is not <slot>=<value> of a slot i_ai0-4, i_quest0-4, c_ai0-4, c_quest0-4, flag, weight or value", kv)
		case slices.ContainsFunc(out, func(m MemSet) bool { return m.Slot == slot }):
			return nil, fmt.Errorf("memory slot %s listed twice", slot)
		case value == "":
			return nil, fmt.Errorf("memory %q has no value", kv)
		case !def.isCrea:
			if _, err := strconv.ParseInt(value, 10, 32); err != nil {
				return nil, fmt.Errorf("memory %s value %q is not a number", slot, value)
			}
		}
		out = append(out, MemSet{Slot: slot, Value: value})
	}
	return out, nil
}

// npcStateOption reads an npc line's level, aggro, mem, privates or master
// option at the head of args, returning what it left; ok is false when args
// starts with none of them.
func npcStateOption(n *NPC, args []string) (rest []string, ok bool, err error) {
	if len(args) < 2 {
		return args, false, nil
	}
	switch args[0] {
	case "level":
		v, err := strconv.Atoi(args[1])
		if n.Level != 0 || err != nil || v < 1 {
			return nil, true, fmt.Errorf("npc option level <n> of 1 or more, once")
		}
		n.Level = v
	case "aggro":
		v, err := strconv.Atoi(args[1])
		if n.Aggro != 0 || err != nil || v < 1 {
			return nil, true, fmt.Errorf("npc option aggro <range> of 1 or more, once")
		}
		n.Aggro = v
	case "mem":
		if n.Mem != nil {
			return nil, true, fmt.Errorf("npc option mem listed twice")
		}
		mem, err := parseMem(args[1])
		if err != nil {
			return nil, true, err
		}
		n.Mem = mem
	case "privates":
		if n.Privates != nil {
			return nil, true, fmt.Errorf("npc option privates listed twice")
		}
		for _, p := range strings.Split(args[1], ",") {
			e, err := parsePrivate(p)
			if err != nil {
				return nil, true, err
			}
			n.Privates = append(n.Privates, e)
		}
	case "master":
		if n.Master != "" {
			return nil, true, fmt.Errorf("npc option master listed twice")
		}
		n.Master = args[1]
	default:
		return args, false, nil
	}
	return args[2:], true, nil
}

// parsePrivate reads <npc id>[:<weight>[:<respawn ms>]], a private the
// template lists (weight default 1, respawn default none).
func parsePrivate(s string) (npc.PrivateEntry, error) {
	f := strings.Split(s, ":")
	bad := fmt.Errorf("private %q is not <npc id>[:<weight>[:<respawn ms>]]", s)
	if len(f) > 3 {
		return npc.PrivateEntry{}, bad
	}
	n := make([]int, len(f))
	for i, v := range f {
		x, err := strconv.Atoi(v)
		if err != nil || x < 0 || (i == 0 && x < 1) {
			return npc.PrivateEntry{}, bad
		}
		n[i] = x
	}
	e := npc.PrivateEntry{NpcID: n[0], Weight: 1}
	if len(n) > 1 {
		e.Weight = n[1]
	}
	if len(n) > 2 {
		e.RespawnDelay = time.Duration(n[2]) * time.Millisecond
	}
	return e, nil
}

// checkNPCState checks an npc line's mem and master options against the
// roles above it.
func (sc *Scenario) checkNPCState(n NPC, roles map[string]bool) error {
	for _, m := range n.Mem {
		if memSlots[m.Slot].isCrea && m.Value != noCreature && m.Value != "player" && !roles[m.Value] {
			return fmt.Errorf("npc %s mem %s: %q is not player, - or the role of an npc line above", n.Role, m.Slot, m.Value)
		}
	}
	if n.Master != "" && !roles[n.Master] {
		return fmt.Errorf("npc %s master %q: no npc line above names it", n.Role, n.Master)
	}
	return nil
}

// hitArgs is a hit step's arguments past the role and damage.
type hitArgs struct {
	by    string
	skill modelskill.Ref
	// hasSkill tells a skill was given.
	hasSkill bool
}

// parseHit reads a hit step's optional "by <role>" and "skill <id> <level>".
func parseHit(args []string) (hitArgs, error) {
	usage := fmt.Errorf("hit <role> <damage> [by <role>] [skill <skill id> <level>]")
	h := hitArgs{by: "player"}
	rest := args[2:]
	if len(rest) >= 2 && rest[0] == "by" {
		h.by, rest = rest[1], rest[2:]
	}
	if len(rest) == 3 && rest[0] == "skill" {
		ref, err := skillRef(rest[1], rest[2])
		if err != nil {
			return hitArgs{}, err
		}
		h.skill, h.hasSkill, rest = ref, true, nil
	}
	if len(rest) != 0 {
		return hitArgs{}, usage
	}
	return h, nil
}

// stateVerbs are the steps that set or run an NPC's or the character's
// state.
var stateVerbs = []string{"hp", "die", "top", "land", "stance", "swing"}

// checkStateStep validates the hp, die, top, land, stance and swing steps'
// roles and numbers against the roles of the NPC lines and the summon line.
func (sc *Scenario) checkStateStep(verb string, args []string, roles map[string]bool) error {
	creature := func(what, name string) error {
		if name != "player" && !roles[name] && !sc.summoned(name) {
			return fmt.Errorf("%s %q: no npc line nor the summon line names it, and it is not player", what, name)
		}
		return nil
	}
	switch verb {
	case "hp":
		if args[0] != "player" && !roles[args[0]] {
			return fmt.Errorf("hp: unknown role %q", args[0])
		}
		if v, err := strconv.Atoi(args[1]); err != nil || v < 0 {
			return fmt.Errorf("hp %q is not a number of 0 or more", args[1])
		}
	case "die":
		if len(args) == 1 && !roles[args[0]] {
			return fmt.Errorf("die by %q: no npc line names it", args[0])
		}
	case "top":
		if !roles[args[0]] {
			return fmt.Errorf("top: unknown role %q", args[0])
		}
		if args[1] != noCreature {
			return creature("top creature", args[1])
		}
	case "land":
		if err := creature("land target", args[0]); err != nil {
			return err
		}
		if _, err := skillRef(args[1], args[2]); err != nil {
			return err
		}
		switch len(args) {
		case 3:
		case 5:
			if args[3] != "by" {
				return fmt.Errorf("land <target> <skill id> <level> [by <creature>]")
			}
			return creature("land by", args[4])
		default:
			return fmt.Errorf("land <target> <skill id> <level> [by <creature>]")
		}
	case "stance":
		if !roles[args[0]] || (args[1] != "on" && args[1] != "off") {
			return fmt.Errorf("stance <role> <on|off>")
		}
	case "swing":
		if !roles[args[0]] {
			return fmt.Errorf("swing: unknown role %q", args[0])
		}
	}
	return nil
}

// stateChecks are the checks on an NPC's or the character's state: inworld
// tells whether the NPC is still in the world (a corpse is, a deleted NPC is
// not).
var stateChecks = []string{"mem", "hp", "privates", "master", "top", "effect", "dead", "inworld", "stance"}

// checkStateCheck validates a state check's arguments.
func checkStateCheck(kind string, args []string) error {
	usage := map[string]string{
		"mem":      "mem <role> <slot>=<value> ...",
		"hp":       "hp <role|player> <hp>",
		"privates": "privates <role> <npc id>=<count>[:<p1>,<p2>,<p3>] ... | -",
		"master":   "master <role> <role|->",
		"top":      "top <role> <creature|->",
		"effect":   "effect <role|player> <skill id> <held|none>",
		"dead":     "dead <role|player> <yes|no>",
		"inworld":  "inworld <role> <yes|no>",
		"stance":   "stance <role> <on|off>",
	}[kind]
	bad := fmt.Errorf("%s", usage)
	switch kind {
	case "mem":
		if len(args) < 2 {
			return bad
		}
		for _, a := range args[1:] {
			if _, err := parseMem(a); err != nil {
				return err
			}
		}
	case "hp":
		if len(args) != 2 {
			return bad
		}
		if v, err := strconv.Atoi(args[1]); err != nil || v < 0 {
			return fmt.Errorf("hp %q is not a number of 0 or more", args[1])
		}
	case "privates":
		if len(args) < 2 {
			return bad
		}
		if len(args) == 2 && args[1] == "-" {
			return nil
		}
		for _, a := range args[1:] {
			id, count, ok := strings.Cut(a, "=")
			count, params, hasParams := strings.Cut(count, ":")
			_, e1 := strconv.ParseInt(id, 10, 32)
			c, e2 := strconv.Atoi(count)
			if !ok || e1 != nil || e2 != nil || c < 1 || (hasParams && len(strings.Split(params, ",")) != 3) {
				return fmt.Errorf("private %q is not <npc id>=<count>[:<p1>,<p2>,<p3>] of 1 or more", a)
			}
		}
	case "master", "top":
		if len(args) != 2 {
			return bad
		}
	case "effect":
		if len(args) != 3 || (args[2] != "held" && args[2] != "none") {
			return bad
		}
		if _, err := skillRef(args[1], "1"); err != nil {
			return err
		}
	case "dead", "inworld":
		if len(args) != 2 || (args[1] != "yes" && args[1] != "no") {
			return bad
		}
	case "stance":
		if len(args) != 2 || (args[1] != "on" && args[1] != "off") {
			return bad
		}
	}
	return nil
}

// checkStateRoles validates the state checks' roles and creatures.
func (sc *Scenario) checkStateRoles(roles map[string]bool) error {
	for _, st := range sc.Steps {
		for _, ck := range st.Checks {
			if !slices.Contains(stateChecks, ck.Kind) {
				continue
			}
			bad := ""
			switch ck.Kind {
			case "hp", "effect", "dead":
				if ck.Args[0] != "player" && !roles[ck.Args[0]] {
					bad = ck.Args[0]
				}
			case "master":
				switch {
				case !roles[ck.Args[0]]:
					bad = ck.Args[0]
				case ck.Args[1] != noCreature && !roles[ck.Args[1]]:
					bad = ck.Args[1]
				}
			case "top":
				switch {
				case !roles[ck.Args[0]]:
					bad = ck.Args[0]
				case ck.Args[1] != noCreature && ck.Args[1] != "player" && !roles[ck.Args[1]] && !sc.summoned(ck.Args[1]):
					bad = ck.Args[1]
				}
			default:
				if !roles[ck.Args[0]] {
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

// tracedState reports what of the state lines a traced scenario uses, ""
// for none: the reference probe plays none of them.
func (sc *Scenario) tracedState() string {
	switch {
	case slices.ContainsFunc(sc.NPCs, func(n NPC) bool {
		return n.Level != 0 || n.Aggro != 0 || n.Mem != nil || n.Privates != nil || n.Master != ""
	}):
		return "level, aggro, mem, privates or master option on an npc line"
	case slices.ContainsFunc(sc.Steps, func(st Step) bool {
		return slices.Contains(stateVerbs, st.Verb) || st.Random != nil || (st.Verb == "hit" && len(st.Args) >= 5)
	}):
		return "hp, die, top, land, stance or swing step, skill hit or random line in a step"
	}
	return ""
}
