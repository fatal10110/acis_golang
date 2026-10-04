package door

import "github.com/fatal10110/acis_golang/internal/gameserver/skill/conditions"

// ConditionActor is o's view for skill condition tests. A door only ever
// sits on the target side, where npcId matches its door id; it never moves,
// knows no skill and carries no effect.
func (o *Object) ConditionActor() conditions.Actor {
	if o == nil {
		return nil
	}
	return doorConditionActor{o: o}
}

type doorConditionActor struct{ o *Object }

var _ conditions.Actor = doorConditionActor{}

func (d doorConditionActor) DoorID() int { return d.o.DoorID() }

func (d doorConditionActor) Level() int { return d.o.Template.Level }

func (d doorConditionActor) HPRatio() float64 {
	if d.o.MaxHP() <= 0 {
		return 0
	}
	return d.o.HPRatio()
}

func (doorConditionActor) MPRatio() float64 { return 0 }

func (d doorConditionActor) X() int { x, _, _ := d.o.Position(); return x }
func (d doorConditionActor) Y() int { _, y, _ := d.o.Position(); return y }
func (d doorConditionActor) Z() int { _, _, z := d.o.Position(); return z }

func (doorConditionActor) IsMoving() bool                    { return false }
func (doorConditionActor) IsRunning() bool                   { return false }
func (doorConditionActor) IsRiding() bool                    { return false }
func (doorConditionActor) IsFlying() bool                    { return false }
func (doorConditionActor) CurrentHeading() int               { return 0 }
func (doorConditionActor) IsBehind(conditions.Actor) bool    { return false }
func (doorConditionActor) IsInFrontOf(conditions.Actor) bool { return false }
func (doorConditionActor) ActiveSkillLevel(int) (int, bool)  { return 0, false }
func (doorConditionActor) ActiveEffectLevel(int) (int, bool) { return 0, false }
func (doorConditionActor) IsNight() bool                     { return false }
