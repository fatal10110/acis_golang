package item

import (
	"fmt"
	"strconv"
	"strings"
)

// SkillRef is an (id, level) pair naming one skill without resolving it:
// looking the id up in the skill table is that table's job, not this
// package's.
type SkillRef struct {
	ID    int32
	Level int32
}

// ParseSkillRef parses s, formatted "id-level", into a SkillRef.
func ParseSkillRef(s string) (SkillRef, error) {
	id, level, ok := strings.Cut(s, "-")
	if !ok {
		return SkillRef{}, fmt.Errorf("item: skill reference %q: want \"id-level\"", s)
	}
	idNum, err := strconv.ParseInt(id, 10, 32)
	if err != nil {
		return SkillRef{}, fmt.Errorf("item: skill reference %q: %w", s, err)
	}
	levelNum, err := strconv.ParseInt(level, 10, 32)
	if err != nil {
		return SkillRef{}, fmt.Errorf("item: skill reference %q: %w", s, err)
	}
	return SkillRef{ID: int32(idNum), Level: int32(levelNum)}, nil
}

// ParseSkillRefs parses s, a ";"-separated list of "id-level" pairs, into a
// slice of SkillRef. An empty s returns a nil slice with no error.
func ParseSkillRefs(s string) ([]SkillRef, error) {
	if s == "" {
		return nil, nil
	}
	parts := strings.Split(s, ";")
	refs := make([]SkillRef, len(parts))
	for i, p := range parts {
		ref, err := ParseSkillRef(p)
		if err != nil {
			return nil, err
		}
		refs[i] = ref
	}
	return refs, nil
}

// SkillTrigger is a skill a weapon casts under some condition (on critical
// hit, on spell cast, on reaching enchant +4), together with the percentage
// chance it triggers. Chance is -1 when the template attaches the skill
// unconditionally (every qualifying hit/cast triggers it).
type SkillTrigger struct {
	Skill  SkillRef
	Chance int32
}
