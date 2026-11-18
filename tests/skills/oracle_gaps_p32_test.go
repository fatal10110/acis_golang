package skills

import (
	"testing"
	"time"

	"github.com/fatal10110/acis_golang/internal/commons/wire"
	"github.com/fatal10110/acis_golang/internal/gameserver/model/actor/player"
	modelskill "github.com/fatal10110/acis_golang/internal/gameserver/model/skill"
	"github.com/fatal10110/acis_golang/internal/gameserver/network/clientpackets"
	"github.com/fatal10110/acis_golang/internal/gameserver/network/serverpackets"
	"github.com/fatal10110/acis_golang/internal/gameserver/skill/effect"
	"github.com/fatal10110/acis_golang/internal/gameserver/skill/stat"
	"github.com/fatal10110/acis_golang/internal/gameservertest"
)

// TestEffectTemplateLandingBranches casts a DEBUFF whose skill-level landing
// roll always passes and whose single effect template carries an effect power
// of 0, so only the template's own landing decides whether the effect is
// created. A CANCEL-typed template lands regardless of its power; an untyped
// template rolls its bare power, here 0, and never lands; a template of 100
// always lands.
func TestEffectTemplateLandingBranches(t *testing.T) {
	for _, tc := range []struct {
		name       string
		effectType string
		power      float64
		lands      bool
	}{
		{name: "cancel type lands at power 0", effectType: "CANCEL", power: 0, lands: true},
		{name: "untyped rolls its bare power 0", effectType: "", power: 0, lands: false},
		{name: "untyped rolls its bare power 100", effectType: "", power: 100, lands: true},
	} {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			const skillID = 1501
			srv := gameservertest.Boot(t,
				gameservertest.WithCharacter("Newbie", 5, 0),
				gameservertest.WithWantChars(1),
				gameservertest.WithSkills(skillPersistence(t, []modelskill.Definition{{
					ID: skillID, Level: 1, Activation: modelskill.ActivationActive, Target: modelskill.TargetOne,
					CastRange: 900, HitTime: 0, StaticHitTime: true,
					SkillType: "DEBUFF", Debuff: true, Offensive: true, Power: 80,
					Effects: []modelskill.EffectTemplate{{
						Name: "Debuff", Time: 60,
						EffectPower: tc.power, EffectPowerSet: true, EffectType: tc.effectType,
					}},
				}})),
			)
			c, objID := srv.Client, srv.SoleObjectID(t)
			seedKnownSkill(t, srv, objID, skillID, 1)
			startInWorld(t, c)
			hostile := srv.SpawnHostileNPC(t)
			drainUntilQuiet(t, c)
			targetHostile(t, c, hostile.ObjectID())
			drainUntilQuiet(t, c)
			onPlayerQueue(t, srv, objID, func(pc *player.Character) {
				pc.SetRollSource(func(int) int { return 0 })
			})

			c.Send(encodeRequestMagicSkillUse(skillID, false, false))
			drainUntilQuiet(t, c)
			want := 0
			if tc.lands {
				want = 1
			}
			if got := len(hostile.EffectList().All()); got != want {
				t.Fatalf("monster effects = %d, want %d", got, want)
			}
		})
	}
}

// TestPhysicalSkillEvasionNeedsWeapon casts a physical skill at a monster that
// evades every physical skill (P_SKILL_EVASION 100). Evasion is only rolled for
// a caster with a weapon equipped: the armed cast keeps the monster's HP and
// tells the caster the target dodged, the unarmed cast and the cast at a
// monster without the evasion drain it, which shows the wait was long enough
// for the hit.
func TestPhysicalSkillEvasionNeedsWeapon(t *testing.T) {
	for _, tc := range []struct {
		name    string
		armed   bool
		evasion bool
	}{
		{name: "armed caster, evading target", armed: true, evasion: true},
		{name: "armed caster, plain target", armed: true, evasion: false},
		{name: "unarmed caster, evading target", armed: false, evasion: true},
	} {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			const skillID = 42
			srv := gameservertest.Boot(t,
				gameservertest.WithCharacter("Newbie", 5, 0),
				gameservertest.WithWantChars(1),
				gameservertest.WithSkills(skillPersistence(t, []modelskill.Definition{{
					ID: skillID, Level: 1, Activation: modelskill.ActivationActive, Target: modelskill.TargetOne,
					CastRange: 900, HitTime: 500, StaticHitTime: true,
					SkillType: "PDAM", Power: 1_000_000,
				}})),
			)
			c, objID := srv.Client, srv.SoleObjectID(t)
			seedKnownSkill(t, srv, objID, skillID, 1)
			var weapon int32
			if tc.armed {
				weapon = srv.GiveItem(t, objID, 30, 1)
			}
			startInWorld(t, c)
			if tc.armed {
				w := wire.NewPacketWriter(clientpackets.OpcodeUseItem)
				w.WriteInt32(weapon)
				w.WriteInt32(0)
				c.Send(w.Bytes())
				srv.InventoryUpdates.Tick()
				drainUntilQuiet(t, c)
			}

			hostile := srv.SpawnHostileNPC(t)
			if tc.evasion {
				hostile.AddStatFuncs([]effect.Mod{{Stat: stat.PSkillEvasion, Op: effect.OpSet, Value: 100}})
			}
			drainUntilQuiet(t, c)
			maxHP := targetHostile(t, c, hostile.ObjectID())
			drainUntilQuiet(t, c)

			c.Send(encodeRequestMagicSkillUse(skillID, false, false))
			readCastStartFrames(t, c, objID, skillID, 1, 500, 0, hostile.ObjectID())
			srv.Advance(t, 2*time.Second)
			srv.Settle(t)
			evaded := tc.armed && tc.evasion
			if hp := hostile.CurrentHP(); (hp < maxHP) == evaded {
				t.Fatalf("monster HP after the skill = %d of %d, want evaded %v", hp, maxHP, evaded)
			}
			frames := c.ReadQueued()
			if evaded {
				assertOneSystemMessage(t, frames, serverpackets.SystemMessageS1DodgesAttack)
			} else {
				assertNoSystemMessage(t, frames, serverpackets.SystemMessageS1DodgesAttack)
			}
		})
	}
}
