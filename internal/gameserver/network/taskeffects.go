package network

import (
	"sync"
	"time"

	"github.com/fatal10110/acis_golang/internal/commons/wire"
	"github.com/fatal10110/acis_golang/internal/gameserver/model/actor/player"
	"github.com/fatal10110/acis_golang/internal/gameserver/model/item"
	"github.com/fatal10110/acis_golang/internal/gameserver/model/location"
	modelskill "github.com/fatal10110/acis_golang/internal/gameserver/model/skill"
	"github.com/fatal10110/acis_golang/internal/gameserver/model/zone"
	"github.com/fatal10110/acis_golang/internal/gameserver/network/serverpackets"
	"github.com/fatal10110/acis_golang/internal/gameserver/skill/stat"
	"github.com/fatal10110/acis_golang/internal/gameserver/task"
	"github.com/fatal10110/acis_golang/internal/gameserver/world"
)

var _ task.WaterEffects = (*TaskEffects)(nil)
var _ task.ShadowItemEffects = (*TaskEffects)(nil)

// liveZoneActor adapts a live player to the zone package without changing
// Character's world.Position method signature.
type liveZoneActor struct {
	mu    sync.Mutex
	live  *livePlayer
	flags zone.Flags
}

func (a *liveZoneActor) ObjectID() int32             { return a.live.ObjectID() }
func (a *liveZoneActor) Position() location.Location { return a.live.CurrentLocation() }
func (a *liveZoneActor) ZoneFlags() *zone.Flags      { return &a.flags }
func (a *liveZoneActor) Class() zone.Class           { return zone.ClassPlayer }
func (a *liveZoneActor) GM() bool                    { return a.live.AccessLevel > 0 }
func (a *liveZoneActor) Online() bool                { return a.live.Visible() }
func (a *liveZoneActor) Race() player.Race           { return a.live.Character.Race }
func (a *liveZoneActor) ClanID() int32               { return int32(a.live.Character.ClanID) }

func (a *liveZoneActor) revalidate(ix *zone.Index) {
	a.mu.Lock()
	defer a.mu.Unlock()
	ix.Revalidate(a)
}

func (a *liveZoneActor) revalidateMove(ix *zone.Index, previous location.Location) {
	a.mu.Lock()
	defer a.mu.Unlock()
	ix.RevalidateMove(a, previous)
}

func (a *liveZoneActor) removeFrom(ix *zone.Index, x, y int) {
	a.mu.Lock()
	defer a.mu.Unlock()
	ix.RemoveFrom(a, x, y)
}

// TaskEffects routes periodic task effects to their current live player.
type TaskEffects struct {
	state *world.State

	mu     sync.RWMutex
	expire func(*livePlayer, *item.Instance)
}

func NewTaskEffects(state *world.State) *TaskEffects {
	return &TaskEffects{state: state}
}

// SetShadowItemExpiry connects expiry to the live inventory owner.
func (e *TaskEffects) SetShadowItemExpiry(expire func(*livePlayer, *item.Instance)) {
	e.mu.Lock()
	defer e.mu.Unlock()
	e.expire = expire
}

func (e *TaskEffects) GaugeSet(actor task.WaterActor, remaining time.Duration) {
	if actor == nil || e.state == nil {
		return
	}
	obj, ok := e.state.Player(actor.ObjectID())
	if !ok {
		return
	}
	live, ok := obj.(*livePlayer)
	if !ok {
		return
	}
	ms := int(remaining.Milliseconds())
	live.SendFrame(serverpackets.FrameSetupGauge(serverpackets.GaugeCyan, ms, ms))
}

func (e *TaskEffects) Drown(actor task.WaterActor) {
	if actor == nil || e.state == nil {
		return
	}
	obj, ok := e.state.Player(actor.ObjectID())
	if !ok {
		return
	}
	live, ok := obj.(*livePlayer)
	if !ok || live.Dead() {
		return
	}
	coefficient := 0.001724
	if player.ClassMage(live.ClassID) {
		coefficient = 0.002698
	}
	damage := live.MaxHPValue() * live.Race.BreathMultiplier() * coefficient
	live.ReduceHP(damage, live, modelskill.Definition{})
	live.BroadcastStatus()
	live.SendFrame(serverpackets.FrameSystemMessageNumber(serverpackets.SystemMessageDrownDamage, int32(damage)))
}

func (e *TaskEffects) ManaThreshold(actorID int32, inst *item.Instance, secondsLeft int) {
	if inst == nil {
		return
	}
	message := serverpackets.SystemMessageRemainingMana10Minutes
	switch secondsLeft {
	case 300:
		message = serverpackets.SystemMessageRemainingMana5Minutes
	case 60:
		message = serverpackets.SystemMessageRemainingMana1Minute
	case 600:
	default:
		return
	}
	e.deliver(actorID, serverpackets.FrameSystemMessageItemName(message, inst.TemplateID))
}

func (e *TaskEffects) Expire(actorID int32, inst *item.Instance) {
	if inst == nil || e.state == nil {
		return
	}
	obj, ok := e.state.Player(actorID)
	if !ok {
		return
	}
	live, ok := obj.(*livePlayer)
	if !ok {
		return
	}
	live.shadowExpiryMu.RLock()
	defer live.shadowExpiryMu.RUnlock()
	if live.detaching {
		return
	}
	if current, ok := e.state.Player(actorID); !ok || current != live {
		return
	}
	e.mu.RLock()
	expire := e.expire
	e.mu.RUnlock()
	if expire != nil {
		expire(live, inst)
	}
}

func (l *GameClientLink) wireWaterZones() {
	if l.zones == nil || l.water == nil {
		return
	}
	for _, kind := range l.zones.All() {
		water, ok := kind.(*zone.Water)
		if !ok {
			continue
		}
		water.SwimStateChanged = func(actor zone.Actor, swimming bool) {
			if actor.Class() != zone.ClassPlayer || l.world == nil {
				return
			}
			obj, ok := l.world.Player(actor.ObjectID())
			if !ok {
				return
			}
			live, ok := obj.(*livePlayer)
			if !ok {
				return
			}
			if swimming {
				breath := time.Duration(live.CalcStat(stat.Breath, float64(time.Minute)*live.Race.BreathMultiplier()))
				l.water.Add(live, breath)
				return
			}
			l.water.Remove(live)
		}
	}
}

func (l *GameClientLink) revalidateZones(live *livePlayer, previous location.Location) {
	if l.zones != nil && live != nil && live.zoneActor != nil {
		live.zoneActor.revalidateMove(l.zones, previous)
	}
}

func (e *TaskEffects) deliver(actorID int32, frame wire.Frame) {
	if e.state == nil {
		return
	}
	obj, ok := e.state.Player(actorID)
	if !ok {
		return
	}
	if live, ok := obj.(*livePlayer); ok {
		live.SendFrame(frame)
	}
}
