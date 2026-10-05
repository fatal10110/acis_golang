package npc

import (
	"errors"
	"fmt"
	"sync"

	"github.com/fatal10110/acis_golang/internal/gameserver/model/actor"
	"github.com/fatal10110/acis_golang/internal/gameserver/model/actor/event"
	"github.com/fatal10110/acis_golang/internal/gameserver/model/actor/npcinfo"
	"github.com/fatal10110/acis_golang/internal/gameserver/model/location"
	"github.com/fatal10110/acis_golang/internal/gameserver/model/zone"
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

	// world, sink and zones are installed by Attach, before the NPC is
	// published.
	world *world.State
	sink  event.Sink
	zones *zoneMember
	// placeMu serializes Spawn and Despawn: the player placing the
	// decoration and a GM deleting it run on different queues, and a
	// deletion landing between the spawn and its zone entry would leave
	// the removed decoration in its zones.
	placeMu sync.Mutex
}

// DecorationRuntime is what a placed decoration lives in: the world it
// stands in, the zones its membership follows (nil for none) and the sink
// its observers are told of its changes through (nil tells nobody).
type DecorationRuntime struct {
	World *world.State
	Zones *zone.Index
	Sink  event.Sink
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
	d := &Decoration{Instance: inst, stats: stats}
	d.zones = newZoneMember(d)
	return d, nil
}

// Attach installs rt. Call it once, before Spawn.
func (d *Decoration) Attach(rt DecorationRuntime) {
	d.world, d.sink = rt.World, rt.Sink
	d.zones.ix = rt.Zones
}

// Spawn places the decoration in the world at (x, y, z) facing heading,
// then enters the zones at its position. It is a no-op until Attach has
// installed a world.
func (d *Decoration) Spawn(x, y, z, heading int) {
	if d.world == nil {
		return
	}
	d.placeMu.Lock()
	defer d.placeMu.Unlock()
	d.world.Spawn(d, x, y, z, heading)
	d.zones.enter()
}

// Despawn takes the decoration out of its zones, while its observers still
// know it, then out of the world. It is a no-op until Attach has installed
// a world.
func (d *Decoration) Despawn() {
	if d.world == nil {
		return
	}
	d.placeMu.Lock()
	defer d.placeMu.Unlock()
	x, y, z := d.Position()
	d.zones.leave(location.Location{X: x, Y: y, Z: z})
	d.world.Despawn(d)
}

// InsideZone reports whether the decoration's zones hold flag.
func (d *Decoration) InsideZone(flag zone.Flag) bool { return d.zones.has(flag) }

// zoneTeleporting reports false: a decoration never moves.
func (d *Decoration) zoneTeleporting() bool { return false }

// swimStateChanged shows the decoration's observers its view again as it
// enters or leaves water: the stationary one when it cannot move at its
// speed, the full one otherwise (WaterZone.onEnter and onExit).
func (d *Decoration) swimStateChanged(bool) {
	if d.sink != nil {
		d.sink.Emit(event.NPCInfoChanged{ServerObject: d.stats.moveSpeed == 0})
	}
}

func (d *Decoration) ObjectID() int32 { return d.Instance.ObjectID }

func (d *Decoration) CollisionRadius() float64 { return d.Instance.Template.CollisionRadius }

func (d *Decoration) NPCInfoSnapshot() npcinfo.Snapshot {
	return fixedNPCInfoSnapshot(d.Instance, d.stats, &d.Presence, d.zones.moveType())
}

// ServerObjectInfoSnapshot is NPCInfoSnapshot with the server-side name
// always shown, the view an NPC that cannot move is announced with.
func (d *Decoration) ServerObjectInfoSnapshot() npcinfo.Snapshot {
	s := d.NPCInfoSnapshot()
	s.Name = d.Instance.Name()
	return s
}

// fixedNPCInfoSnapshot is the client view of an NPC with fixed stats,
// standing where p is and moving by moveType.
func fixedNPCInfoSnapshot(inst *Instance, stats fixedStats, p *world.Presence, moveType int) npcinfo.Snapshot {
	t := inst.Template
	x, y, z := p.Position()
	name := ""
	if t.UsingServerSideName {
		name = inst.Name()
	}
	return npcinfo.Snapshot{
		ObjectID: inst.ObjectID, TemplateID: t.TemplateID,
		X: x, Y: y, Z: z, Heading: p.Heading(),
		MAtkSpd: stats.mAtkSpd, PAtkSpd: stats.pAtkSpd,
		RunSpd: int(t.RunSpeed), WalkSpd: int(t.WalkSpeed),
		MoveMultiplier: stats.moveMultiplier, AtkSpdMultiplier: stats.atkSpdMultiplier,
		MoveType:        moveType,
		CollisionRadius: t.CollisionRadius, CollisionHeight: t.CollisionHeight,
		RightHand: t.RightHand, LeftHand: t.LeftHand,
		Running: !inst.WalkMode, SummonAnimation: 2, Name: name, Title: inst.Title(),
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
