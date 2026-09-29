package conditions

import (
	"fmt"
	"sort"
	"strconv"
	"strings"

	"github.com/fatal10110/acis_golang/internal/gameserver/model/geometry"
	"github.com/fatal10110/acis_golang/internal/gameserver/model/item"
	modelskill "github.com/fatal10110/acis_golang/internal/gameserver/model/skill"
)

// Compile turns one parsed condition node into its runnable Condition.
// Element and attribute names match case-insensitively, and every attribute
// of one <player>/<target>/<using>/<game> element must hold (they are
// ANDed). An element or attribute this package cannot evaluate is an error,
// never a silent pass.
func Compile(node modelskill.Condition) (Condition, error) {
	switch strings.ToLower(node.Kind) {
	case "and", "or":
		children := make([]Condition, 0, len(node.Children))
		for _, ch := range node.Children {
			c, err := Compile(ch)
			if err != nil {
				return nil, err
			}
			children = append(children, c)
		}
		if strings.EqualFold(node.Kind, "and") {
			return &And{Conditions: children}, nil
		}
		return &Or{Conditions: children}, nil
	case "not":
		if len(node.Children) != 1 {
			return nil, fmt.Errorf("condition: not: want exactly one child, got %d", len(node.Children))
		}
		child, err := Compile(node.Children[0])
		if err != nil {
			return nil, err
		}
		return Not{Condition: child}, nil
	case "player":
		return compilePlayer(node)
	case "target":
		return compileAttrs(node, compileTargetAttr)
	case "using":
		return compileAttrs(node, compileUsingAttr)
	case "game":
		return compileAttrs(node, compileGameAttr)
	default:
		return nil, fmt.Errorf("condition: unsupported element %q", node.Kind)
	}
}

// compileAttrs ANDs one leaf per attribute of node, in attribute-name order
// so a compiled tree is deterministic.
func compileAttrs(node modelskill.Condition, leaf func(name, value string) (Condition, error)) (Condition, error) {
	names := sortedAttrNames(node.Attrs)
	if len(names) == 0 {
		return nil, fmt.Errorf("condition: <%s> has no attribute", node.Kind)
	}
	conds := make([]Condition, 0, len(names))
	for _, name := range names {
		c, err := leaf(strings.ToLower(name), node.Attrs[name])
		if err != nil {
			return nil, fmt.Errorf("condition: <%s %s=%q>: %w", node.Kind, name, node.Attrs[name], err)
		}
		conds = append(conds, c)
	}
	if len(conds) == 1 {
		return conds[0], nil
	}
	return &And{Conditions: conds}, nil
}

func sortedAttrNames(attrs map[string]string) []string {
	names := make([]string, 0, len(attrs))
	for name := range attrs {
		names = append(names, name)
	}
	sort.Strings(names)
	return names
}

// compilePlayer handles <player>: most attributes are one leaf each, but
// the five elemental-seed attributes fold into one ElementSeed and the two
// force attributes into one ForceBuff, each appended once after the rest.
func compilePlayer(node modelskill.Condition) (Condition, error) {
	var seeds [5]int
	var forces [2]int
	var conds []Condition
	for _, name := range sortedAttrNames(node.Attrs) {
		value := node.Attrs[name]
		lower := strings.ToLower(name)
		var err error
		switch lower {
		case "seed_fire", "seed_water", "seed_wind", "seed_various", "seed_any":
			seeds[seedIndex[lower]], err = decodeInt(value)
		case "battle_force":
			forces[0], err = decodeInt(value)
		case "spell_force":
			forces[1], err = decodeInt(value)
		case "insidepoly":
			var c Condition
			c, err = compileInsidePoly(node, value)
			conds = append(conds, c)
		default:
			var c Condition
			c, err = compilePlayerAttr(lower, value)
			conds = append(conds, c)
		}
		if err != nil {
			return nil, fmt.Errorf("condition: <player %s=%q>: %w", name, value, err)
		}
	}
	for _, s := range seeds {
		if s > 0 {
			conds = append(conds, ElementSeed{Required: seeds})
			break
		}
	}
	if forces[0]+forces[1] > 0 {
		conds = append(conds, ForceBuff{BattleForce: forces[0], SpellForce: forces[1]})
	}
	switch len(conds) {
	case 0:
		return nil, fmt.Errorf("condition: <player> has no recognized attribute in %v", node.Attrs)
	case 1:
		return conds[0], nil
	default:
		return &And{Conditions: conds}, nil
	}
}

var seedIndex = map[string]int{"seed_fire": 0, "seed_water": 1, "seed_wind": 2, "seed_various": 3, "seed_any": 4}

var playerStates = map[string]State{
	"resting":  StateResting,
	"moving":   StateMoving,
	"running":  StateRunning,
	"riding":   StateRiding,
	"flying":   StateFlying,
	"behind":   StateBehind,
	"front":    StateFront,
	"olympiad": StateOlympiad,
}

func compilePlayerAttr(name, value string) (Condition, error) {
	if state, ok := playerStates[name]; ok {
		return PlayerState{Check: state, Required: parseBool(value)}, nil
	}
	switch name {
	case "ishero":
		return IsHero{Want: parseBool(value)}, nil
	case "active_skill_id", "active_effect_id":
		id, err := decodeInt(value)
		if err != nil {
			return nil, err
		}
		if name == "active_skill_id" {
			return ActiveSkillID{SkillID: id, Level: -1}, nil
		}
		return ActiveEffectID{EffectID: id, Level: -1}, nil
	case "active_skill_id_lvl", "active_effect_id_lvl":
		id, level, err := decodePair(value)
		if err != nil {
			return nil, err
		}
		if name == "active_skill_id_lvl" {
			return ActiveSkillID{SkillID: id, Level: level}, nil
		}
		return ActiveEffectID{EffectID: id, Level: level}, nil
	case "clanhall":
		ids, err := decodeList(value)
		if err != nil {
			return nil, err
		}
		return HasClanHall{ClanHallIDs: ids}, nil
	}
	n, err := decodeInt(value)
	if err != nil {
		return nil, err
	}
	switch name {
	case "level":
		return Level{Level: n}, nil
	case "hp":
		return Hp{Percent: n}, nil
	case "mp":
		return Mp{Percent: n}, nil
	case "pkcount":
		return PkCount{Max: n}, nil
	case "charges":
		return Charges{Min: n}, nil
	case "weight":
		return Weight{Tier: n}, nil
	case "invsize":
		return InvSize{Size: n}, nil
	case "pledgeclass":
		return PledgeClass{Class: n}, nil
	case "castle":
		return HasCastle{CastleID: n}, nil
	case "sex":
		return Sex{Sex: n}, nil
	}
	return nil, fmt.Errorf("unsupported player attribute")
}

// compileInsidePoly reads the <zone minZ maxZ><node x y/>...</zone> child
// the insidePoly attribute's polygon lives in.
func compileInsidePoly(node modelskill.Condition, value string) (Condition, error) {
	if len(node.Children) == 0 {
		return nil, fmt.Errorf("missing <zone> child")
	}
	zone := node.Children[0]
	minZ, err := decodeInt(zone.Attrs["minZ"])
	if err != nil {
		return nil, fmt.Errorf("zone minZ: %w", err)
	}
	maxZ, err := decodeInt(zone.Attrs["maxZ"])
	if err != nil {
		return nil, fmt.Errorf("zone maxZ: %w", err)
	}
	points := make([]geometry.Point, 0, len(zone.Children))
	for _, n := range zone.Children {
		x, err := decodeInt(n.Attrs["x"])
		if err != nil {
			return nil, fmt.Errorf("zone node x: %w", err)
		}
		y, err := decodeInt(n.Attrs["y"])
		if err != nil {
			return nil, fmt.Errorf("zone node y: %w", err)
		}
		points = append(points, geometry.Point{X: x, Y: y})
	}
	poly, err := geometry.NewPolygon(points)
	if err != nil {
		return nil, err
	}
	territory, err := geometry.NewTerritory(minZ, maxZ, poly)
	if err != nil {
		return nil, err
	}
	return InsidePoly{Zone: territory, CheckInside: parseBool(value)}, nil
}

func compileTargetAttr(name, value string) (Condition, error) {
	switch name {
	case "hp_min_max":
		lo, hi, err := decodePair(value)
		if err != nil {
			return nil, err
		}
		return TargetHpMinMax{Min: lo, Max: hi}, nil
	case "active_skill_id":
		id, err := decodeInt(value)
		if err != nil {
			return nil, err
		}
		return TargetActiveSkillID{SkillID: id}, nil
	case "race_id":
		ids, err := decodeList(value)
		if err != nil {
			return nil, err
		}
		return TargetRaceID{IDs: ids}, nil
	case "npcid":
		ids, err := decodeList(value)
		if err != nil {
			return nil, err
		}
		return TargetNpcID{IDs: ids}, nil
	}
	return nil, fmt.Errorf("unsupported target attribute")
}

func compileUsingAttr(name, value string) (Condition, error) {
	if name != "kind" {
		return nil, fmt.Errorf("unsupported using attribute")
	}
	return UsingItemType{Mask: int(item.ParseWornKindMask(value))}, nil
}

func compileGameAttr(name, value string) (Condition, error) {
	switch name {
	case "night":
		return GameTime{Night: parseBool(value)}, nil
	case "chance":
		pct, err := decodeInt(value)
		if err != nil {
			return nil, err
		}
		return GameChance{Percent: pct}, nil
	}
	return nil, fmt.Errorf("unsupported game attribute")
}

// parseBool is true only for a case-insensitive "true"; anything else,
// including "1", is false.
func parseBool(v string) bool { return strings.EqualFold(v, "true") }

// decodeInt accepts decimal, 0x hex and leading-zero octal integers.
func decodeInt(v string) (int, error) {
	n, err := strconv.ParseInt(v, 0, 32)
	return int(n), err
}

// decodePair reads an "a,b" integer pair; anything after a second comma is
// ignored.
func decodePair(v string) (int, int, error) {
	parts := strings.Split(v, ",")
	if len(parts) < 2 {
		return 0, 0, fmt.Errorf("want two comma-separated values")
	}
	a, err := decodeInt(parts[0])
	if err != nil {
		return 0, 0, err
	}
	b, err := decodeInt(parts[1])
	return a, b, err
}

// decodeList reads a comma-separated integer list, trimming each entry.
func decodeList(v string) ([]int, error) {
	var out []int
	for _, tok := range strings.Split(v, ",") {
		tok = strings.TrimSpace(tok)
		if tok == "" {
			continue
		}
		n, err := decodeInt(tok)
		if err != nil {
			return nil, err
		}
		out = append(out, n)
	}
	return out, nil
}
