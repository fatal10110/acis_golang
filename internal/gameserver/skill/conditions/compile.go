package conditions

import (
	"fmt"
	"math"
	"sort"
	"strings"

	"github.com/fatal10110/acis_golang/internal/commons"
	"github.com/fatal10110/acis_golang/internal/gameserver/model/geometry"
	"github.com/fatal10110/acis_golang/internal/gameserver/model/item"
	modelskill "github.com/fatal10110/acis_golang/internal/gameserver/model/skill"
)

// Compile turns one parsed condition node into its runnable Condition.
// Element and attribute names match case-insensitively, and every
// recognized attribute of one <player>/<target>/<using>/<game> element must
// hold (they are ANDed); an unrecognized attribute is ignored.
//
// A node that holds no condition compiles to (nil, nil): an unknown element
// (the zero Condition among them), a <player>/<target>/<using>/<game> with
// no recognized attribute, or a <not> with no child. <and>/<or> drop such a
// child, a <not> wraps it (testing that <not> aborts, see Evaluate), and
// each caller decides what a nil root means: EvaluateSkill refuses the cast
// without feedback, a stat func gate is no gate. A <not> reads only its
// first child.
//
// A recognized element or attribute this package cannot evaluate yet
// (<skill>, <player race>) or a value that does not decode is an error,
// never a silent pass.
func Compile(node modelskill.Condition) (Condition, error) {
	switch strings.ToLower(node.Kind) {
	case "and", "or":
		var children []Condition
		for _, ch := range node.Children {
			c, err := Compile(ch)
			if err != nil {
				return nil, err
			}
			if c != nil {
				children = append(children, c)
			}
		}
		if strings.EqualFold(node.Kind, "and") {
			return &And{Conditions: children}, nil
		}
		return &Or{Conditions: children}, nil
	case "not":
		if len(node.Children) == 0 {
			return nil, nil
		}
		child, err := Compile(node.Children[0])
		if err != nil {
			return nil, err
		}
		return Not{Condition: child}, nil
	case "player":
		return compilePlayer(node)
	case "target":
		return compileAttrs(node, targetAttrs, compileTargetAttr)
	case "using":
		return compileAttrs(node, usingAttrs, compileUsingAttr)
	case "game":
		return compileAttrs(node, gameAttrs, compileGameAttr)
	case "skill":
		return nil, fmt.Errorf("condition: <%s> is not evaluated", node.Kind)
	default:
		return nil, nil
	}
}

// IsNull reports whether node holds no condition (see Compile). A node that
// does not compile is not null: it names a condition that cannot be built.
func IsNull(node modelskill.Condition) bool {
	c, err := Compile(node)
	return err == nil && c == nil
}

// The attributes each leaf element recognizes, lowercased; any other is
// ignored.
var (
	targetAttrs = attrSet("hp_min_max", "active_skill_id", "race_id", "npcid")
	usingAttrs  = attrSet("kind")
	gameAttrs   = attrSet("night")
	playerAttrs = attrSet("race", "level", "resting", "riding", "flying", "moving", "running", "behind",
		"front", "olympiad", "ishero", "hp", "mp", "pkcount", "battle_force", "spell_force", "charges",
		"weight", "invsize", "pledgeclass", "clanhall", "castle", "sex", "active_effect_id",
		"active_effect_id_lvl", "active_skill_id", "active_skill_id_lvl", "seed_fire", "seed_water",
		"seed_wind", "seed_various", "seed_any", "insidepoly")
)

func attrSet(names ...string) map[string]bool {
	set := make(map[string]bool, len(names))
	for _, n := range names {
		set[n] = true
	}
	return set
}

// compileAttrs ANDs one leaf per recognized attribute of node, in
// attribute-name order so a compiled tree is deterministic. A node with no
// recognized attribute holds no condition.
func compileAttrs(node modelskill.Condition, recognized map[string]bool, leaf func(name, value string) (Condition, error)) (Condition, error) {
	var conds []Condition
	for _, name := range sortedAttrNames(node.Attrs) {
		lower := strings.ToLower(name)
		if !recognized[lower] {
			continue
		}
		c, err := leaf(lower, node.Attrs[name])
		if err != nil {
			return nil, fmt.Errorf("condition: <%s %s=%q>: %w", node.Kind, name, node.Attrs[name], err)
		}
		conds = append(conds, c)
	}
	switch len(conds) {
	case 0:
		return nil, nil
	case 1:
		return conds[0], nil
	default:
		return &And{Conditions: conds}, nil
	}
}

func sortedAttrNames(attrs map[string]string) []string {
	names := make([]string, 0, len(attrs))
	for name := range attrs {
		names = append(names, name)
	}
	sort.Strings(names)
	return names
}

// compilePlayer handles <player>: most recognized attributes are one leaf
// each, but the five elemental-seed attributes fold into one ElementSeed
// and the two force attributes into one ForceBuff, each appended once after
// the rest and only when some seed is positive or the forces sum positive.
// A <player> left with no leaf holds no condition.
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
			seeds[seedIndex[lower]], err = DecodeInt(value)
		case "battle_force":
			forces[0], err = DecodeByte(value)
		case "spell_force":
			forces[1], err = DecodeByte(value)
		case "insidepoly":
			var c Condition
			c, err = compileInsidePoly(node, value)
			conds = append(conds, c)
		default:
			if !playerAttrs[lower] {
				continue
			}
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
		return nil, nil
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
		id, err := DecodeInt(value)
		if err != nil {
			return nil, err
		}
		if name == "active_skill_id" {
			return ActiveSkillID{SkillID: id, Level: -1}, nil
		}
		return ActiveEffectID{EffectID: id, Level: -1}, nil
	case "active_skill_id_lvl", "active_effect_id_lvl":
		id, level, err := DecodePair(value)
		if err != nil {
			return nil, err
		}
		if name == "active_skill_id_lvl" {
			return ActiveSkillID{SkillID: id, Level: level}, nil
		}
		return ActiveEffectID{EffectID: id, Level: level}, nil
	case "clanhall":
		ids, err := DecodeList(value)
		if err != nil {
			return nil, err
		}
		return HasClanHall{ClanHallIDs: ids}, nil
	case "race":
		return nil, fmt.Errorf("%s is not evaluated", name)
	}
	n, err := DecodeInt(value)
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
	return nil, fmt.Errorf("%s is not evaluated", name)
}

// compileInsidePoly reads the <zone minZ maxZ><node x y/>...</zone> child
// the insidePoly attribute's polygon lives in.
func compileInsidePoly(node modelskill.Condition, value string) (Condition, error) {
	if len(node.Children) == 0 {
		return nil, fmt.Errorf("missing <zone> child")
	}
	zone := node.Children[0]
	minZ, err := DecodeInt(zone.Attrs["minZ"])
	if err != nil {
		return nil, fmt.Errorf("zone minZ: %w", err)
	}
	maxZ, err := DecodeInt(zone.Attrs["maxZ"])
	if err != nil {
		return nil, fmt.Errorf("zone maxZ: %w", err)
	}
	points := make([]geometry.Point, 0, len(zone.Children))
	for _, n := range zone.Children {
		x, err := DecodeInt(n.Attrs["x"])
		if err != nil {
			return nil, fmt.Errorf("zone node x: %w", err)
		}
		y, err := DecodeInt(n.Attrs["y"])
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
		lo, hi, err := DecodePair(value)
		if err != nil {
			return nil, err
		}
		return TargetHpMinMax{Min: lo, Max: hi}, nil
	case "active_skill_id":
		id, err := DecodeInt(value)
		if err != nil {
			return nil, err
		}
		return TargetActiveSkillID{SkillID: id}, nil
	case "race_id":
		ids, err := DecodeList(value)
		if err != nil {
			return nil, err
		}
		return TargetRaceID{IDs: ids}, nil
	case "npcid":
		ids, err := DecodeList(value)
		if err != nil {
			return nil, err
		}
		return TargetNpcID{IDs: ids}, nil
	}
	return nil, fmt.Errorf("%s is not evaluated", name)
}

func compileUsingAttr(_, value string) (Condition, error) {
	return UsingItemType{Mask: int(item.ParseWornKindMask(value))}, nil
}

// compileGameAttr reads the one <game> attribute a data file can set,
// "night". A chance roll is a GameChance built in code (weapon cast/crit
// triggers), never a <game chance> attribute, which is ignored like any
// other unrecognized one.
func compileGameAttr(_, value string) (Condition, error) {
	return GameTime{Night: parseBool(value)}, nil
}

// parseBool is true only for a case-insensitive "true"; anything else,
// including "1", is false.
func parseBool(v string) bool { return strings.EqualFold(v, "true") }

// DecodeInt reads an int32 integer literal: decimal, "0x"/"#" hex or
// leading-zero octal (see commons.DecodeInt32).
func DecodeInt(v string) (int, error) {
	n, err := commons.DecodeInt32(v)
	return int(n), err
}

// DecodeByte is DecodeInt restricted to the signed 8-bit range.
func DecodeByte(v string) (int, error) {
	n, err := DecodeInt(v)
	if err != nil {
		return 0, err
	}
	if n < math.MinInt8 || n > math.MaxInt8 {
		return 0, fmt.Errorf("%q: value out of byte range", v)
	}
	return n, nil
}

// DecodePair reads an "a,b" integer pair; anything after a second comma is
// ignored.
func DecodePair(v string) (int, int, error) {
	parts := strings.Split(v, ",")
	if len(parts) < 2 {
		return 0, 0, fmt.Errorf("want two comma-separated values")
	}
	a, err := DecodeInt(parts[0])
	if err != nil {
		return 0, 0, err
	}
	b, err := DecodeInt(parts[1])
	return a, b, err
}

// DecodeList reads a comma-separated integer list. Empty entries between
// adjacent commas are skipped; every other entry is trimmed of control
// characters and spaces and must then be an integer literal, so a
// blank-but-not-empty entry is an error.
func DecodeList(v string) ([]int, error) {
	var out []int
	for _, tok := range strings.Split(v, ",") {
		if tok == "" {
			continue
		}
		tok = strings.TrimFunc(tok, func(r rune) bool { return r <= ' ' })
		n, err := DecodeInt(tok)
		if err != nil {
			return nil, err
		}
		out = append(out, n)
	}
	return out, nil
}
