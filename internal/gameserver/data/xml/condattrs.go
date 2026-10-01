package xml

import (
	"errors"
	"fmt"
	"strings"
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
	case "and", "or", "not":
		return condRolePredicate
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
	return vals, nil
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
