package npc

import (
	"github.com/fatal10110/acis_golang/internal/gameserver/model/actor/npcinfo"
	"github.com/fatal10110/acis_golang/internal/gameserver/world"
)

// Polymorph returns the NPC whose look the NPC is shown with, nil while it
// is shown as itself.
func (inst *Instance) Polymorph() *Template { return inst.form.Load() }

// showForm shows s in the NPC's form, when it has one: the form's template
// id, held items and body. Everything else stays the NPC's own.
func (inst *Instance) showForm(s *npcinfo.Snapshot) {
	form := inst.form.Load()
	if form == nil {
		return
	}
	s.TemplateID = form.TemplateID
	s.RightHand, s.LeftHand = form.RightHand, form.LeftHand
	s.CollisionRadius, s.CollisionHeight = form.CollisionRadius, form.CollisionHeight
}

// reshow takes t off w's grid and puts it back where it stands: the players
// around it forget it, then see it again as it now is. A despawned t stays
// out of the world.
func reshow(w *world.State, t world.Tracked) {
	if w == nil {
		return
	}
	w.Leave(t)
	w.Rejoin(t)
}

// Polymorph returns the NPC whose look the NPC is shown with, nil while it
// is shown as itself.
func (h *Hostile) Polymorph() *Template { return h.Instance.Polymorph() }

// Morph shows the NPC as form, or as itself again for nil: the players
// around it forget it, then see it again so.
func (h *Hostile) Morph(form *Template) {
	h.Instance.form.Store(form)
	reshow(h.world, h)
}

// Polymorph returns the NPC whose look the NPC is shown with, nil while it
// is shown as itself.
func (f *Folk) Polymorph() *Template { return f.Instance.Polymorph() }

// Morph shows the NPC as form, or as itself again for nil, as
// Hostile.Morph does. A control tower in a form works; one shown as itself
// stands dead.
func (f *Folk) Morph(form *Template) {
	f.Instance.form.Store(form)
	reshow(f.world, f)
}

// Morph shows the decoration as form, or as itself again for nil, as
// Hostile.Morph does.
func (d *Decoration) Morph(form *Template) {
	d.Instance.form.Store(form)
	reshow(d.world, d)
}

// Polymorph returns the NPC whose look the point is shown with, nil while
// it is shown as itself.
func (ep *EffectPoint) Polymorph() *Template { return ep.Instance.Polymorph() }

// Morph shows the point as form, or as itself again for nil, as
// Hostile.Morph does.
func (ep *EffectPoint) Morph(form *Template) {
	ep.Instance.form.Store(form)
	reshow(ep.world, ep)
}
