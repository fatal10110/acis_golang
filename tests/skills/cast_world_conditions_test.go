package skills

import (
	"strconv"
	"testing"

	modelskill "github.com/fatal10110/acis_golang/internal/gameserver/model/skill"
	"github.com/fatal10110/acis_golang/internal/gameserver/network/serverpackets"
	"github.com/fatal10110/acis_golang/internal/gameservertest"
)

// worldGatedSkill is a free self-cast carrying one cast condition.
func worldGatedSkill(id modelskill.ID, root modelskill.Condition) modelskill.Definition {
	return modelskill.Definition{
		ID: id, Level: 1, Activation: modelskill.ActivationActive, Target: modelskill.TargetSelf,
		StaticHitTime: true, SkillType: "BUFF",
		Conditions: []modelskill.ConditionClause{{
			Root: root, MessageID: serverpackets.SystemMessageS1CannotBeUsed, AddName: true,
		}},
	}
}

// squareZone is a <player insidePoly> condition over a square of the given
// half-width around (centerX, centerY), tall enough to hold any spawn height.
func squareZone(inside bool, half, centerX, centerY int) modelskill.Condition {
	node := func(x, y int) modelskill.Condition {
		return modelskill.Condition{Kind: "node", Attrs: map[string]string{"x": strconv.Itoa(x), "y": strconv.Itoa(y)}}
	}
	return modelskill.Condition{
		Kind: "player", Attrs: map[string]string{"insidePoly": strconv.FormatBool(inside)},
		Children: []modelskill.Condition{{
			Kind: "zone", Attrs: map[string]string{"minZ": "-100000", "maxZ": "100000"},
			Children: []modelskill.Condition{
				node(centerX-half, centerY-half), node(centerX+half, centerY-half),
				node(centerX+half, centerY+half), node(centerX-half, centerY+half),
			},
		}},
	}
}

// TestCastConditionsReadServerNightAndPlayerZone casts skills gated by
// <game night="True|False"/> and <player insidePoly="True|False"> against the
// server's day/night source and the player's position: a gate opens only
// when the world matches it, in both clock states.
func TestCastConditionsReadServerNightAndPlayerZone(t *testing.T) {
	t.Parallel()
	const (
		nightOnly    = 3411
		dayOnly      = 3412
		inZone       = 3413
		outsideZone  = 3414
		inFarZone    = 3415
		outsideFar   = 3416
		farCenter    = 90_000
		wideHalfSize = 50_000
	)
	night := func(v bool) modelskill.Condition {
		return modelskill.Condition{Kind: "game", Attrs: map[string]string{"night": strconv.FormatBool(v)}}
	}
	defs := []modelskill.Definition{
		worldGatedSkill(nightOnly, night(true)),
		worldGatedSkill(dayOnly, night(false)),
		worldGatedSkill(inZone, squareZone(true, wideHalfSize, 0, 0)),
		worldGatedSkill(outsideZone, squareZone(false, wideHalfSize, 0, 0)),
		worldGatedSkill(inFarZone, squareZone(true, 1000, farCenter, farCenter)),
		worldGatedSkill(outsideFar, squareZone(false, 1000, farCenter, farCenter)),
	}
	for _, isNight := range []bool{true, false} {
		t.Run("night="+strconv.FormatBool(isNight), func(t *testing.T) {
			t.Parallel()
			srv := gameservertest.Boot(t,
				gameservertest.WithCharacter("Newbie", 5, 0),
				gameservertest.WithWantChars(1),
				gameservertest.WithNightSource(fixedNight(isNight)),
				gameservertest.WithSkills(skillPersistence(t, defs)),
			)
			c, objID := srv.Client, srv.SoleObjectID(t)
			for _, d := range defs {
				seedKnownSkill(t, srv, objID, int(d.ID), 1)
			}
			startInWorld(t, c)
			drainUntilQuiet(t, c)

			for _, tc := range []struct {
				name string
				id   int32
				want bool
			}{
				{"night gate", nightOnly, isNight},
				{"day gate", dayOnly, !isNight},
				{"inside the wide zone", inZone, true},
				{"outside the wide zone", outsideZone, false},
				{"inside a far zone", inFarZone, false},
				{"outside a far zone", outsideFar, true},
			} {
				if got := castStarts(t, c, tc.id); got != tc.want {
					t.Errorf("%s: cast started = %v, want %v", tc.name, got, tc.want)
				}
			}
		})
	}
}
