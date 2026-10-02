package skills

import (
	"slices"
	"testing"
	"time"

	"github.com/fatal10110/acis_golang/internal/gameserver/model/actor/npc"
	"github.com/fatal10110/acis_golang/internal/gameserver/model/location"
	modelskill "github.com/fatal10110/acis_golang/internal/gameserver/model/skill"
	"github.com/fatal10110/acis_golang/internal/gameserver/network/serverpackets"
	"github.com/fatal10110/acis_golang/internal/gameservertest"
	"github.com/fatal10110/acis_golang/internal/testsupport"
)

// TestFearLethalRollsAfterFailedLanding casts a FEAR skill carrying lethal2
// (the shape of Turn Undead / Banish Undead) whose landing roll always fails
// (ignoreResists with a zero land rate). The continuous handler still rolls
// the lethal strike after the failed landing: a monster drops to 1 HP and the
// caster sees ATTACK_FAILED before LETHAL_STRIKE_SUCCESSFUL; a raid boss is
// exempt from the strike and keeps its HP.
func TestFearLethalRollsAfterFailedLanding(t *testing.T) {
	for _, tc := range []struct {
		kind       string
		wantLethal bool
	}{
		{kind: "Monster", wantLethal: true},
		{kind: "RaidBoss"},
	} {
		t.Run(tc.kind, func(t *testing.T) {
			t.Parallel()
			const skillID = 1400
			srv := gameservertest.Boot(t,
				gameservertest.WithCharacter("Newbie", 5, 0),
				gameservertest.WithWantChars(1),
				gameservertest.WithSkills(skillPersistence(t, []modelskill.Definition{{
					ID: skillID, Level: 1, Activation: modelskill.ActivationActive, Target: modelskill.TargetOne,
					CastRange: 900, HitTime: 500, ReuseDelay: 60_000, StaticHitTime: true, StaticReuse: true,
					SkillType: "FEAR", Offensive: true, Magic: true, IgnoreResists: true,
					LethalChance2: 100,
				}})),
			)
			c, objID := srv.Client, srv.SoleObjectID(t)
			seedKnownSkill(t, srv, objID, skillID, 1)
			startInWorld(t, c)
			resistLandingRolls(t, srv, objID, randomRoll)
			hostile := srv.SpawnHostileNPCTemplateAt(t, &npc.Template{
				ID: 100, TemplateID: 100, Type: tc.kind, Level: 1, HPMax: 10_000, AtkSpd: 300,
				RunSpeed: 120, WalkSpeed: 60, CollisionRadius: 8, CollisionHeight: 20,
			}, location.Location{X: hostileX, Y: hostileY, Z: hostileZ})
			drainUntilQuiet(t, c)

			maxHP := targetHostile(t, c, hostile.ObjectID())
			drainUntilQuiet(t, c)

			c.Send(encodeRequestMagicSkillUse(skillID, false, false))
			readCastStartFrames(t, c, objID, skillID, 1, 500, 60_000, hostile.ObjectID())
			drainUntilQuiet(t, c)

			advanceUntilCastEnds(t, srv, objID)
			ids := drainSystemMessageIDs(t, c)

			failed := slices.Index(ids, int32(serverpackets.SystemMessageAttackFailed))
			if failed < 0 {
				t.Fatalf("system messages %v, want ATTACK_FAILED for the failed landing", ids)
			}
			lethal := slices.Index(ids, int32(serverpackets.SystemMessageLethalStrikeSuccessful))
			hp := hostile.CurrentHP()
			if !tc.wantLethal {
				if lethal >= 0 || hp != maxHP {
					t.Fatalf("%s HP = %d of %d, messages %v; want no lethal strike", tc.kind, hp, maxHP, ids)
				}
				return
			}
			if hp != 1 {
				t.Fatalf("%s HP after the failed FEAR landing = %d, want 1 from the lethal strike", tc.kind, hp)
			}
			if lethal < failed {
				t.Fatalf("system messages %v, want ATTACK_FAILED before LETHAL_STRIKE_SUCCESSFUL", ids)
			}
		})
	}
}

// drainSystemMessageIDs reads until the client goes quiet and returns the
// SystemMessage ids it saw, in order.
func drainSystemMessageIDs(t *testing.T, c *testsupport.ScriptedClient) []int32 {
	t.Helper()
	var ids []int32
	for range 100 {
		frame := c.ReadWithTimeout(300 * time.Millisecond)
		if frame == nil {
			return ids
		}
		if frame[0] == serverpackets.OpcodeSystemMessage {
			ids = append(ids, wireReader(frame[1:]).ReadInt32())
		}
	}
	t.Fatal("client kept receiving frames after 100 drains")
	return nil
}
