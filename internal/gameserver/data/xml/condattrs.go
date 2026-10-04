package xml

import (
	"errors"
	"fmt"
	"strings"

	"github.com/fatal10110/acis_golang/internal/gameserver/model/restart"
	"github.com/fatal10110/acis_golang/internal/gameserver/skill/conditions"
	"github.com/fatal10110/acis_golang/internal/gameserver/skill/stat"
)

// errTableRefNotAllowed marks a "#name" value where the reader takes no
// table reference.
var errTableRefNotAllowed = errors.New("table reference not allowed here")

// tableResolver resolves a "#name" value against the tables of the template
// a condition belongs to. A nil resolver means that template has no tables:
// the conditions of an item, and those inside a skill's <effect>.
type tableResolver func(name, val string) string

// condRole is where a condition element sits, which decides the attributes
// the condition reader takes from it.
type condRole int

const (
	condRoleUnread    condRole = iota // never read as a condition
	condRolePredicate                 // a predicate or and/or/not element
	condRolePolyZone                  // the <zone> of a <player insidePoly>
	condRolePolyNode                  // a polygon point of that <zone>
)

// condAttrRead is how the condition reader takes one attribute's value.
type condAttrRead int

const (
	condAttrRaw          condAttrRead = iota // as written: a "#" value is not a table reference
	condAttrTemplate                         // may name a table of the template
	condAttrTemplatePair                     // like condAttrTemplate, then again for each of the first two comma parts
	condAttrNoTable                          // may never name a table
	condAttrNoTableList                      // comma list whose trimmed entries may never name a table
)

func conditionAttrRead(role condRole, kind, name string) condAttrRead {
	switch role {
	case condRolePolyZone:
		if name == "minZ" || name == "maxZ" {
			return condAttrNoTable
		}
		return condAttrRaw
	case condRolePolyNode:
		if name == "x" || name == "y" {
			return condAttrNoTable
		}
		return condAttrRaw
	case condRolePredicate:
	default:
		return condAttrRaw
	}

	name = strings.ToLower(name)
	switch strings.ToLower(kind) {
	case "player":
		switch name {
		case "level", "pkcount", "charges", "active_effect_id", "active_skill_id":
			return condAttrTemplate
		case "active_effect_id_lvl", "active_skill_id_lvl":
			return condAttrTemplatePair
		case "hp", "mp", "battle_force", "spell_force", "weight", "invsize", "pledgeclass", "castle", "sex",
			"seed_fire", "seed_water", "seed_wind", "seed_various", "seed_any", "insidepoly":
			return condAttrNoTable
		case "clanhall":
			return condAttrNoTableList
		}
	case "target":
		switch name {
		case "active_skill_id":
			return condAttrTemplate
		case "hp_min_max":
			return condAttrTemplatePair
		case "race_id", "npcid":
			return condAttrNoTableList
		}
	}
	return condAttrRaw
}

// childConditionRole is the role of the i-th child element of n.
func childConditionRole(n condNode, role condRole, i int) condRole {
	switch role {
	case condRolePolyZone:
		return condRolePolyNode
	case condRolePredicate:
	default:
		return condRoleUnread
	}
	switch strings.ToLower(n.XMLName.Local) {
	case "and", "or":
		return condRolePredicate
	case "not":
		if i == 0 {
			return condRolePredicate
		}
	case "player":
		if i == 0 && hasAttrFold(n, "insidePoly") {
			return condRolePolyZone
		}
	}
	return condRoleUnread
}

func hasAttrFold(n condNode, name string) bool {
	for _, a := range n.Attrs {
		if strings.EqualFold(a.Name.Local, name) {
			return true
		}
	}
	return false
}

// conditionAttrs returns the attribute values of one condition element as
// the condition reader takes them. A table reference resolves only where the
// reader accepts one and resolve is set; anywhere else it fails the load.
// A repeated attribute name keeps the last value.
func conditionAttrs(n condNode, role condRole, resolve tableResolver) (map[string]string, error) {
	vals := make(map[string]string, len(n.Attrs))
	for _, a := range n.Attrs {
		name, val := a.Name.Local, a.Value
		var err error
		switch conditionAttrRead(role, n.XMLName.Local, name) {
		case condAttrTemplate:
			val, err = templateValue(name, val, resolve)
		case condAttrTemplatePair:
			val, err = resolveConditionPair(name, val, resolve)
		case condAttrNoTable:
			err = noTableRef(name, val)
		case condAttrNoTableList:
			for _, entry := range strings.Split(val, ",") {
				if err = noTableRef(name, strings.TrimFunc(entry, func(r rune) bool { return r <= ' ' })); err != nil {
					break
				}
			}
		}
		if err != nil {
			return nil, err
		}
		vals[name] = val
	}
	if err := checkConditionValues(n, role, vals); err != nil {
		return nil, err
	}
	return vals, nil
}

// condAttrDecode is how the condition reader decodes one attribute's value
// when it loads the condition.
type condAttrDecode int

const (
	condDecodeNone condAttrDecode = iota // read as written, or not at all
	condDecodeInt                        // an int32 literal
	condDecodeByte                       // an int8 literal
	condDecodePair                       // the first two comma parts are int32 literals
	condDecodeList                       // a comma list of int32 literals
	condDecodeRace                       // a race name
	condDecodeStat                       // a stat name
)

func conditionAttrDecode(role condRole, kind, name string) condAttrDecode {
	switch role {
	case condRolePolyZone:
		if name == "minZ" || name == "maxZ" {
			return condDecodeInt
		}
		return condDecodeNone
	case condRolePolyNode:
		if name == "x" || name == "y" {
			return condDecodeInt
		}
		return condDecodeNone
	case condRolePredicate:
	default:
		return condDecodeNone
	}

	// A <skill> condition reads only an attribute spelled exactly "stat";
	// every other condition attribute matches case-insensitively.
	if strings.EqualFold(kind, "skill") {
		if name == "stat" {
			return condDecodeStat
		}
		return condDecodeNone
	}
	name = strings.ToLower(name)
	switch strings.ToLower(kind) {
	case "player":
		switch name {
		case "level", "pkcount", "charges", "active_effect_id", "active_skill_id", "hp", "mp", "weight",
			"invsize", "pledgeclass", "castle", "sex", "seed_fire", "seed_water", "seed_wind", "seed_various", "seed_any":
			return condDecodeInt
		case "battle_force", "spell_force":
			return condDecodeByte
		case "active_effect_id_lvl", "active_skill_id_lvl":
			return condDecodePair
		case "clanhall":
			return condDecodeList
		case "race":
			return condDecodeRace
		}
	case "target":
		switch name {
		case "active_skill_id":
			return condDecodeInt
		case "hp_min_max":
			return condDecodePair
		case "race_id", "npcid":
			return condDecodeList
		}
	}
	return condDecodeNone
}

// conditionRequiredAttrs are the attributes the condition reader reads from
// an element in role whether or not the element has them, so a missing one
// fails the load.
func conditionRequiredAttrs(n condNode, role condRole) []string {
	switch role {
	case condRolePolyZone:
		return []string{"minZ", "maxZ"}
	case condRolePolyNode:
		return []string{"x", "y"}
	case condRolePredicate:
		if strings.EqualFold(n.XMLName.Local, "skill") {
			return []string{"stat"}
		}
	}
	return nil
}

// checkConditionValues fails a condition element whose values, as resolved
// in vals, the condition reader cannot decode when it loads the condition,
// or which lacks an attribute the reader requires. A <player insidePoly>
// needs its <zone> child. Everything else the reader takes as written, or
// only when the condition is tested.
func checkConditionValues(n condNode, role condRole, vals map[string]string) error {
	for _, name := range conditionRequiredAttrs(n, role) {
		if _, ok := vals[name]; !ok {
			return fmt.Errorf("attribute %q is missing", name)
		}
	}
	for _, a := range n.Attrs {
		name := a.Name.Local
		val := vals[name]
		var err error
		switch conditionAttrDecode(role, n.XMLName.Local, name) {
		case condDecodeInt:
			_, err = conditions.DecodeInt(val)
		case condDecodeByte:
			_, err = conditions.DecodeByte(val)
		case condDecodePair:
			_, _, err = conditions.DecodePair(val)
		case condDecodeList:
			_, err = conditions.DecodeList(val)
		case condDecodeRace:
			_, err = restart.ParseRace(val)
		case condDecodeStat:
			_, err = stat.ByName(val)
		}
		if err != nil {
			return fmt.Errorf("attribute %q: %w", name, err)
		}
		if role == condRolePredicate && strings.EqualFold(n.XMLName.Local, "player") &&
			strings.EqualFold(name, "insidePoly") && len(n.Children) == 0 {
			return fmt.Errorf("attribute %q: no <zone> child", name)
		}
	}
	return nil
}

// resolveConditionPair reads an "a,b" value: the whole value first, then
// each of its first two comma parts, may name a table. Anything after a
// second comma is kept as written.
func resolveConditionPair(name, val string, resolve tableResolver) (string, error) {
	val, err := templateValue(name, val, resolve)
	if err != nil {
		return "", err
	}
	parts := strings.Split(val, ",")
	if len(parts) < 2 {
		return val, nil
	}
	for i := range parts[:2] {
		if parts[i], err = templateValue(name, parts[i], resolve); err != nil {
			return "", err
		}
	}
	return strings.Join(parts, ","), nil
}

// templateValue resolves a value against the template's tables, or, when
// the template has none, fails a value that names one.
func templateValue(name, val string, resolve tableResolver) (string, error) {
	if resolve == nil {
		return val, noTableRef(name, val)
	}
	return resolve(name, val), nil
}

// noTableRef fails a value that names a table where none can be read.
func noTableRef(name, val string) error {
	if strings.HasPrefix(val, "#") {
		return fmt.Errorf("attribute %q: %w: %q", name, errTableRefNotAllowed, val)
	}
	return nil
}
