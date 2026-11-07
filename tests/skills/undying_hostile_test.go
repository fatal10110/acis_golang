package skills

import (
	"testing"
	"time"

	"github.com/fatal10110/acis_golang/internal/gameserver/model/actor/npc"
	"github.com/fatal10110/acis_golang/internal/gameserver/model/location"
	modelskill "github.com/fatal10110/acis_golang/internal/gameserver/model/skill"
	"github.com/fatal10110/acis_golang/internal/gameserver/skill/effect"
	"github.com/fatal10110/acis_golang/internal/gameservertest"
)

// undyingHostileTemplate is the fixture monster with hp max HP and the
// given undying flag.
func undyingHostileTemplate(undying bool, hp float64) *npc.Template {
	return &npc.Template{
		ID:              100,
		TemplateID:      100,
		Type:            "Monster",
		Level:           1,
		HPMax:           hp,
		AtkSpd:          300,
		RunSpeed:        120,
		WalkSpeed:       60,
		CollisionRadius: 8,
		CollisionHeight: 20,
		Undying:         undying,
	}
}

// undyingHostileAt is the fixture NPC spawn point, in sight of a
// just-entered player.
var undyingHostileAt = location.Location{X: 60, Y: 20, Z: 30}

// assertUndyingOutcome checks a lethal HP loss left an undying monster alive
// at 1 HP and killed a mortal one.
func assertUndyingOutcome(t *testing.T, hostile *npc.Hostile, undying bool) {
	t.Helper()
	if undying {
		if hostile.AlikeDead() || hostile.CurrentHP() != 1 {
			t.Fatalf("undying monster: dead %v, HP %d; want alive at 1 HP", hostile.AlikeDead(), hostile.CurrentHP())
		}
		return
	}
	if !hostile.AlikeDead() || hostile.CurrentHP() != 0 {
		t.Fatalf("mortal monster: dead %v, HP %d; want dead at 0 HP", hostile.AlikeDead(), hostile.CurrentHP())
	}
}

// TestUndyingHostileSurvivesLethalMelee pins the undying floor on a monster
// (Npc constructor setMortal(!isUndying()), CreatureStatus.reduceHp:
// Math.max(hp - value, isMortal() ? 0 : 1), doDie under 0.5): a player's
// melee swings, far past the monster's HP left, leave an undying one alive at
// 1 HP, while the same swings kill a mortal one.
func TestUndyingHostileSurvivesLethalMelee(t *testing.T) {
	for _, undying := range []bool{true, false} {
		t.Run(map[bool]string{true: "undying", false: "mortal"}[undying], func(t *testing.T) {
			t.Parallel()
			srv := gameservertest.Boot(t,
				gameservertest.WithCharacter("Newbie", 5, 0),
				gameservertest.WithWantChars(1),
			)
			c := srv.Client
			startInWorld(t, c)
			hostile := srv.SpawnHostileNPCTemplateAt(t, undyingHostileTemplate(undying, 1000), undyingHostileAt)
			drainUntilQuiet(t, c)
			targetHostile(t, c, hostile.ObjectID())
			drainUntilQuiet(t, c)
			hostile.SetCurrentHP(3)

			c.Send(encodeAttackRequestOn(hostile.ObjectID()))
			srv.AdvanceUntil(t, "a swing landing on the monster", func() bool { return hostile.CurrentHP() < 3 })
			srv.Advance(t, 3*time.Second)
			drainUntilQuiet(t, c)

			assertUndyingOutcome(t, hostile, undying)
		})
	}
}

// TestUndyingHostileSurvivesLethalSkill pins the same floor on skill damage:
// a PDAM far past the monster's HP leaves an undying one at 1 HP and kills
// a mortal one.
func TestUndyingHostileSurvivesLethalSkill(t *testing.T) {
	for _, undying := range []bool{true, false} {
		t.Run(map[bool]string{true: "undying", false: "mortal"}[undying], func(t *testing.T) {
			t.Parallel()
			srv := gameservertest.Boot(t,
				gameservertest.WithCharacter("Newbie", 5, 0),
				gameservertest.WithWantChars(1),
				gameservertest.WithSkills(skillPersistence(t, []modelskill.Definition{
					{
						ID: 42, Level: 1, Activation: modelskill.ActivationActive, Target: modelskill.TargetOne,
						CastRange: 900, HitTime: 500, ReuseDelay: 60_000, StaticHitTime: true, StaticReuse: true,
						SkillType: "PDAM", Power: 1_000_000,
					},
				})),
			)
			c, objID := srv.Client, srv.SoleObjectID(t)
			seedKnownSkill(t, srv, objID, 42, 1)
			startInWorld(t, c)
			hostile := srv.SpawnHostileNPCTemplateAt(t, undyingHostileTemplate(undying, 1000), undyingHostileAt)
			drainUntilQuiet(t, c)
			maxHP := targetHostile(t, c, hostile.ObjectID())
			drainUntilQuiet(t, c)

			c.Send(encodeRequestMagicSkillUse(42, false, false))
			srv.AdvanceUntil(t, "PDAM landing", func() bool { return hostile.CurrentHP() < maxHP })
			drainUntilQuiet(t, c)

			assertUndyingOutcome(t, hostile, undying)
		})
	}
}

// TestUndyingHostileSurvivesLethalHPLoss pins the floor on the HP losses
// that are not a hit: a damage-over-time tick and the monster's own skill
// HP cost both go through the same reduceHp, so neither kills an undying
// monster, and both kill a mortal one.
func TestUndyingHostileSurvivesLethalHPLoss(t *testing.T) {
	cases := []struct {
		name string
		lose func(h *npc.Hostile, attacker effect.Actor)
	}{
		{"dot", func(h *npc.Hostile, attacker effect.Actor) { h.ReduceHPByDOT(5000, attacker, true) }},
		{"hp cost", func(h *npc.Hostile, _ effect.Actor) { h.ConsumeHP(5000) }},
	}
	for _, tc := range cases {
		for _, undying := range []bool{true, false} {
			t.Run(tc.name+"/"+map[bool]string{true: "undying", false: "mortal"}[undying], func(t *testing.T) {
				t.Parallel()
				srv := gameservertest.Boot(t,
					gameservertest.WithCharacter("Newbie", 5, 0),
					gameservertest.WithWantChars(1),
				)
				c, objID := srv.Client, srv.SoleObjectID(t)
				startInWorld(t, c)
				obj, ok := srv.State.Player(objID)
				if !ok {
					t.Fatal("player missing from world")
				}
				hostile := srv.SpawnHostileNPCTemplateAt(t, undyingHostileTemplate(undying, 1000), undyingHostileAt)
				drainUntilQuiet(t, c)

				tc.lose(hostile, obj.(effect.Actor))
				srv.Settle(t)
				drainUntilQuiet(t, c)

				assertUndyingOutcome(t, hostile, undying)
			})
		}
	}
}
