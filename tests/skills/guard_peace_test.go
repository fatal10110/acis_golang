package skills

import (
	"testing"

	"github.com/fatal10110/acis_golang/internal/gameserver/model/actor/npc"
	"github.com/fatal10110/acis_golang/internal/gameserver/model/location"
	modelskill "github.com/fatal10110/acis_golang/internal/gameserver/model/skill"
	"github.com/fatal10110/acis_golang/internal/gameserver/model/zone"
	"github.com/fatal10110/acis_golang/internal/gameserver/task"
	"github.com/fatal10110/acis_golang/internal/gameservertest"
)

// TestHelpfulCastOnNPCFlagsCasterUnlessGuard drives a non-offensive
// ONE-target buff from a player onto an attackable NPC. A helpful skill
// landing on an attackable NPC PvP-flags its caster, except when the NPC is a
// town Guard or a SiegeGuard. The flagging control is a FriendlyMonster: an
// attackable that is not a guard and, unlike a Monster, takes a helpful ONE
// skill without CTRL.
func TestHelpfulCastOnNPCFlagsCasterUnlessGuard(t *testing.T) {
	t.Parallel()
	for _, tc := range []struct {
		kind     string
		wantFlag bool
	}{
		{kind: "FriendlyMonster", wantFlag: true},
		{kind: "Guard", wantFlag: false},
		{kind: "SiegeGuard", wantFlag: false},
	} {
		t.Run(tc.kind, func(t *testing.T) {
			t.Parallel()
			const skillID int32 = 1068
			srv := gameservertest.Boot(t,
				gameservertest.WithCharacter("Newbie", 5, 0),
				gameservertest.WithWantChars(1),
				gameservertest.WithPvPFlags(task.NewPvPFlags(task.DefaultPvPFlagOptions(), nil)),
				gameservertest.WithSkills(skillPersistence(t, []modelskill.Definition{{
					ID: modelskill.ID(skillID), Level: 1, Activation: modelskill.ActivationActive,
					Target: modelskill.TargetOne, SkillType: "BUFF",
					CastRange: 900, HitTime: 500, ReuseDelay: 60_000, StaticHitTime: true, StaticReuse: true,
					Effects: []modelskill.EffectTemplate{{Name: "Buff", Time: 60, Icon: true}},
				}})),
			)
			c, objID := srv.Client, srv.SoleObjectID(t)
			seedKnownSkill(t, srv, objID, int(skillID), 1)
			startInWorld(t, c)
			target := srv.SpawnHostileNPCKindAt(t, tc.kind, location.Location{X: hostileX, Y: hostileY, Z: hostileZ})
			drainUntilQuiet(t, c)

			targetHostile(t, c, target.ObjectID())
			drainUntilQuiet(t, c)

			c.Send(encodeRequestMagicSkillUse(skillID, false, false))
			readCastStartFrames(t, c, objID, skillID, 1, 500, 60_000, target.ObjectID())
			srv.AdvanceUntil(t, "buff lands on the NPC", func() bool {
				_, ok := target.EffectList().ActiveBySkillID(int(skillID))
				return ok
			})

			obj, ok := srv.State.Player(objID)
			if !ok {
				t.Fatal("caster missing from world state")
			}
			caster := obj.(interface{ PvPFlagState() task.PvPFlagState })
			if tc.wantFlag {
				srv.AdvanceUntil(t, "caster PvP-flagged by its helpful cast", func() bool {
					return caster.PvPFlagState() != task.PvPFlagNone
				})
				return
			}
			srv.Settle(t)
			if got := caster.PvPFlagState(); got != task.PvPFlagNone {
				t.Fatalf("caster PvP flag after buffing a %s = %v, want none", tc.kind, got)
			}
		})
	}
}

// TestPlayerSignetSparesNPCInPeaceZone has a player outside any peace zone
// lay a SignetMDam whose radius reaches a monster. The signet skips a
// creature standing in a peace zone, so the monster keeps its HP when a
// peace zone covers it and takes damage when the same zone lies elsewhere.
func TestPlayerSignetSparesNPCInPeaceZone(t *testing.T) {
	t.Parallel()
	for _, tc := range []struct {
		name       string
		peaceMinX  int
		wantDamage bool
	}{
		// The caster stands at x=10, the monster at x=60.
		{name: "monster in peace zone", peaceMinX: 40, wantDamage: false},
		{name: "peace zone elsewhere", peaceMinX: 5_000, wantDamage: true},
	} {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			form, err := zone.NewCuboid(tc.peaceMinX, tc.peaceMinX+1_000, -1_000, 1_000, -1_000, 1_000)
			if err != nil {
				t.Fatal(err)
			}
			zones := zone.NewIndex()
			zones.Add(zone.NewPeace(1, form))
			srv := gameservertest.Boot(t,
				gameservertest.WithCharacter("Mage", 5, 0),
				gameservertest.WithWantChars(1),
				gameservertest.WithZones(zones),
				gameservertest.WithNPCs(npc.NewTable([]*npc.Template{{ID: 13018, Type: "EffectPoint", CollisionRadius: 8}})),
				gameservertest.WithSkills(skillPersistence(t, []modelskill.Definition{signetMDamSkill()})),
			)
			c, objID := srv.Client, srv.SoleObjectID(t)
			seedKnownSkill(t, srv, objID, 1419, 1)
			startInWorld(t, c)
			monster := srv.SpawnHostileNPCKindAt(t, "Monster", location.Location{X: hostileX, Y: hostileY, Z: hostileZ})
			if got := monster.InPeaceZone(); got == tc.wantDamage {
				t.Errorf("monster InPeaceZone = %v, want %v", got, !tc.wantDamage)
			}
			drainUntilQuiet(t, c)
			setCasterMagicRolls(t, srv, objID, func() int { return 500 })

			c.Send(encodeRequestMagicSkillUse(1419, false, false))
			readCastStartFrames(t, c, objID, 1419, 1, 500, 60_000, objID)
			tickSignetMDamLive(t, srv)
			srv.Settle(t)

			hp, maxHP := monster.CurrentHP(), monster.MaxHP()
			if damaged := hp < maxHP; damaged != tc.wantDamage {
				t.Fatalf("monster HP after signet ticks = %d/%d, want damaged=%v", hp, maxHP, tc.wantDamage)
			}
		})
	}
}
