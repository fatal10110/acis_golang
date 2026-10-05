package npc

import (
	"fmt"
	"strings"

	"github.com/fatal10110/acis_golang/internal/commons"
	"github.com/fatal10110/acis_golang/internal/gameserver/model/skill"
)

// AIParams are the name/value pairs of one <ai> block, kept as text. A
// template and a spawn entry each carry one; neither is written after
// load. The zero value is an empty set.
type AIParams map[string]string

// Int reads name as a decimal int32: an optional sign, then digits, in
// range. It returns def when name is absent and an error when the value is
// present but does not read as one.
func (p AIParams) Int(name string, def int32) (int32, error) {
	v, ok := p[name]
	if !ok {
		return def, nil
	}
	n, err := commons.ParseInt(v, 32)
	if err != nil {
		return 0, fmt.Errorf("npc: AI parameter %q: %w", name, err)
	}
	return int32(n), nil
}

// String returns the value of name, def when it is absent. A present empty
// value is returned as it is.
func (p AIParams) String(name, def string) string {
	if v, ok := p[name]; ok {
		return v
	}
	return def
}

// resolveAIInt reads name from spawn first and tmpl second, falling back
// to def. The template value is parsed even when spawn shadows it, and a
// value that does not read as an int, in either set, gives def.
func resolveAIInt(spawn, tmpl AIParams, name string, def int32) int32 {
	fallback, err := tmpl.Int(name, def)
	if err != nil {
		return def
	}
	v, err := spawn.Int(name, fallback)
	if err != nil {
		return def
	}
	return v
}

// resolveAIString reads name from spawn first and tmpl second, falling
// back to def.
func resolveAIString(spawn, tmpl AIParams, name, def string) string {
	return spawn.String(name, tmpl.String(name, def))
}

// resolveAISkill reads name, resolved as resolveAIString does, as an
// "id-level" skill reference. ok is false when neither set holds name. A
// value with fewer than two '-'-separated fields, or a field that is not
// an int, is an error. Fields past the second are ignored. The reference
// is not checked against the skill table.
func resolveAISkill(spawn, tmpl AIParams, name string) (ref skill.Ref, ok bool, err error) {
	v, found := spawn[name]
	if !found {
		v, found = tmpl[name]
	}
	if !found {
		return skill.Ref{}, false, nil
	}
	fields := strings.Split(v, "-")
	// Trailing empty fields do not count: "1-" has one field.
	for len(fields) > 0 && fields[len(fields)-1] == "" {
		fields = fields[:len(fields)-1]
	}
	if len(fields) < 2 {
		return skill.Ref{}, false, fmt.Errorf("npc: AI skill parameter %q: %q is not id-level", name, v)
	}
	id, err := commons.ParseInt(fields[0], 32)
	if err != nil {
		return skill.Ref{}, false, fmt.Errorf("npc: AI skill parameter %q: %w", name, err)
	}
	level, err := commons.ParseInt(fields[1], 32)
	if err != nil {
		return skill.Ref{}, false, fmt.Errorf("npc: AI skill parameter %q: %w", name, err)
	}
	return skill.Ref{ID: skill.ID(id), Level: int(level)}, true, nil
}
