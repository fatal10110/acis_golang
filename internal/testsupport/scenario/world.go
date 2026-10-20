package scenario

import (
	"fmt"
	"slices"
	"strconv"
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
	// DX, DY and DZ are where it stands, an offset from where the character
	// first enters the world.
	DX, DY, DZ int
}

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
		slices.ContainsFunc(sc.Grounds, func(g Ground) bool { return g.Role == name })
}

// world reads the territory, member, ground and roll lines.
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
	case "member":
		usage := fmt.Errorf("member <name> <level> [dead] [<dx> <dy> <dz>]")
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
		if len(rest) > 0 && rest[0] == "dead" {
			m.Dead, rest = true, rest[1:]
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
		if !roles[args[0]] {
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
		default:
			return NPC{}, fmt.Errorf("npc option %q is not walks, maker <territory>, clan or skill", args[0])
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
