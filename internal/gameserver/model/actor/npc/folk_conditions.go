package npc

import (
	"github.com/fatal10110/acis_golang/internal/gameserver/model/location"
	"github.com/fatal10110/acis_golang/internal/gameserver/skill/conditions"
	"github.com/fatal10110/acis_golang/internal/gameserver/skill/stat"
)

// folkStatActor is a civilian NPC as the effector of its own stat funcs and
// as either side of a condition test: its template attributes for the
// finalize funcs, its live position, facing, movement, HP, MP and effects
// for the conditions. It is never a conditions.PlayerActor, so a
// player-only condition fails on it.
type folkStatActor struct {
	templateStatActor
	f *Folk
}

var (
	_ stat.Actor       = folkStatActor{}
	_ conditions.Actor = folkStatActor{}
)

// ConditionActor is f's view for skill and item condition tests.
func (f *Folk) ConditionActor() conditions.Actor {
	if f == nil {
		return nil
	}
	return f.statActor()
}

func (f *Folk) statActor() folkStatActor {
	return folkStatActor{templateStatActor: templateStatActor{t: f.Instance.Template}, f: f}
}

func (a folkStatActor) HPRatio() float64 {
	maxHP := a.f.MaxHPValue()
	if maxHP <= 0 {
		return 0
	}
	return a.f.HP() / maxHP
}

func (a folkStatActor) MPRatio() float64 {
	maxMP := a.f.MaxMPValue()
	if maxMP <= 0 {
		return 0
	}
	return a.f.MPValue() / maxMP
}

func (a folkStatActor) X() int { return a.f.X() }
func (a folkStatActor) Y() int { return a.f.Y() }
func (a folkStatActor) Z() int { return a.f.Z() }

func (a folkStatActor) IsMoving() bool  { return a.f.IsMoving() }
func (a folkStatActor) IsRunning() bool { return a.f.Running() }

// IsRiding and IsFlying report false: an NPC is never mounted, and no
// civilian NPC flies.
func (folkStatActor) IsRiding() bool { return false }
func (folkStatActor) IsFlying() bool { return false }

func (a folkStatActor) CurrentHeading() int { return a.f.Heading() }

// IsBehind reports whether a stands behind other, by other's facing.
func (a folkStatActor) IsBehind(other conditions.Actor) bool {
	return facingOf(other).IsBehind(location.Location{X: a.X(), Y: a.Y(), Z: a.Z()})
}

// IsInFrontOf reports whether a stands in front of other, by other's
// facing.
func (a folkStatActor) IsInFrontOf(other conditions.Actor) bool {
	return facingOf(other).IsInFrontOf(location.Location{X: a.X(), Y: a.Y(), Z: a.Z()})
}

func facingOf(other conditions.Actor) location.OrientedLocation {
	return location.OrientedLocation{
		Location: location.Location{X: other.X(), Y: other.Y(), Z: other.Z()},
		Heading:  other.CurrentHeading(),
	}
}

// ActiveSkillLevel reads the template skills; passives and race markers
// are not among them, and an active effect of the skill does not count.
func (a folkStatActor) ActiveSkillLevel(id int) (int, bool) {
	level, ok := a.f.Instance.Template.Skills[id]
	return level, ok
}

// NpcID is the template id a target npcId condition matches.
func (a folkStatActor) NpcID() int { return a.f.NpcID() }

// RaceOrdinal is the template race a target race_id condition matches.
func (a folkStatActor) RaceOrdinal() int { return int(a.f.Instance.Template.Race) }

func (a folkStatActor) ActiveEffectLevel(effectID int) (int, bool) {
	return a.f.effects.ActiveBySkillID(effectID)
}

func (a folkStatActor) IsNight() bool { return a.f.effects.IsNight() }
