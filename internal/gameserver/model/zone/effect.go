package zone

import (
	"fmt"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/fatal10110/acis_golang/internal/commons"
)

// TargetScope narrows which occupant families an effect zone works on.
type TargetScope uint8

// Effect target scopes, from narrowest to widest.
const (
	// ScopePlayer targets player characters only.
	ScopePlayer TargetScope = iota
	// ScopeSummon targets player-owned summons and pets only.
	ScopeSummon
	// ScopePlayable targets players and their summons.
	ScopePlayable
	// ScopeNPC targets world NPCs and monsters only.
	ScopeNPC
	// ScopeCreature targets every occupant.
	ScopeCreature
)

// ParseTargetScope maps the data files' target type names onto TargetScope.
func ParseTargetScope(s string) (TargetScope, error) {
	switch s {
	case "Player":
		return ScopePlayer, nil
	case "Summon":
		return ScopeSummon, nil
	case "Playable":
		return ScopePlayable, nil
	case "Npc":
		return ScopeNPC, nil
	case "Creature":
		return ScopeCreature, nil
	default:
		return 0, fmt.Errorf("zone: unknown target scope %q", s)
	}
}

// Matches reports whether an occupant of class c falls under the scope.
func (s TargetScope) Matches(c Class) bool {
	switch s {
	case ScopePlayer:
		return c == ClassPlayer
	case ScopeSummon:
		return c == ClassSummon
	case ScopePlayable:
		return c.Playable()
	case ScopeNPC:
		return c == ClassNPC
	case ScopeCreature:
		return true
	default:
		return false
	}
}

// SkillRef names one skill at one level.
type SkillRef struct {
	ID    int
	Level int
}

// Effect is a zone that periodically applies skill effects to the
// occupants of its target scope, and marks players as being in danger.
type Effect struct {
	Zone
	// Skills lists the effects applied by each pulse.
	Skills []SkillRef
	// Chance is the percent chance, rolled per occupant per pulse, that
	// the skills land.
	Chance int
	// InitialDelay and ReuseDelay time the effect pulse.
	InitialDelay time.Duration
	ReuseDelay   time.Duration
	// Target picks which occupant families the pulse touches.
	Target TargetScope

	// StartPulse begins the periodic effect task; nil until the skill
	// system wires it. The zone fires it at most once until PulseStopped
	// resets the latch.
	StartPulse func()
	// DangerNotice refreshes a player's danger status display; nil until
	// the messaging layer wires it.
	DangerNotice func(a Actor)

	// mu guards enabled and pulsing.
	mu      sync.Mutex
	enabled bool
	pulsing bool
}

// NewEffect builds an effect zone from its data settings.
func NewEffect(id int, form Form, set *commons.StatSet) (*Effect, error) {
	f := commons.NewFields(set, "zone: effect")
	chance := f.IntDefault("chance", 100)
	initial := f.IntDefault("initialDelay", 0)
	reuse := f.IntDefault("reuseDelay", 30000)
	enabled := f.BoolDefault("defaultStatus", true)
	skills, err := parseSkillRefs(f.StringDefault("skill", ""))
	if err != nil {
		f.Fail(err)
	}
	target := ScopePlayer
	if raw := f.StringDefault("targetType", ""); raw != "" {
		if t, err := ParseTargetScope(raw); err != nil {
			f.Fail(err)
		} else {
			target = t
		}
	}
	if err := f.Err(); err != nil {
		return nil, err
	}
	return &Effect{
		Zone:         newZone(id, form),
		Skills:       skills,
		Chance:       chance,
		InitialDelay: time.Duration(initial) * time.Millisecond,
		ReuseDelay:   time.Duration(reuse) * time.Millisecond,
		Target:       target,
		enabled:      enabled,
	}, nil
}

// parseSkillRefs parses the data files' skill list syntax: semicolon
// separated "id-level" pairs.
func parseSkillRefs(raw string) ([]SkillRef, error) {
	if raw == "" {
		return nil, nil
	}
	parts := strings.Split(raw, ";")
	refs := make([]SkillRef, 0, len(parts))
	for _, part := range parts {
		idLevel := strings.Split(part, "-")
		if len(idLevel) != 2 {
			return nil, fmt.Errorf("zone: malformed skill reference %q", part)
		}
		id, err := strconv.Atoi(idLevel[0])
		if err != nil {
			return nil, fmt.Errorf("zone: malformed skill reference %q: %w", part, err)
		}
		level, err := strconv.Atoi(idLevel[1])
		if err != nil {
			return nil, fmt.Errorf("zone: malformed skill reference %q: %w", part, err)
		}
		refs = append(refs, SkillRef{ID: id, Level: level})
	}
	return refs, nil
}

// Core exposes the shared zone state.
func (z *Effect) Core() *Zone { return &z.Zone }

func (z *Effect) affects(a Actor) bool { return z.Target.Matches(a.Class()) }

func (z *Effect) enter(a Actor) {
	z.mu.Lock()
	if !z.pulsing {
		z.pulsing = true
		if z.StartPulse != nil {
			z.StartPulse()
		}
	}
	z.mu.Unlock()
	if a.Class() == ClassPlayer {
		a.ZoneFlags().Set(FlagDanger, true)
		if z.DangerNotice != nil {
			z.DangerNotice(a)
		}
	}
}

func (z *Effect) exit(a Actor) {
	if a.Class() == ClassPlayer {
		a.ZoneFlags().Set(FlagDanger, false)
		// Refresh the display only once the last overlapping danger zone
		// released its hold.
		if !a.ZoneFlags().Has(FlagDanger) && z.DangerNotice != nil {
			z.DangerNotice(a)
		}
	}
}

// Enabled reports whether the pulse currently applies its effects.
func (z *Effect) Enabled() bool {
	z.mu.Lock()
	defer z.mu.Unlock()
	return z.enabled
}

// SetEnabled toggles whether the pulse applies its effects.
func (z *Effect) SetEnabled(v bool) {
	z.mu.Lock()
	z.enabled = v
	z.mu.Unlock()
}

// PulseStopped resets the pulse latch; the effect task calls it when it
// shuts itself down, so the next entry can start a fresh pulse.
func (z *Effect) PulseStopped() {
	z.mu.Lock()
	z.pulsing = false
	z.mu.Unlock()
}
