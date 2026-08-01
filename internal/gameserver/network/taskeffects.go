package network

import (
	"time"

	"github.com/fatal10110/acis_golang/internal/commons/wire"
	"github.com/fatal10110/acis_golang/internal/gameserver/model/actor/player"
	"github.com/fatal10110/acis_golang/internal/gameserver/model/item"
	modelskill "github.com/fatal10110/acis_golang/internal/gameserver/model/skill"
	"github.com/fatal10110/acis_golang/internal/gameserver/network/serverpackets"
	"github.com/fatal10110/acis_golang/internal/gameserver/task"
	"github.com/fatal10110/acis_golang/internal/gameserver/world"
)

var _ task.WaterEffects = (*TaskEffects)(nil)
var _ task.ShadowItemEffects = (*TaskEffects)(nil)

// TaskEffects routes periodic task effects to their current live player.
type TaskEffects struct {
	state *world.State
}

func NewTaskEffects(state *world.State, _ any) *TaskEffects {
	return &TaskEffects{state: state}
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
	live.SendFrame(serverpackets.FrameSetupGauge(serverpackets.GaugeCyan, 0, int(remaining.Milliseconds())))
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

func (*TaskEffects) Expire(int32, *item.Instance) {}

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
