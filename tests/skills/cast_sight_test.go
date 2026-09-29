package skills

import (
	"testing"

	"github.com/fatal10110/acis_golang/internal/gameserver/geo/dynamic"
	"github.com/fatal10110/acis_golang/internal/gameserver/model/actor/cast"
	"github.com/fatal10110/acis_golang/internal/gameserver/model/actor/move"
	"github.com/fatal10110/acis_golang/internal/gameserver/model/actor/player"
	modelskill "github.com/fatal10110/acis_golang/internal/gameserver/model/skill"
	"github.com/fatal10110/acis_golang/internal/gameserver/network/serverpackets"
	"github.com/fatal10110/acis_golang/internal/gameservertest"
)

// sightSkillID is a ranged single-target strike, like 1177 Wind Strike.
const sightSkillID = 1177

func sightSkill(tune func(*modelskill.Definition)) modelskill.Definition {
	def := modelskill.Definition{
		ID: sightSkillID, Level: 1, Activation: modelskill.ActivationActive, Target: modelskill.TargetOne,
		CastRange: 600, HitTime: 500, ReuseDelay: 60_000, StaticHitTime: true, StaticReuse: true,
		MPInitialConsume: 2, MPConsume: 3, SkillType: "MDAM", Power: 1, Magic: true,
	}
	if tune != nil {
		tune(&def)
	}
	return def
}

// seeingGeo is passable movement geo whose line-of-sight query always
// succeeds, so the caster's sight check runs and passes.
type seeingGeo struct{ gameservertest.Geo }

func (seeingGeo) CanSeeActor(int, int, int, float64, int, int, int, float64) bool { return true }

// TestPlayerRangedCastNeedsLineOfSight pins CreatureCast.canCast's sight
// check (CreatureCast.java:392-396) on a player's cast: a ranged skill aimed
// at a monster the geodata hides answers CANT_SEE_TARGET alone — no
// ActionFailed, no MagicSkillUse, no MP or reuse charged — while the same
// skill with sight casts. The check follows HP/MP and mute and precedes the
// weapon and <cond> checks, so a cast failing several reads the first only.
func TestPlayerRangedCastNeedsLineOfSight(t *testing.T) {
	t.Parallel()
	hpCond := []modelskill.ConditionClause{{
		// The player is at full HP, so the clause fails.
		Root:      modelskill.Condition{Kind: "player", Attrs: map[string]string{"hp": "25"}},
		MessageID: serverpackets.SystemMessageS1CannotBeUsed, AddName: true,
	}}
	for _, tc := range []struct {
		name    string
		tune    func(*modelskill.Definition)
		blind   bool
		casts   bool
		message int
	}{
		{name: "sight casts", casts: true},
		{name: "no sight", blind: true, message: serverpackets.SystemMessageCantSeeTarget},
		{
			name:    "MP before sight",
			tune:    func(d *modelskill.Definition) { d.MPConsume = 100_000 },
			blind:   true,
			message: serverpackets.SystemMessageNotEnoughMP,
		},
		{
			name:    "sight before weapon",
			tune:    func(d *modelskill.Definition) { d.WeaponsAllowed = "BOW" },
			blind:   true,
			message: serverpackets.SystemMessageCantSeeTarget,
		},
		{
			name:    "sight before skill condition",
			tune:    func(d *modelskill.Definition) { d.Conditions = hpCond },
			blind:   true,
			message: serverpackets.SystemMessageCantSeeTarget,
		},
	} {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			var geo move.Geo = seeingGeo{}
			if tc.blind {
				geo = blindGeo{}
			}
			def := sightSkill(tc.tune)
			srv := gameservertest.Boot(t,
				gameservertest.WithCharacter("Newbie", 5, 0),
				gameservertest.WithWantChars(1),
				gameservertest.WithGeo(geo),
				gameservertest.WithSkills(skillPersistence(t, []modelskill.Definition{def})),
			)
			c, objID := srv.Client, srv.SoleObjectID(t)
			seedKnownSkill(t, srv, objID, sightSkillID, 1)
			startInWorld(t, c)
			hostile := srv.SpawnHostileNPC(t)
			drainUntilQuiet(t, c)
			targetHostile(t, c, hostile.ObjectID())
			drainUntilQuiet(t, c)
			mp := playerMP(t, srv, objID)

			c.Send(encodeRequestMagicSkillUse(sightSkillID, false, false))
			if tc.casts {
				assertCasterMPStatus(t, srv, c.Read(), objID, mp-2)
				readCastStartFrames(t, c, objID, sightSkillID, 1, 500, 60_000, hostile.ObjectID())
				return
			}
			assertStaticSystemMessage(t, c.Read(), tc.message)
			assertNoActionFailedUntilQuiet(t, c, tc.name)
			if srv.PlayerCastingNow(t, objID) {
				t.Fatalf("%s started a cast", tc.name)
			}
			if got := playerMP(t, srv, objID); got != mp {
				t.Fatalf("%s: MP = %d after the refusal, want untouched %d", tc.name, got, mp)
			}
			var disabled bool
			onPlayerQueue(t, srv, objID, func(pc *player.Character) { disabled = pc.SkillDisabled(cast.ReuseKey(def)) })
			if disabled {
				t.Fatalf("%s charged the skill's reuse", tc.name)
			}
		})
	}
}

// doorSightGeo is geodata where a closed door hides itself unless the
// sight query leaves that door out, as GeoEngine.canSeeTarget does for a
// target that is itself a geo object.
type doorSightGeo struct{ gameservertest.Geo }

func (doorSightGeo) CanSeeActor(int, int, int, float64, int, int, int, float64) bool { return false }

func (doorSightGeo) CanSeeActorIgnoring(_, _, _ int, _ float64, _, _, _ int, _ float64, ignore dynamic.Object) bool {
	return ignore != nil
}

// TestRangedCastAtDoorIgnoresTheDoorItself pins that the cast-start sight
// check never lets a closed door block the line to itself: an unlock skill
// aimed at the door casts and opens it.
func TestRangedCastAtDoorIgnoresTheDoorItself(t *testing.T) {
	t.Parallel()
	def := unlockSkill(2235, "UNLOCK_SPECIAL", unlockSkillLevel, 100)
	srv := gameservertest.Boot(t,
		gameservertest.WithCharacter("Newbie", 5, 0),
		gameservertest.WithWantChars(1),
		gameservertest.WithGeo(doorSightGeo{}),
		gameservertest.WithSkills(skillPersistence(t, []modelskill.Definition{def})),
		gameservertest.WithDoors(unlockDoorTemplate(false)),
	)
	c, objID := srv.Client, srv.SoleObjectID(t)
	seedKnownSkill(t, srv, objID, int(def.ID), def.Level)
	c.Send(encodeRequestGameStart(0))
	assertFrameOpcode(t, c.Read(), serverpackets.OpcodeSSQInfo, "SSQInfo")
	assertFrameOpcode(t, c.Read(), serverpackets.OpcodeCharSelected, "CharSelected")
	c.Send(encodeEnterWorld())
	drainUntilQuiet(t, c)
	gate, ok := srv.WorldObjects.Door(unlockDoorID)
	if !ok {
		t.Fatal("door not spawned")
	}
	selectTarget(t, c, gate.ObjectID())

	frames := castUnlock(t, c, objID, def, gate.ObjectID())

	if !gate.Opened() {
		t.Fatal("door still closed after a guaranteed unlock")
	}
	assertNoSystemMessage(t, frames, serverpackets.SystemMessageCantSeeTarget)
}
