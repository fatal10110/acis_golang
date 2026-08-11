package npc

import (
	"errors"

	"github.com/fatal10110/acis_golang/internal/gameserver/model/actor/npcinfo"
	"github.com/fatal10110/acis_golang/internal/gameserver/world"
)

// Decoration is an immobile, non-attackable NPC placed by an item.
type Decoration struct {
	world.Presence
	*Instance
	title string
}

func NewDecoration(inst *Instance, title string) (*Decoration, error) {
	if inst == nil || inst.Template == nil {
		return nil, errors.New("npc: nil decoration instance")
	}
	return &Decoration{Instance: inst, title: title}, nil
}

func (d *Decoration) ObjectID() int32 { return d.Instance.ObjectID }

func (d *Decoration) Name() string { return d.Instance.Template.Name }

func (d *Decoration) CollisionRadius() float64 { return d.Instance.Template.CollisionRadius }

func (d *Decoration) NPCInfoSnapshot() npcinfo.Snapshot {
	t := d.Instance.Template
	x, y, z := d.Position()
	name := ""
	if t.UsingServerSideName {
		name = t.Name
	}
	return npcinfo.Snapshot{
		ObjectID: d.ObjectID(), TemplateID: t.TemplateID,
		X: x, Y: y, Z: z, Heading: d.Heading(),
		MAtkSpd: int(t.AtkSpd), PAtkSpd: int(t.AtkSpd),
		RunSpd: int(t.RunSpeed), WalkSpd: int(t.WalkSpeed),
		CollisionRadius: t.CollisionRadius, CollisionHeight: t.CollisionHeight,
		RightHand: t.RightHand, LeftHand: t.LeftHand,
		SummonAnimation: 2, Name: name, Title: d.title,
	}
}
