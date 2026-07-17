package conditions

import (
	"github.com/fatal10110/acis_golang/internal/gameserver/model/actor/player"
	"github.com/fatal10110/acis_golang/internal/gameserver/model/zone"
)

// Level requires the effector to be at least the given level.
type Level struct{ Level int }

func (c Level) Test(effector, effected, skill any) bool {
	return effector.(Actor).Level() >= c.Level
}

// Hp requires the effector's current HP to be at or below the given
// percentage (0-100).
type Hp struct{ Percent int }

func (c Hp) Test(effector, effected, skill any) bool {
	return effector.(Actor).HPRatio()*100 <= float64(c.Percent)
}

// Mp requires the effector's current MP to be at or below the given
// percentage (0-100).
type Mp struct{ Percent int }

func (c Mp) Test(effector, effected, skill any) bool {
	return effector.(Actor).MPRatio()*100 <= float64(c.Percent)
}

// PkCount requires the effector to be a player with at most the given
// number of PK kills.
type PkCount struct{ Max int }

func (c PkCount) Test(effector, effected, skill any) bool {
	p, ok := asPlayer(effector)
	return ok && p.PkKills() <= c.Max
}

// HasCastle requires the effector to be a player whose clan owns a
// specific castle (CastleID > 0), any castle (CastleID == -1), or no
// castle at all (CastleID == 0, the default when the player has no clan).
type HasCastle struct{ CastleID int }

func (c HasCastle) Test(effector, effected, skill any) bool {
	p, ok := asPlayer(effector)
	if !ok {
		return false
	}
	if !p.HasClan() {
		return c.CastleID == 0
	}
	if c.CastleID == -1 {
		return p.ClanHasAnyCastle()
	}
	return p.ClanCastleID() == c.CastleID
}

// HasClanHall requires the effector to be a player whose clan owns one of
// the listed clan halls, any clan hall (ClanHallIDs == [-1]), or no clan
// hall at all (ClanHallIDs == [0], the default when the player has no
// clan).
type HasClanHall struct{ ClanHallIDs []int }

func (c HasClanHall) Test(effector, effected, skill any) bool {
	p, ok := asPlayer(effector)
	if !ok {
		return false
	}
	if !p.HasClan() {
		return len(c.ClanHallIDs) == 1 && c.ClanHallIDs[0] == 0
	}
	if len(c.ClanHallIDs) == 1 && c.ClanHallIDs[0] == -1 {
		return p.ClanHasAnyClanHall()
	}
	id := p.ClanHallID()
	for _, want := range c.ClanHallIDs {
		if want == id {
			return true
		}
	}
	return false
}

// InvSize requires a player's inventory to have room for at least Size more
// slots. A non-player effector always passes.
type InvSize struct{ Size int }

func (c InvSize) Test(effector, effected, skill any) bool {
	p, ok := asPlayer(effector)
	if !ok {
		return true
	}
	return p.InventorySize() <= p.InventoryLimit()-c.Size
}

// IsHero requires the effector to be a player whose hero status matches
// Want.
type IsHero struct{ Want bool }

func (c IsHero) Test(effector, effected, skill any) bool {
	p, ok := asPlayer(effector)
	return ok && p.IsHero() == c.Want
}

// PledgeClass requires the effector to be a clan member: either the clan
// leader (Class == -1), or at least the given pledge class.
type PledgeClass struct{ Class int }

func (c PledgeClass) Test(effector, effected, skill any) bool {
	p, ok := asPlayer(effector)
	if !ok || !p.HasClan() {
		return false
	}
	if c.Class == -1 {
		return p.IsClanLeader()
	}
	return p.PledgeClass() >= c.Class
}

// Race requires the effector to be a player of the given race.
type Race struct{ Race player.Race }

func (c Race) Test(effector, effected, skill any) bool {
	p, ok := asPlayer(effector)
	return ok && p.Race() == c.Race
}

// Sex requires the effector to be a player whose sex ordinal matches Sex
// (0 male, 1 female).
type Sex struct{ Sex int }

func (c Sex) Test(effector, effected, skill any) bool {
	p, ok := asPlayer(effector)
	return ok && int(p.Sex()) == c.Sex
}

// State names one of the effector-state checks State gates on.
type State uint8

const (
	StateResting State = iota
	StateMoving
	StateRunning
	StateRiding
	StateFlying
	StateBehind
	StateFront
	StateOlympiad
)

// PlayerState requires the effector's named State to match Required. Every
// case but Moving/Riding/Flying/Behind/Front is player-only; a non-player
// effector fails Resting/Olympiad by reporting the opposite of Required
// instead of panicking.
type PlayerState struct {
	Check    State
	Required bool
}

func (c PlayerState) Test(effector, effected, skill any) bool {
	a := effector.(Actor)
	switch c.Check {
	case StateResting:
		if p, ok := asPlayer(effector); ok {
			return p.IsSitting() == c.Required
		}
		return !c.Required
	case StateMoving:
		return a.IsMoving() == c.Required
	case StateRunning:
		return a.IsMoving() == c.Required && a.IsRunning() == c.Required
	case StateRiding:
		return a.IsRiding() == c.Required
	case StateFlying:
		return a.IsFlying() == c.Required
	case StateBehind:
		return a.IsBehind(effected.(Actor)) == c.Required
	case StateFront:
		return a.IsInFrontOf(effected.(Actor)) == c.Required
	case StateOlympiad:
		if p, ok := asPlayer(effector); ok {
			return p.IsInOlympiadMode() == c.Required
		}
		return !c.Required
	}
	return !c.Required
}

// Weight requires a player's current weight-penalty tier to be below the
// given tier. A non-player effector always passes.
type Weight struct{ Tier int }

func (c Weight) Test(effector, effected, skill any) bool {
	p, ok := asPlayer(effector)
	if !ok {
		return true
	}
	return p.WeightPenalty() < c.Tier
}

// Charges requires a player to have at least the given number of charges.
type Charges struct{ Min int }

func (c Charges) Test(effector, effected, skill any) bool {
	p, ok := asPlayer(effector)
	return ok && p.Charges() >= c.Min
}

// ActiveEffectID requires the effector to currently have an active effect
// of the given id, at or above Level (Level == -1 matches any level).
type ActiveEffectID struct {
	EffectID int
	Level    int
}

func (c ActiveEffectID) Test(effector, effected, skill any) bool {
	level, ok := effector.(Actor).ActiveEffectLevel(c.EffectID)
	return ok && c.Level <= level
}

// ActiveSkillID requires the effector to currently know a skill of the
// given id, at or above Level (Level == -1 matches any level).
type ActiveSkillID struct {
	SkillID int
	Level   int
}

func (c ActiveSkillID) Test(effector, effected, skill any) bool {
	level, ok := effector.(Actor).ActiveSkillLevel(c.SkillID)
	return ok && c.Level <= level
}

// InsidePoly requires the effector's position to be inside (or, if
// CheckInside is false, outside) the given zone form.
type InsidePoly struct {
	Zone        zone.Form
	CheckInside bool
}

func (c InsidePoly) Test(effector, effected, skill any) bool {
	a := effector.(Actor)
	inside := c.Zone.Contains(a.X(), a.Y(), a.Z())
	if c.CheckInside {
		return inside
	}
	return !inside
}
