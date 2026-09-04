package npc

import (
	"errors"

	skilltarget "github.com/fatal10110/acis_golang/internal/gameserver/handler/target"
	"github.com/fatal10110/acis_golang/internal/gameserver/world"
)

// HolyThing is the stationary, non-combat NPC accepted by HOLY skills.
type HolyThing struct {
	world.Presence
	Instance *Instance
}

// NewHolyThing creates the runtime actor for a HolyThing NPC template.
func NewHolyThing(inst *Instance) (*HolyThing, error) {
	if inst == nil || inst.Template == nil {
		return nil, errors.New("npc: nil holy thing instance")
	}
	if hostileKind(inst) != "HolyThing" {
		return nil, errors.New("npc: instance is not a holy thing")
	}
	return &HolyThing{Instance: inst}, nil
}

func (h *HolyThing) ObjectID() int32 { return h.Instance.ObjectID }
func (h *HolyThing) Dead() bool      { return false }
func (h *HolyThing) Category() skilltarget.Category {
	return skilltarget.CategoryFolk
}
func (h *HolyThing) Holy() bool { return true }

var _ skilltarget.Creature = (*HolyThing)(nil)
var _ skilltarget.HolyTarget = (*HolyThing)(nil)
