package npc

import (
	"errors"
	"fmt"

	"github.com/fatal10110/acis_golang/internal/gameserver/model/actor"
	"github.com/fatal10110/acis_golang/internal/gameserver/model/actor/npcinfo"
	"github.com/fatal10110/acis_golang/internal/gameserver/skill/effect"
	"github.com/fatal10110/acis_golang/internal/gameserver/skill/stat"
	"github.com/fatal10110/acis_golang/internal/gameserver/world"
)

// Decoration is an immobile, non-attackable NPC placed by an item. It holds
// no effects, so its stats are fixed at spawn.
type Decoration struct {
	world.Presence
	*Instance
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
	inst.SetTitle(title)
	return &Decoration{Instance: inst, stats: stats}, nil
}

func (d *Decoration) ObjectID() int32 { return d.Instance.ObjectID }

func (d *Decoration) CollisionRadius() float64 { return d.Instance.Template.CollisionRadius }

func (d *Decoration) NPCInfoSnapshot() npcinfo.Snapshot {
	t := d.Instance.Template
	x, y, z := d.Position()
	name := ""
	if t.UsingServerSideName {
		name = d.Instance.Name()
	}
	return npcinfo.Snapshot{
		ObjectID: d.ObjectID(), TemplateID: t.TemplateID,
		X: x, Y: y, Z: z, Heading: d.Heading(),
		MAtkSpd: d.stats.mAtkSpd, PAtkSpd: d.stats.pAtkSpd,
		RunSpd: int(t.RunSpeed), WalkSpd: int(t.WalkSpeed),
		MoveMultiplier: d.stats.moveMultiplier, AtkSpdMultiplier: d.stats.atkSpdMultiplier,
		CollisionRadius: t.CollisionRadius, CollisionHeight: t.CollisionHeight,
		RightHand: t.RightHand, LeftHand: t.LeftHand,
		Running: !d.Instance.WalkMode, SummonAnimation: 2, Name: name, Title: d.Instance.Title(),
	}
}

// Kind reports KindNPC.
func (d *Decoration) Kind() actor.Kind { return actor.KindNPC }

// fixedStats are the client-visible stats of an NPC that holds no effects,
// settled once at spawn from its template and passive skills.
type fixedStats struct {
	maxHP, pAtkSpd, mAtkSpd          int
	moveMultiplier, atkSpdMultiplier float64
	// moveSpeed is the speed the NPC walks or runs at, by its stance.
	moveSpeed float64
}

// settleFixedStats finalizes inst's stats through the builtin stat funcs
// and the template passives lookup resolves.
func settleFixedStats(inst *Instance, lookup skillDefinitions) (fixedStats, error) {
	mods, err := effect.TemplatePassiveMods(lookup, inst.Template.Passives)
	if err != nil {
		return fixedStats{}, fmt.Errorf("npc %d template passives: %w", inst.Template.ID, err)
	}
	t := inst.Template
	calc := func(s stat.Stat, base float64) float64 {
		c := effect.NewCalculator(defaultBuiltin(s))
		for _, m := range mods {
			if m.Stat == s {
				c.AddMod(m)
			}
		}
		v := c.Calc(templateStatActor{t: t}, base)
		if s.CantBeNegative() && v <= 0 {
			return 1
		}
		return v
	}
	fs := fixedStats{
		maxHP:   int(calc(stat.MaxHP, t.HPMax)),
		pAtkSpd: int(calc(stat.PowerAttackSpeed, t.AtkSpd)),
		mAtkSpd: int(calc(stat.MagicAttackSpeed, magicAttackSpeedBase)),
	}
	fs.atkSpdMultiplier = npcinfo.AttackSpeedMultiplier(fs.pAtkSpd, t.AtkSpd)
	// The move speed over the base speed the stance picks, as a moving NPC
	// computes it; 0 for an NPC whose base is 0.
	base := int(t.RunSpeed)
	if inst.WalkMode {
		base = int(t.WalkSpeed)
	}
	if base != 0 {
		speed := float32(calc(stat.RunSpeed, float64(base)))
		fs.moveMultiplier = float64(speed / float32(base))
		fs.moveSpeed = float64(speed)
	}
	return fs, nil
}
