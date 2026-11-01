package scenario

import (
	"fmt"
	"math"
	"slices"
	"strconv"
	"strings"

	"github.com/fatal10110/acis_golang/internal/gameserver/model/actor/npc"
)

// Territory is one territory line: an npcmaker territory, the box between
// two offsets from where the character first enters the world.
type Territory struct {
	Name               string
	DX1, DY1, DX2, DY2 int
	DZ1, DZ2           int
}

// Member is one member line: a second character in the character's party.
type Member struct {
	Name  string
	Level int
	Dead  bool
	// Standing is its Ketra/Varka alliance standing (positive Ketra, negative
	// Varka, 0 none) and Holds the items it holds, both as it enters the world.
	Standing int
	Holds    []Hold
	// DX, DY and DZ are where it stands, an offset from where the character
	// first enters the world.
	DX, DY, DZ int
}

// ZoneLine is one zone line: a zone of Kind (boss or peace) and id ID, the box
// between those offsets from where the character first enters the world.
type ZoneLine struct {
	Kind               string
	ID                 int
	DX1, DY1, DX2, DY2 int
	DZ1, DZ2           int
}

// zoneKinds are the kinds of zone a zone line builds.
var zoneKinds = []string{"boss", "peace"}

// Ground is one ground line: Count of item template Item lying at an offset
// from the character, named Role. Cursed: the item is a cursed weapon whose
// life starts as it is laid.
type Ground struct {
	Role       string
	Item       int32
	Count      int32
	DX, DY, DZ int
	Cursed     bool
}

// named reports whether name already names the player, an NPC role, a
// member or a ground item.
func (sc *Scenario) named(name string, roles map[string]bool) bool {
	_, isDoor := sc.door(name)
	return name == "player" || roles[name] || isDoor || sc.summoned(name) ||
		slices.ContainsFunc(sc.Members, func(m Member) bool { return m.Name == name }) ||
		slices.ContainsFunc(sc.Mates, func(m Mate) bool { return m.Name == name }) ||
		slices.ContainsFunc(sc.Grounds, func(g Ground) bool { return g.Role == name })
}

// world reads the territory, zone, member, ground, roll and gauss lines.
func (sc *Scenario) world(key string, args []string, roles map[string]bool) error {
	switch key {
	case "territory":
		if len(args) != 7 {
			return fmt.Errorf("territory <name> <dx1> <dy1> <dx2> <dy2> <dz1> <dz2>")
		}
		if slices.ContainsFunc(sc.Territories, func(tr Territory) bool { return tr.Name == args[0] }) {
			return fmt.Errorf("territory %s listed twice", args[0])
		}
		n, err := ints(args[1:])
		if err != nil {
			return fmt.Errorf("territory: %w", err)
		}
		if n[0] >= n[2] || n[1] >= n[3] || n[4] > n[5] {
			return fmt.Errorf("territory %s is no box: want dx1 < dx2, dy1 < dy2 and dz1 <= dz2", args[0])
		}
		sc.Territories = append(sc.Territories, Territory{Name: args[0], DX1: n[0], DY1: n[1], DX2: n[2], DY2: n[3], DZ1: n[4], DZ2: n[5]})
	case "zone":
		usage := fmt.Errorf("zone <boss|peace> <id> <dx1> <dy1> <dx2> <dy2> <dz1> <dz2>")
		if len(args) != 8 {
			return usage
		}
		if !slices.Contains(zoneKinds, args[0]) {
			return fmt.Errorf("zone kind %q is not one of %s", args[0], strings.Join(zoneKinds, ", "))
		}
		n, err := ints(args[1:])
		if err != nil {
			return fmt.Errorf("zone: %w", err)
		}
		if n[0] < 1 || n[0] == battlefieldID || slices.ContainsFunc(sc.Zones, func(z ZoneLine) bool { return z.ID == n[0] }) {
			return fmt.Errorf("zone id %d is not 1 or more, the summon battlefield's, or listed twice", n[0])
		}
		if n[1] >= n[3] || n[2] >= n[4] || n[5] > n[6] {
			return fmt.Errorf("zone %d is no box: want dx1 < dx2, dy1 < dy2 and dz1 <= dz2", n[0])
		}
		sc.Zones = append(sc.Zones, ZoneLine{Kind: args[0], ID: n[0], DX1: n[1], DY1: n[2], DX2: n[3], DY2: n[4], DZ1: n[5], DZ2: n[6]})
	case "member":
		usage := fmt.Errorf("member <name> <level> [dead] [standing <n>] [hold <item id> <count>]... [<dx> <dy> <dz>]")
		if len(args) < 2 {
			return usage
		}
		if sc.named(args[0], roles) || args[0] == sc.Character {
			return fmt.Errorf("member %q named twice", args[0])
		}
		level, err := strconv.Atoi(args[1])
		if err != nil || level < 1 {
			return fmt.Errorf("member level %q is not a number of 1 or more", args[1])
		}
		m, rest := Member{Name: args[0], Level: level}, args[2:]
	options:
		for len(rest) > 0 {
			switch rest[0] {
			case "dead":
				m.Dead = true
			case "standing":
				if len(rest) < 2 {
					return usage
				}
				v, err := strconv.Atoi(rest[1])
				if err != nil || v == 0 || v > maxStanding || v < -maxStanding {
					return fmt.Errorf("member standing %q is not 1 to %d (Ketra) or -1 to -%d (Varka)", rest[1], maxStanding, maxStanding)
				}
				m.Standing = v
				rest = rest[1:]
			case "hold":
				if len(rest) < 3 {
					return usage
				}
				n, err := ints(rest[1:3])
				if err != nil || n[1] < 1 {
					return fmt.Errorf("member hold %q %q is not <item id> <count of 1 or more>", rest[1], rest[2])
				}
				m.Holds = append(m.Holds, Hold{Item: int32(n[0]), Count: int32(n[1])})
				rest = rest[2:]
			default:
				break options
			}
			rest = rest[1:]
		}
		switch len(rest) {
		case 0:
		case 3:
			n, err := ints(rest)
			if err != nil {
				return fmt.Errorf("member: %w", err)
			}
			m.DX, m.DY, m.DZ = n[0], n[1], n[2]
		default:
			return usage
		}
		sc.Members = append(sc.Members, m)
	case "roll":
		if len(args) != 2 {
			return fmt.Errorf("roll <role> <draw>")
		}
		if args[0] != "player" && !roles[args[0]] {
			return fmt.Errorf("roll: no NPC line above names role %q", args[0])
		}
		if _, dup := sc.Rolls[args[0]]; dup {
			return fmt.Errorf("roll: role %s listed twice", args[0])
		}
		draw, err := strconv.Atoi(args[1])
		if err != nil || draw < 0 {
			return fmt.Errorf("roll draw %q is not a number of 0 or more", args[1])
		}
		if sc.Rolls == nil {
			sc.Rolls = map[string]int{}
		}
		sc.Rolls[args[0]] = draw
	case "gauss":
		if len(args) != 2 {
			return fmt.Errorf("gauss <role> <value>")
		}
		if !roles[args[0]] {
			return fmt.Errorf("gauss: no NPC line above names role %q", args[0])
		}
		if _, dup := sc.Gauss[args[0]]; dup {
			return fmt.Errorf("gauss: role %s listed twice", args[0])
		}
		v, err := strconv.ParseFloat(args[1], 64)
		if err != nil || math.IsNaN(v) || math.IsInf(v, 0) {
			return fmt.Errorf("gauss value %q is not a number", args[1])
		}
		if sc.Gauss == nil {
			sc.Gauss = map[string]float64{}
		}
		sc.Gauss[args[0]] = v
	case "ground":
		cursed := len(args) == 7 && args[6] == "cursed"
		if cursed {
			args = args[:6]
		}
		if len(args) != 6 {
			return fmt.Errorf("ground <role> <item id> <count> <dx> <dy> <dz> [cursed]")
		}
		if sc.named(args[0], roles) {
			return fmt.Errorf("role %q named twice", args[0])
		}
		n, err := ints(args[1:])
		if err != nil {
			return fmt.Errorf("ground: %w", err)
		}
		if n[1] < 1 {
			return fmt.Errorf("ground count %d is not 1 or more", n[1])
		}
		if cursed && n[1] != 1 {
			return fmt.Errorf("ground %s: a cursed weapon lies as one item, not %d", args[0], n[1])
		}
		sc.Grounds = append(sc.Grounds, Ground{Role: args[0], Item: int32(n[0]), Count: int32(n[1]), DX: n[2], DY: n[3], DZ: n[4], Cursed: cursed})
	}
	return nil
}

// npcOptions reads an npc line's options past its offsets: walks, and
// maker naming a territory line above.
func npcOptions(args []string, territories []Territory) (NPC, error) {
	var n NPC
	for len(args) > 0 {
		rest, ok, err := npcFightOption(&n, args)
		if err == nil && !ok {
			rest, ok, err = npcStateOption(&n, args)
		}
		if err != nil {
			return NPC{}, err
		}
		if ok {
			args = rest
			continue
		}
		switch {
		case args[0] == "walks" && !n.Walks:
			n.Walks, args = true, args[1:]
		case args[0] == "maker" && n.Maker == "" && len(args) >= 2:
			name := args[1]
			if !slices.ContainsFunc(territories, func(tr Territory) bool { return tr.Name == name }) {
				return NPC{}, fmt.Errorf("maker territory %q is no territory line above", name)
			}
			n.Maker, args = name, args[2:]
		case args[0] == "ai" && n.AI == nil && len(args) >= 2:
			n.AI = npc.AIParams{}
			for _, kv := range strings.Split(args[1], ",") {
				k, v, ok := strings.Cut(kv, "=")
				if !ok || k == "" {
					return NPC{}, fmt.Errorf("npc ai parameter %q is not <name>=<value>", kv)
				}
				n.AI[k] = v
			}
			args = args[2:]
		case args[0] == "params" && n.Params == [3]string{} && len(args) >= 4:
			copy(n.Params[:], args[1:4])
			args = args[4:]
		default:
			return NPC{}, fmt.Errorf("npc option %q is not walks, maker <territory>, clan, skill, ai, params <p1> <p2> <p3>, level, aggro, mem, privates or master", args[0])
		}
	}
	return n, nil
}

// ints reads 32-bit numbers.
func ints(args []string) ([]int, error) {
	out := make([]int, len(args))
	for i, a := range args {
		v, err := strconv.ParseInt(a, 10, 32)
		if err != nil {
			return nil, fmt.Errorf("number %q: %w", a, err)
		}
		out[i] = int(v)
	}
	return out, nil
}

// look is a look step's arguments past the role.
type look struct {
	radius   int
	quantity int32
	items    []int32
}

// lookArgs reads a look step's arguments: role, radius, quantity, items.
func lookArgs(args []string) (look, error) {
	n, err := ints(args[1:])
	if err != nil {
		return look{}, fmt.Errorf("look: %w", err)
	}
	if n[0] < 1 || n[1] < 1 {
		return look{}, fmt.Errorf("look radius %d and quantity %d must be 1 or more", n[0], n[1])
	}
	l := look{radius: n[0], quantity: int32(n[1])}
	for _, id := range n[2:] {
		l.items = append(l.items, int32(id))
	}
	return l, nil
}
