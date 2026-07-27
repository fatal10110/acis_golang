package player

import (
	"testing"

	"github.com/fatal10110/acis_golang/internal/gameserver/model/actor/creature"
	"github.com/fatal10110/acis_golang/internal/gameserver/model/location"
	"github.com/fatal10110/acis_golang/internal/gameserver/skill/effect"
)

// permissiveGeo is a test-only move.Geo that permits every move, needed
// only because creature.NewLive requires a non-nil Geo.
type permissiveGeo struct{}

func (permissiveGeo) CanMove(ox, oy, oz, tx, ty, tz int) bool { return true }
func (permissiveGeo) Height(x, y, z int) int16                { return int16(z) }
func (permissiveGeo) FindPath(origin, target location.Location) ([]location.Location, bool) {
	return nil, false
}
func (permissiveGeo) ValidLocation(ox, oy, oz, tx, ty, tz int) location.Location {
	return location.Location{X: tx, Y: ty, Z: tz}
}

func withEffectList(t *testing.T, c *Character) *Character {
	t.Helper()
	live, err := creature.NewLive(location.Location{}, 100, permissiveGeo{}, c)
	if err != nil {
		t.Fatal(err)
	}
	c.Live = live
	return c
}

func TestCharacterSeedPowerReadsActiveSeedEffectLevel(t *testing.T) {
	c := withEffectList(t, liveCharacter(1, combatTemplate(), combatItems()))

	if got := c.SeedPower(1285); got != 0 {
		t.Fatalf("SeedPower(1285) uncharged = %d, want 0", got)
	}

	c.EffectList().Add(&effect.Effect{Skill: effect.Skill{ID: 1285}, Level: 4, Type: effect.TypeBuff})

	if got := c.SeedPower(1285); got != 4 {
		t.Fatalf("SeedPower(1285) charged = %d, want 4", got)
	}
}

func TestCharacterForceLevelReadsActiveForceEffectLevel(t *testing.T) {
	c := withEffectList(t, liveCharacter(1, combatTemplate(), combatItems()))

	if level, ok := c.ForceLevel(5104); ok || level != 0 {
		t.Fatalf("ForceLevel(5104) inactive = (%d, %v), want (0, false)", level, ok)
	}

	c.EffectList().Add(&effect.Effect{Skill: effect.Skill{ID: 5104}, Level: 2, Type: effect.TypeBuff})

	if level, ok := c.ForceLevel(5104); !ok || level != 2 {
		t.Fatalf("ForceLevel(5104) active = (%d, %v), want (2, true)", level, ok)
	}
}
