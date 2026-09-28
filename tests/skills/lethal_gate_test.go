package skills

import (
	"testing"

	"github.com/fatal10110/acis_golang/internal/gameserver/model/actor/npc"
	"github.com/fatal10110/acis_golang/internal/gameserver/model/location"
	modelskill "github.com/fatal10110/acis_golang/internal/gameserver/model/skill"
	"github.com/fatal10110/acis_golang/internal/gameservertest"
)

// TestLethalStrikeSkipsRaidAndNonLethalableNPCs casts a sure-fire lethal2
// PDAM (level 5 caster, level 1 target: rate 5000/1000) at real NPCs.
// Formulas.calcLethalHit returns before rolling when the target is raid
// related (RaidBoss/GrandBoss constructors call setRaidRelated) or when
// Attackable.isLethalable rejects its npc id; a plain monster drops to 1 HP.
func TestLethalStrikeSkipsRaidAndNonLethalableNPCs(t *testing.T) {
	for _, tc := range []struct {
		name       string
		id         int
		kind       string
		wantLethal bool
	}{
		{name: "monster", id: 100, kind: "Monster", wantLethal: true},
		{name: "raid boss", id: 100, kind: "RaidBoss"},
		{name: "grand boss", id: 100, kind: "GrandBoss"},
		{name: "tyrannosaurus", id: 22215, kind: "Monster"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			const skillID = 43
			srv := gameservertest.Boot(t,
				gameservertest.WithCharacter("Newbie", 5, 0),
				gameservertest.WithWantChars(1),
				gameservertest.WithSkills(skillPersistence(t, []modelskill.Definition{{
					ID: skillID, Level: 1, Activation: modelskill.ActivationActive, Target: modelskill.TargetOne,
					CastRange: 900, HitTime: 500, ReuseDelay: 60_000, StaticHitTime: true, StaticReuse: true,
					SkillType: "PDAM", Power: 10, LethalChance2: 100,
				}})),
			)
			c, objID := srv.Client, srv.SoleObjectID(t)
			seedKnownSkill(t, srv, objID, skillID, 1)
			startInWorld(t, c)
			hostile := srv.SpawnHostileNPCTemplateAt(t, &npc.Template{
				ID: tc.id, TemplateID: tc.id, Type: tc.kind, Level: 1, HPMax: 10_000, PDef: 10_000, AtkSpd: 300,
				RunSpeed: 120, WalkSpeed: 60, CollisionRadius: 8, CollisionHeight: 20,
			}, location.Location{X: hostileX, Y: hostileY, Z: hostileZ})
			drainUntilQuiet(t, c)

			maxHP := targetHostile(t, c, hostile.ObjectID())
			drainUntilQuiet(t, c)

			c.Send(encodeRequestMagicSkillUse(skillID, false, false))
			readCastStartFrames(t, c, objID, skillID, 1, 500, 60_000, hostile.ObjectID())
			drainUntilQuiet(t, c)

			srv.AdvanceUntil(t, "PDAM hit", func() bool { return hostile.CurrentHP() < maxHP })
			hp := hostile.CurrentHP()
			if tc.wantLethal {
				if hp != 1 {
					t.Fatalf("%s HP after lethal2 PDAM = %d, want 1", tc.kind, hp)
				}
				return
			}
			if hp <= maxHP/2 {
				t.Fatalf("%s %d HP after lethal2 PDAM = %d of %d, want only the skill damage (no lethal strike)", tc.kind, tc.id, hp, maxHP)
			}
		})
	}
}
