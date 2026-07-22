package skill

import (
	"reflect"
	"strings"

	modelskill "github.com/fatal10110/acis_golang/internal/gameserver/model/skill"
)

// Cast carries the already-resolved inputs a skill handler needs.
type Cast struct {
	Caster  any
	Skill   modelskill.Definition
	Targets []any
	Item    any
}

// Handler applies one skill action to already-resolved targets.
type Handler interface {
	Types() []string
	Use(Cast)
}

// Result reports caster-visible outcomes produced while a skill handler ran.
type Result struct {
	AttackFailed int
}

type resultHandler interface {
	UseResult(Cast) Result
}

// Registry maps skill type names to their handlers.
type Registry struct {
	entries map[string]Handler
}

// NewRegistry returns a registry populated with handlers.
func NewRegistry(handlers ...Handler) *Registry {
	r := &Registry{entries: make(map[string]Handler)}
	for _, h := range handlers {
		r.Register(h)
	}
	return r
}

// NewDefaultRegistry returns the representative handlers that currently have
// enough surrounding model support to run deterministically.
func NewDefaultRegistry() *Registry {
	return NewRegistry(
		pdamHandler{},
		mdamHandler{},
		blowHandler{},
		manaDamageHandler{},
		healHandler{},
		healPercentHandler{},
		manaHealHandler{},
		combatPointHealHandler{},
		cpDamagePercentHandler{},
		balanceLifeHandler{},
		realDamageHandler{},
		giveSPHandler{},
		dummyHandler{},
		cancelHandler{},
		disablersHandler{},
		resurrectHandler{},
		instantJumpHandler{},
		getPlayerHandler{},
		summonCreatureHandler{},
		summonFriendHandler{},
		unlockHandler{},
		extractableHandler{},
		sowHandler{},
		harvestHandler{},
		spoilHandler{},
		sweepHandler{},
		continuousHandler{},
	)
}

// Register adds h for every skill type it reports.
func (r *Registry) Register(h Handler) {
	if r == nil || h == nil {
		return
	}
	if r.entries == nil {
		r.entries = make(map[string]Handler)
	}
	for _, skillType := range h.Types() {
		key := skillTypeKey(skillType)
		if key != "" {
			r.entries[key] = h
		}
	}
}

// Handler returns the handler for skillType.
func (r *Registry) Handler(skillType string) (Handler, bool) {
	if r == nil {
		return nil, false
	}
	h, ok := r.entries[skillTypeKey(skillType)]
	return h, ok
}

// Use dispatches cast to the handler registered for cast.Skill.SkillType.
func (r *Registry) Use(cast Cast) bool {
	_, ok := r.UseResult(cast)
	return ok
}

// UseResult dispatches cast and returns any caster-visible handler result.
func (r *Registry) UseResult(cast Cast) (Result, bool) {
	h, ok := r.Handler(cast.Skill.SkillType)
	if !ok {
		return Result{}, false
	}
	if rh, ok := h.(resultHandler); ok {
		return rh.UseResult(cast), true
	}
	h.Use(cast)
	return Result{}, true
}

func skillTypeKey(skillType string) string {
	return strings.ToUpper(strings.TrimSpace(skillType))
}

type dummyHandler struct{}

func (dummyHandler) Types() []string { return []string{"DUMMY", "BEAST_FEED"} }

func (dummyHandler) Use(Cast) {}

func alikeDead(v any) bool {
	if d, ok := v.(interface{ AlikeDead() bool }); ok {
		return d.AlikeDead()
	}
	if d, ok := v.(interface{ Dead() bool }); ok {
		return d.Dead()
	}
	return false
}

func sameObject(a, b any) bool {
	if a == nil || b == nil {
		return a == b
	}

	ta := reflect.TypeOf(a)
	tb := reflect.TypeOf(b)
	if ta != tb || !ta.Comparable() {
		return false
	}

	return a == b
}

func cursed(v any) bool {
	c, ok := v.(interface{ CursedWeaponEquipped() bool })
	return ok && c.CursedWeaponEquipped()
}
