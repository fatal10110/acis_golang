package npc

import (
	"errors"

	"github.com/fatal10110/acis_golang/internal/gameserver/model/actor"
	"github.com/fatal10110/acis_golang/internal/gameserver/model/actor/npcinfo"
	"github.com/fatal10110/acis_golang/internal/gameserver/world"
)

// Decoration is an immobile, non-attackable NPC placed by an item. Like a
// civilian NPC it holds no effects, so its stats are fixed at spawn.
type Decoration struct {
	world.Presence
	*Instance
	title string
	stats fixedStats
}

// NewDecoration builds an item-placed NPC from inst, titled with title.
// skills, when provided, resolves the template's passive skills into the
// stats it is shown with.
func NewDecoration(inst *Instance, title string, skills ...skillDefinitions) (*Decoration, error) {
	if inst == nil || inst.Template == nil {
		return nil, errors.New("npc: nil decoration instance")
	}
	var lookup skillDefinitions
	if len(skills) > 0 {
		lookup = skills[0]
	}
	stats, err := settleFixedStats(inst, lookup)
	if err != nil {
		return nil, err
	}
	return &Decoration{Instance: inst, title: title, stats: stats}, nil
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
		MAtkSpd: d.stats.mAtkSpd, PAtkSpd: d.stats.pAtkSpd,
		RunSpd: int(t.RunSpeed), WalkSpd: int(t.WalkSpeed),
		MoveMultiplier: d.stats.moveMultiplier, AtkSpdMultiplier: d.stats.atkSpdMultiplier,
		CollisionRadius: t.CollisionRadius, CollisionHeight: t.CollisionHeight,
		RightHand: t.RightHand, LeftHand: t.LeftHand,
		Running: !d.Instance.WalkMode, SummonAnimation: 2, Name: name, Title: d.title,
	}
}

// Kind reports KindNPC.
func (d *Decoration) Kind() actor.Kind { return actor.KindNPC }
