package npc

import (
	"errors"
	"fmt"
	"sync"
	"sync/atomic"
	"time"

	"github.com/fatal10110/acis_golang/internal/gameserver/model/actor"
	"github.com/fatal10110/acis_golang/internal/gameserver/model/actor/attackable"
	"github.com/fatal10110/acis_golang/internal/gameserver/model/actor/creature"
	"github.com/fatal10110/acis_golang/internal/gameserver/model/actor/event"
	"github.com/fatal10110/acis_golang/internal/gameserver/model/actor/npcinfo"
	modelskill "github.com/fatal10110/acis_golang/internal/gameserver/model/skill"
	"github.com/fatal10110/acis_golang/internal/gameserver/model/zone"
	"github.com/fatal10110/acis_golang/internal/gameserver/sim"
	"github.com/fatal10110/acis_golang/internal/gameserver/skill/effect"
	"github.com/fatal10110/acis_golang/internal/gameserver/skill/stat"
	"github.com/fatal10110/acis_golang/internal/gameserver/task"
	"github.com/fatal10110/acis_golang/internal/gameserver/world"
)

// ChristmasTree is the kind of the decoration a Christmas tree item places
// and an administrator spawns. A click on one is answered ActionFailed and
// never selects it.
const ChristmasTree InstanceKind = "ChristmasTree"

// Decoration is an immobile, non-attackable NPC placed by an item. It holds
// no effects, so its stats are fixed at spawn. Only an administrator's
// //kill takes its HP; it then dies once and its corpse decays.
type Decoration struct {
	world.Presence
	*Instance
	stats fixedStats
	// RespawnDelay is the respawn delay its spawn declares. It is set
	// before Spawn and schedules nothing: a decoration never respawns, not
	// even when its corpse decays.
	RespawnDelay time.Duration

	// world, sink, zones, queue and decay are installed by Attach, before
	// the NPC is published.
	world *world.State
	sink  event.Sink
	zones *zoneMember
	// queue runs the decoration's HP changes, death, revive and decay.
	queue *sim.Queue
	// decay removes its corpse at the template corpse time; nil leaves a
	// corpse in place.
	decay *task.Decay
	// regenSettings answers the base regeneration multipliers in force;
	// nil takes the shipped ones.
	regenSettings RegenSettings
	// regen is the HP regeneration phase SettleRegen arms when its HP drops
	// below max.
	regen creature.Regen

	// vitalsMu guards hp, dead and removed: they change on queue, and
	// observers read them from their own queues.
	vitalsMu sync.Mutex
	hp       float64
	dead     bool
	// removed is set once the decoration has left the world for good: its
	// corpse decayed or it was deleted.
	removed bool
	// running is its run stance, which Spawn sets under placeMu; observers
	// read it from their own queues.
	running atomic.Bool
	// placeMu serializes Spawn and Despawn: the player placing the
	// decoration and a GM deleting it run on different queues, and a
	// deletion landing between the spawn and its zone entry would leave
	// the removed decoration in its zones.
	placeMu sync.Mutex
}

// DecorationRuntime is what a placed decoration lives in: the world it
// stands in, the zones its membership follows (nil for none), the sink its
// observers are told of its changes through (nil tells nobody), the queue
// its work runs on, the decay task its corpse registers with (nil for none)
// and the base regeneration multipliers in force (nil for the shipped ones).
type DecorationRuntime struct {
	World *world.State
	Zones *zone.Index
	Sink  event.Sink
	Queue *sim.Queue
	Decay *task.Decay
	Regen RegenSettings
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
	d := &Decoration{Instance: inst, stats: stats, hp: float64(stats.maxHP)}
	d.zones = newZoneMember(d)
	return d, nil
}

// Attach installs rt. Call it once, before Spawn.
func (d *Decoration) Attach(rt DecorationRuntime) {
	d.world, d.sink = rt.World, rt.Sink
	d.zones.ix = rt.Zones
	d.queue, d.decay = rt.Queue, rt.Decay
	d.regenSettings = rt.Regen
}

// Spawn places the decoration in the world at (x, y, z) facing heading,
// walking, so the players who see it appear are shown it so, then enters the
// zones at its position and takes its run stance. It is a no-op until Attach
// has installed a world.
func (d *Decoration) Spawn(x, y, z, heading int) {
	if d.world == nil {
		return
	}
	d.placeMu.Lock()
	defer d.placeMu.Unlock()
	d.running.Store(false)
	d.world.Spawn(d, x, y, z, heading)
	d.zones.enter()
	takeRunStance(&d.running, d.stats, d.sink)
}

// takeRunStance puts a walking fixed-stats NPC in its run stance and shows
// its observers, through sink, the change: ChangeMoveType when it can move
// at its run speed, then its info again.
func takeRunStance(running *atomic.Bool, stats fixedStats, sink event.Sink) {
	if !running.CompareAndSwap(false, true) || sink == nil {
		return
	}
	if stats.moveSpeed != 0 {
		sink.Emit(event.MoveTypeChanged{Running: true})
	}
	sink.Emit(event.NPCInfoChanged{})
}

// Despawn deletes the decoration, dead or alive: its corpse, if any, no
// longer decays, and it leaves its zones, while its observers still know
// it, then the world, and its queue takes no more work. It runs once, and
// is a no-op until Attach has installed a world.
func (d *Decoration) Despawn() { d.remove() }

// Decay removes the decoration's corpse as Despawn does and reports whether
// this call removed it. The respawn hook is never run: a decoration never
// respawns.
func (d *Decoration) Decay(*world.State, func()) bool { return d.remove() }

// remove is Despawn, reporting whether this call ran it.
func (d *Decoration) remove() bool {
	if d.world == nil {
		return false
	}
	d.vitalsMu.Lock()
	if d.removed {
		d.vitalsMu.Unlock()
		return false
	}
	d.removed = true
	d.vitalsMu.Unlock()
	if d.decay != nil {
		d.decay.Cancel(d)
	}
	d.placeMu.Lock()
	d.zones.leave()
	d.world.Despawn(d)
	d.placeMu.Unlock()
	d.SettleRegen()
	if d.queue != nil {
		d.queue.Close()
	}
	return true
}

// Queue returns the queue the decoration's work runs on.
func (d *Decoration) Queue() *sim.Queue { return d.queue }

// HP returns the current HP.
func (d *Decoration) HP() float64 {
	d.vitalsMu.Lock()
	defer d.vitalsMu.Unlock()
	return d.hp
}

// MaxHPValue returns the maximum HP in whole points.
func (d *Decoration) MaxHPValue() float64 { return float64(d.stats.maxHP) }

// Dead reports whether the decoration has died. A corpse stays dead once it
// decays.
func (d *Decoration) Dead() bool {
	d.vitalsMu.Lock()
	defer d.vitalsMu.Unlock()
	return d.dead
}

// ReduceHP takes amount from the decoration's HP, never under zero for a
// template that is not undying nor under folkMinHP for one that is, and
// kills it once its HP falls under folkDeathHP; one left alive below its
// maximum starts regenerating. A dead or removed decoration, or a hit from an
// attacker without damage permission, takes nothing. Nobody can select a
// decoration, so no player watches its health bar and the loss shows nobody.
func (d *Decoration) ReduceHP(amount float64, attacker attackable.Combatant, _ modelskill.Definition) {
	if !creature.CanDealDamage(attacker) {
		return
	}
	floor := 0.0
	if d.Instance.Template.Undying {
		floor = folkMinHP
	}
	d.vitalsMu.Lock()
	if d.dead || d.removed {
		d.vitalsMu.Unlock()
		return
	}
	// A NaN amount takes nothing, like a negative one.
	if amount > 0 {
		d.hp = max(d.hp-amount, floor)
	}
	if d.hp >= folkDeathHP {
		d.vitalsMu.Unlock()
		d.SettleRegen()
		return
	}
	d.hp, d.dead = 0, true
	d.vitalsMu.Unlock()
	d.SettleRegen()
	d.die()
}

// die shows the decoration's observers it died, then registers its corpse
// for decay after the template corpse time. A decoration deleted meanwhile
// leaves no decay behind.
func (d *Decoration) die() {
	if d.sink != nil {
		d.sink.Emit(event.Died{})
	}
	if d.decay == nil {
		return
	}
	d.decay.Add(d, time.Duration(d.Instance.Template.CorpseTime)*time.Second)
	d.vitalsMu.Lock()
	removed := d.removed
	d.vitalsMu.Unlock()
	if removed {
		d.decay.Cancel(d)
	}
}

// SetMaxHpMp fills a living decoration's HP to its maximum, which ends its
// regeneration; its MP is always full. Nobody watches its health bar, so the
// fill shows nobody.
func (d *Decoration) SetMaxHpMp() {
	d.vitalsMu.Lock()
	if !d.dead && !d.removed {
		d.hp = float64(d.stats.maxHP)
	}
	d.vitalsMu.Unlock()
	d.SettleRegen()
}

// Revive stands this dead decoration up again with restoreHP of its
// maximum HP, which starts its regeneration when short of the maximum, and
// shows its observers it rise; nobody watches its health bar, so the HP
// itself shows nobody. A living decoration, or one that has
// left the world, is left as it is, and Revive reports false. The caller
// drops the corpse's pending decay first.
func (d *Decoration) Revive(restoreHP float64) bool {
	maxHP := d.MaxHPValue()
	d.vitalsMu.Lock()
	if !d.dead || d.removed {
		d.vitalsMu.Unlock()
		return false
	}
	d.dead = false
	d.hp = min(maxHP*restoreHP, maxHP)
	d.vitalsMu.Unlock()
	d.SettleRegen()
	if d.sink != nil {
		d.sink.Emit(event.Revived{})
	}
	return true
}

// InsideZone reports whether the decoration's zones hold flag.
func (d *Decoration) InsideZone(flag zone.Flag) bool { return d.zones.has(flag) }

// zoneTeleporting reports false: a decoration never moves.
func (d *Decoration) zoneTeleporting() bool { return false }

// swimStateChanged shows the decoration's observers its view again as it
// enters or leaves water: the stationary one when it cannot move at its
// speed, the full one otherwise.
func (d *Decoration) swimStateChanged(bool) {
	if d.sink != nil {
		d.sink.Emit(event.NPCInfoChanged{ServerObject: d.stats.moveSpeed == 0})
	}
}

func (d *Decoration) ObjectID() int32 { return d.Instance.ObjectID }

func (d *Decoration) CollisionRadius() float64 { return d.Instance.Template.CollisionRadius }

// NPCInfoSnapshot captures the decoration's client view, in its form when it
// has one.
func (d *Decoration) NPCInfoSnapshot() npcinfo.Snapshot {
	s := d.infoSnapshot()
	d.Instance.showForm(&s)
	return s
}

// infoSnapshot is NPCInfoSnapshot shown as the decoration itself, whatever
// its form.
func (d *Decoration) infoSnapshot() npcinfo.Snapshot {
	s := fixedNPCInfoSnapshot(d.Instance, d.stats, &d.Presence, d.zones.moveType(), d.running.Load())
	s.AlikeDead = d.Dead()
	return s
}

// ServerObjectInfoSnapshot is NPCInfoSnapshot with the server-side name
// always shown, the view an NPC that cannot move is announced with. It
// shows the decoration attackable, with its HP: like any civilian NPC it
// may be attacked by force, though it can never be targeted. It never
// shows a form.
func (d *Decoration) ServerObjectInfoSnapshot() npcinfo.Snapshot {
	s := d.infoSnapshot()
	s.Name = d.Instance.Name()
	s.Attackable = true
	s.CurrentHP, s.MaxHP = int(d.HP()), d.stats.maxHP
	return s
}

// fixedNPCInfoSnapshot is the client view of an NPC with fixed stats,
// standing where p is, moving by moveType, in its run stance when running
// and its walk stance otherwise.
func fixedNPCInfoSnapshot(inst *Instance, stats fixedStats, p *world.Presence, moveType int, running bool) npcinfo.Snapshot {
	t := inst.Template
	moveMultiplier := stats.walkMoveMultiplier
	if running {
		moveMultiplier = stats.moveMultiplier
	}
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
		MoveMultiplier: moveMultiplier, AtkSpdMultiplier: stats.atkSpdMultiplier,
		MoveType:        moveType,
		CollisionRadius: t.CollisionRadius, CollisionHeight: t.CollisionHeight,
		RightHand: t.RightHand, LeftHand: t.LeftHand,
		Running: running, SummonAnimation: 2, Name: name, Title: inst.Title(),
	}
}

// Kind reports KindNPC.
func (d *Decoration) Kind() actor.Kind { return actor.KindNPC }

// fixedStats are the client-visible stats of an NPC that holds no effects,
// settled once at spawn from its template and passive skills.
type fixedStats struct {
	maxHP, pAtkSpd, mAtkSpd          int
	moveMultiplier, atkSpdMultiplier float64
	// walkMoveMultiplier is moveMultiplier in the walk stance.
	walkMoveMultiplier float64
	// moveSpeed is the speed the NPC runs at.
	moveSpeed float64
	// template and mods are what every other stat is finalized from: the
	// NPC's template and its passives' stat funcs.
	template *Template
	mods     []effect.Mod
}

// settleFixedStats finalizes inst's stats through the builtin stat funcs
// and the template passives lookup resolves.
func settleFixedStats(inst *Instance, lookup skillDefinitions) (fixedStats, error) {
	mods, err := effect.TemplatePassiveMods(lookup, inst.Template.Passives)
	if err != nil {
		return fixedStats{}, fmt.Errorf("npc %d template passives: %w", inst.Template.ID, err)
	}
	t := inst.Template
	// A decoration has no condition view: a func with an attached condition
	// never attaches.
	fs := fixedStats{template: t, mods: effect.Attachable(mods, nil)}
	fs.maxHP = int(fs.calc(stat.MaxHP, t.HPMax))
	fs.pAtkSpd = int(fs.calc(stat.PowerAttackSpeed, t.AtkSpd))
	fs.mAtkSpd = int(fs.calc(stat.MagicAttackSpeed, magicAttackSpeedBase))
	fs.atkSpdMultiplier = npcinfo.AttackSpeedMultiplier(fs.pAtkSpd, t.AtkSpd)
	// The move speed over the stance's base speed, as a moving NPC computes
	// it; 0 for a stance whose base is 0.
	fs.moveSpeed, fs.moveMultiplier = fs.stanceSpeed(int(t.RunSpeed))
	_, fs.walkMoveMultiplier = fs.stanceSpeed(int(t.WalkSpeed))
	return fs, nil
}

// stanceSpeed is the move speed over the stance base speed base, narrowed
// to float32 like the client-facing speed, and that speed over base; both
// are 0 when base is 0.
func (fs fixedStats) stanceSpeed(base int) (speed, multiplier float64) {
	if base == 0 {
		return 0, 0
	}
	s := float32(fs.calc(stat.RunSpeed, float64(base)))
	return float64(s), float64(s / float32(base))
}

// calc finalizes base for s through the builtin stat funcs and the
// passives, flooring a non-positive stat that can't go negative at one.
func (fs fixedStats) calc(s stat.Stat, base float64) float64 {
	c := effect.NewCalculator(defaultBuiltin(s))
	for _, m := range fs.mods {
		if m.Stat == s {
			c.AddMod(m)
		}
	}
	v := c.Calc(templateStatActor{t: fs.template}, base, nil, nil)
	if s.CantBeNegative() && v <= 0 {
		return 1
	}
	return v
}
