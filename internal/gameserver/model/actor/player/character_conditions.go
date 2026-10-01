package player

import (
	"github.com/fatal10110/acis_golang/internal/gameserver/model/location"
	"github.com/fatal10110/acis_golang/internal/gameserver/skill/conditions"
)

// characterStatActor is the value every stat-calculation call site passes as
// both the effector and (via the *Character also passed as effected) the
// owner of a stat func's gating condition — see calcStat/hostile_stats.go/
// summon/formula.go's identical Calc(actor{...}, owner, nil, base) shape.
// Implementing conditions.Actor/PlayerActor here, rather than on *Character
// directly, avoids a method/field name collision: Character already has
// exported Race and Sex fields, so it cannot also declare Race()/Sex()
// methods.
var (
	_ conditions.Actor       = characterStatActor{}
	_ conditions.PlayerActor = characterStatActor{}
	_ conditions.SeedActor   = characterStatActor{}
	_ conditions.ForceActor  = characterStatActor{}
)

// ConditionActor is c's view for skill and item condition tests.
func (c *Character) ConditionActor() conditions.Actor {
	if c == nil {
		return nil
	}
	return characterStatActor{c: c}
}

// SeedPower satisfies conditions.SeedActor.
func (a characterStatActor) SeedPower(effectID int) int { return a.c.SeedPower(effectID) }

// ForceLevel satisfies conditions.ForceActor.
func (a characterStatActor) ForceLevel(skillID int) (int, bool) { return a.c.ForceLevel(skillID) }

// HPRatio satisfies conditions.Actor.
func (a characterStatActor) HPRatio() float64 {
	max := a.c.MaxHPValue()
	if max <= 0 {
		return 0
	}
	return a.c.HP() / max
}

// MPRatio satisfies conditions.Actor.
func (a characterStatActor) MPRatio() float64 {
	max := a.c.MaxMPValue()
	if max <= 0 {
		return 0
	}
	return a.c.MPValue() / max
}

// CurrentHeading satisfies conditions.Actor.
func (a characterStatActor) CurrentHeading() int { return a.c.CurrentHeading() }

// X satisfies conditions.Actor.
func (a characterStatActor) X() int { return a.c.X() }

// Y satisfies conditions.Actor.
func (a characterStatActor) Y() int { return a.c.Y() }

// Z satisfies conditions.Actor.
func (a characterStatActor) Z() int { return a.c.Z() }

// IsMoving satisfies conditions.Actor.
func (a characterStatActor) IsMoving() bool {
	return a.c.IsMoving()
}

// IsRunning satisfies conditions.Actor: c's run/walk toggle, independent of
// whether it is currently moving.
func (a characterStatActor) IsRunning() bool { return a.c.Running() }

// IsRiding satisfies conditions.Actor.
func (a characterStatActor) IsRiding() bool { return a.c.MountNPCID() != 0 }

// IsFlying satisfies conditions.Actor.
func (a characterStatActor) IsFlying() bool { return a.c.Flying() }

// headingActor is the extra capability IsBehind/IsInFrontOf need from other
// beyond conditions.Actor's plain X/Y/Z: the reference position's own
// heading, which decides which side of it counts as front. Every conditions
// caller this codebase's Calc() call sites can produce is, in practice, this
// same characterStatActor (a stat func's effector and effected both resolve
// to the buff's own owner — see the package doc on effect.Condition), so
// the type assertion below always succeeds today; a target-side facing
// condition (none exist in the shipped datapack) would fail it and read as
// "not behind/in front" rather than panic.
// IsBehind satisfies conditions.Actor: reports whether a is positioned
// behind other, using other's own facing (matching
// creature.ResolveBlowInput's identical behind/front check).
func (a characterStatActor) IsBehind(other conditions.Actor) bool {
	facing := location.OrientedLocation{
		Location: location.Location{X: other.X(), Y: other.Y(), Z: other.Z()},
		Heading:  other.CurrentHeading(),
	}
	return facing.IsBehind(location.Location{X: a.X(), Y: a.Y(), Z: a.Z()})
}

// IsInFrontOf satisfies conditions.Actor: reports whether a is positioned in
// front of other, using other's own facing.
func (a characterStatActor) IsInFrontOf(other conditions.Actor) bool {
	facing := location.OrientedLocation{
		Location: location.Location{X: other.X(), Y: other.Y(), Z: other.Z()},
		Heading:  other.CurrentHeading(),
	}
	return facing.IsInFrontOf(location.Location{X: a.X(), Y: a.Y(), Z: a.Z()})
}

// ActiveSkillLevel satisfies conditions.Actor from c's learned skills; an
// active effect of the skill does not count.
func (a characterStatActor) ActiveSkillLevel(id int) (int, bool) {
	level := a.c.SkillLevel(id)
	return level, level > 0
}

// ActiveEffectLevel satisfies conditions.Actor.
func (a characterStatActor) ActiveEffectLevel(effectID int) (int, bool) {
	if a.c.Live == nil {
		return 0, false
	}
	return a.c.EffectList().ActiveBySkillID(effectID)
}

// IsNight satisfies conditions.Actor.
func (a characterStatActor) IsNight() bool { return a.c.EffectList().IsNight() }

// IsSitting satisfies conditions.PlayerActor.
func (a characterStatActor) IsSitting() bool { return !a.c.Standing() }

// IsInOlympiadMode satisfies conditions.PlayerActor. Always false: Olympiad
// participation isn't modeled on Character yet (#1507). This gates shipped
// skill <cond>s: every olympiad="false" clause passes, which is only right
// while nobody can enter the Olympiad.
func (a characterStatActor) IsInOlympiadMode() bool { return false }

// PkKills satisfies conditions.PlayerActor.
func (a characterStatActor) PkKills() int { return a.c.ProgressionValues().PKKills }

// PledgeClass satisfies conditions.PlayerActor. Always 0: Character.
// PledgeClass now holds the clan rank, but the skill conditions do not read
// it yet (#1507), and no shipped stat func's condition needs it.
func (a characterStatActor) PledgeClass() int { return 0 }

// IsClanLeader satisfies conditions.PlayerActor. Always false: see
// PledgeClass.
func (a characterStatActor) IsClanLeader() bool { return false }

// HasClan satisfies conditions.PlayerActor.
func (a characterStatActor) HasClan() bool { return a.c.ClanID() != 0 }

// ClanCastleID satisfies conditions.PlayerActor. Always 0: castle ownership
// isn't modeled on Character yet (#1507). This gates shipped skill <cond>s:
// Blessed Scroll of Escape: Castle (2178, castle="-1") is always refused
// until a clan can own a castle.
func (a characterStatActor) ClanCastleID() int { return 0 }

// ClanHasAnyCastle satisfies conditions.PlayerActor. Always false, which
// refuses 2178: see ClanCastleID.
func (a characterStatActor) ClanHasAnyCastle() bool { return false }

// ClanHallID satisfies conditions.PlayerActor. Always 0: clan-hall ownership
// isn't modeled on Character yet (#1507). This gates shipped skill <cond>s:
// Blessed Scroll of Escape: Clan Hall (2177, clanHall="-1") is always
// refused until a clan can own a hall.
func (a characterStatActor) ClanHallID() int { return 0 }

// ClanHasAnyClanHall satisfies conditions.PlayerActor. Always false, which
// refuses 2177: see ClanHallID.
func (a characterStatActor) ClanHasAnyClanHall() bool { return false }

// Race satisfies conditions.PlayerActor, returning c.Race's ordinal.
func (a characterStatActor) Race() int { return int(a.c.Race) }

// Sex satisfies conditions.PlayerActor, returning c.Sex's ordinal.
func (a characterStatActor) Sex() int { return int(a.c.Sex) }

// InventorySize satisfies conditions.PlayerActor.
func (a characterStatActor) InventorySize() int {
	if a.c.inventory == nil {
		return 0
	}
	return a.c.inventory.Size()
}

// InventoryLimit satisfies conditions.PlayerActor.
func (a characterStatActor) InventoryLimit() int { return a.c.InventoryLimit() }

// IsHero satisfies conditions.PlayerActor.
func (a characterStatActor) IsHero() bool { return a.c.IsHero() }

// WeightPenalty satisfies conditions.PlayerActor.
func (a characterStatActor) WeightPenalty() int { return a.c.WeightPenalty() }

// Charges satisfies conditions.PlayerActor.
func (a characterStatActor) Charges() int { return a.c.Charges() }

// IsWearingType satisfies conditions.PlayerActor.
func (a characterStatActor) IsWearingType(mask int) bool {
	if a.c.inventory == nil {
		return false
	}
	return a.c.inventory.IsWearingType(int32(mask))
}
