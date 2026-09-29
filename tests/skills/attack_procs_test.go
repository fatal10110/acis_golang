package skills

import (
	"encoding/binary"
	"math"
	"slices"
	"testing"

	"github.com/fatal10110/acis_golang/internal/commons/wire"
	"github.com/fatal10110/acis_golang/internal/gameserver/model/actor/npc"
	"github.com/fatal10110/acis_golang/internal/gameserver/model/actor/player"
	"github.com/fatal10110/acis_golang/internal/gameserver/model/item"
	"github.com/fatal10110/acis_golang/internal/gameserver/model/location"
	"github.com/fatal10110/acis_golang/internal/gameserver/network/clientpackets"
	"github.com/fatal10110/acis_golang/internal/gameserver/network/serverpackets"
	"github.com/fatal10110/acis_golang/internal/gameserver/skill/effect"
	"github.com/fatal10110/acis_golang/internal/gameserver/skill/stat"
	"github.com/fatal10110/acis_golang/internal/gameservertest"
	"github.com/fatal10110/acis_golang/internal/testsupport"
)

// The fixture weapons: the suite catalog's Sword (30), Bow (14) and Wooden
// Arrow (17). A weapon test gives the Sword an on-critical or on-magic skill
// the way the shipped data does with oncrit_skill/oncrit_chance and
// oncast_skill/oncast_chance.
const (
	fixtureSwordID = 30
	fixtureBowID   = 14
	fixtureArrowID = 17
)

// procWeaponCatalog returns the suite item catalog with the Sword's weapon
// data changed by tune.
func procWeaponCatalog(t *testing.T, tune func(*item.WeaponDetail)) *item.Table {
	t.Helper()
	templates := gameservertest.ItemTemplates().All()
	i := slices.IndexFunc(templates, func(tmpl *item.Template) bool { return tmpl.ID == fixtureSwordID })
	if i < 0 || templates[i].Weapon == nil {
		t.Fatalf("fixture catalog has no Sword %d", fixtureSwordID)
	}
	sword := *templates[i]
	weapon := *sword.Weapon
	tune(&weapon)
	sword.Weapon = &weapon
	templates[i] = &sword
	return item.NewTable(templates)
}

// bootArmed boots the chance-skill fixture player against catalog, knowing
// skills, and equips each of equip before returning it in world.
func bootArmed(t *testing.T, catalog *item.Table, roll func(int) int, equip []int32, skills ...int) (*gameservertest.Server, *testsupport.ScriptedClient, int32) {
	t.Helper()
	srv := gameservertest.Boot(t,
		gameservertest.WithCharacter("Newbie", 5, 0),
		gameservertest.WithWantChars(1),
		gameservertest.WithSkills(skillPersistence(t, chanceSkills())),
		gameservertest.WithItemTemplates(catalog),
	)
	c, objID := srv.Client, srv.SoleObjectID(t)
	objects := make([]int32, 0, len(equip))
	for _, templateID := range equip {
		count := int32(1)
		if templateID == fixtureArrowID {
			count = 100
		}
		objects = append(objects, srv.GiveItem(t, objID, templateID, count))
	}
	for _, id := range skills {
		seedKnownSkill(t, srv, objID, id, 1)
	}
	startInWorld(t, c)
	for _, objectID := range objects {
		w := wire.NewPacketWriter(clientpackets.OpcodeUseItem)
		w.WriteInt32(objectID)
		w.WriteInt32(0)
		c.Send(w.Bytes())
		srv.InventoryUpdates.Tick()
		drainUntilQuiet(t, c)
	}
	setPlayerRollSource(t, srv, objID, roll)
	return srv, c, objID
}

// noCritRoll wins every roll but the critical one: the critical roll (out
// of 1000) of 299 still lands against the 300 hit-rate floor and loses to
// the character's critical rate.
func noCritRoll(n int) int {
	if n == 1000 {
		return 299
	}
	return 0
}

func playerHP(t *testing.T, srv *gameservertest.Server, objID int32) float64 {
	t.Helper()
	var hp float64
	onPlayerQueue(t, srv, objID, func(pc *player.Character) { hp = pc.HP() })
	return hp
}

// spawnReflector spawns the parked fixture monster beside the player with
// reflectPercent of REFLECT_DAMAGE_PERCENT.
func spawnReflector(t *testing.T, srv *gameservertest.Server, objID int32, reflectPercent float64) *npc.Hostile {
	t.Helper()
	px, py, pz := srv.PlayerPosition(t, objID)
	hostile := srv.SpawnHostileNPCAt(t, location.Location{X: px + 20, Y: py, Z: pz})
	if reflectPercent > 0 {
		hostile.AddStatFuncs([]effect.Mod{{Stat: stat.ReflectDamagePercent, Op: effect.OpAdd, Value: reflectPercent}})
	}
	drainUntilQuiet(t, srv.Client)
	return hostile
}

// landFirstHit starts the player's auto-attack on hostile and returns once
// its first hit has landed, with the damage that hit dealt.
func landFirstHit(t *testing.T, srv *gameservertest.Server, objID int32, hostile *npc.Hostile) int {
	t.Helper()
	full := hostile.CurrentHP()
	startMelee(t, srv, objID, hostile.ObjectID())
	srv.AdvanceUntil(t, "a landed hit", func() bool { return hostile.CurrentHP() < full })
	return full - hostile.CurrentHP()
}

// lastNumberParam reads the trailing number parameter of a SystemMessage.
func lastNumberParam(frame []byte) int32 {
	return int32(binary.LittleEndian.Uint32(frame[len(frame)-4:]))
}

// TestAutoAttackReflectHurtsAttacker: a player's melee hit on a monster with
// 20% REFLECT_DAMAGE_PERCENT costs the player 20% of the damage it dealt,
// reported to it as damage the monster gave it; a bow hit on the same
// monster reflects nothing.
func TestAutoAttackReflectHurtsAttacker(t *testing.T) {
	for _, tc := range []struct {
		name     string
		equip    []int32
		reflects bool
	}{
		{"melee", []int32{fixtureSwordID}, true},
		{"bow", []int32{fixtureBowID, fixtureArrowID}, false},
	} {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			catalog := gameservertest.ItemTemplates()
			srv, c, objID := bootArmed(t, catalog, noCritRoll, tc.equip)
			hostile := spawnReflector(t, srv, objID, 20)
			before := playerHP(t, srv, objID)

			damage := landFirstHit(t, srv, objID, hostile)
			srv.Settle(t)
			frames := queueFrames(t, c)

			reflected := int(20.0 / 100 * float64(damage))
			if !tc.reflects {
				reflected = 0
			}
			if got := playerHP(t, srv, objID); got != before-float64(reflected) {
				t.Fatalf("player HP after dealing %d = %v, want %v - %d reflected", damage, got, before, reflected)
			}
			i := slices.IndexFunc(frames, func(f []byte) bool {
				return f[0] == serverpackets.OpcodeSystemMessage &&
					wireReader(f[1:]).ReadInt32() == serverpackets.SystemMessageS1GaveYouS2Dmg
			})
			if !tc.reflects {
				if i >= 0 {
					t.Fatalf("bow hit reported reflected damage: %v", systemMessageIDs(frames))
				}
				return
			}
			if reflected <= 0 {
				t.Fatalf("hit of %d reflects nothing at 20%%; the fixture needs a bigger hit", damage)
			}
			if i < 0 {
				t.Fatalf("no S1_GAVE_YOU_S2_DMG among %v", systemMessageIDs(frames))
			}
			if got := lastNumberParam(frames[i]); got != int32(reflected) {
				t.Fatalf("reflected damage reported = %d, want %d", got, reflected)
			}
		})
	}
}

// TestAutoAttackAbsorbHealsAttacker: a wounded player with 50%
// ABSORB_DAMAGE_PERCENT regains half the damage of its landed melee hit,
// and its client gets the new HP.
func TestAutoAttackAbsorbHealsAttacker(t *testing.T) {
	t.Parallel()
	srv, c, objID := bootArmed(t, gameservertest.ItemTemplates(), noCritRoll, []int32{fixtureSwordID})
	onPlayerQueue(t, srv, objID, func(pc *player.Character) {
		pc.AddStatFuncs([]effect.Mod{{Stat: stat.AbsorbDamagePercent, Op: effect.OpAdd, Value: 50}})
		pc.SetHP(1)
	})
	hostile := spawnReflector(t, srv, objID, 0)
	drainUntilQuiet(t, c)

	damage := landFirstHit(t, srv, objID, hostile)
	srv.Settle(t)

	var maxHP float64
	onPlayerQueue(t, srv, objID, func(pc *player.Character) { maxHP = pc.MaxHPValue() })
	want := math.Min(1+50.0/100*float64(damage), maxHP)
	got := playerHP(t, srv, objID)
	if got != want {
		t.Fatalf("player HP after dealing %d = %v, want %v", damage, got, want)
	}
	statusHP := int32(-1)
	for _, frame := range queueFrames(t, c) {
		if frame[0] != serverpackets.OpcodeStatusUpdate {
			continue
		}
		r := wireReader(frame[1:])
		if r.ReadInt32() != objID {
			continue
		}
		for n := r.ReadInt32(); n > 0; n-- {
			attr, value := r.ReadInt32(), r.ReadInt32()
			if attr == int32(serverpackets.StatusCurrentHP) {
				statusHP = value
			}
		}
	}
	if statusHP != int32(got) {
		t.Fatalf("last self StatusUpdate CUR_HP = %d, want %d", statusHP, int32(got))
	}
}

// TestReflectedHitProcsAttackersOnAttackedTrigger: a player under Mirage
// (an ON_ATTACKED chance trigger) whose melee hit a monster reflects casts
// the triggered debuff at the monster; the same hit on a monster that
// reflects nothing does not.
func TestReflectedHitProcsAttackersOnAttackedTrigger(t *testing.T) {
	for _, tc := range []struct {
		name    string
		reflect float64
		procs   bool
	}{
		{"reflected", 20, true},
		{"not reflected", 0, false},
	} {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			srv, c, objID := bootArmed(t, gameservertest.ItemTemplates(), noCritRoll, []int32{fixtureSwordID}, mirageSkill)
			castSelf(t, srv, c, objID, mirageSkill)
			drainUntilQuiet(t, c)
			hostile := spawnReflector(t, srv, objID, tc.reflect)

			landFirstHit(t, srv, objID, hostile)
			srv.Settle(t)
			frames := queueFrames(t, c)

			if !tc.procs {
				if seen := procSeen(t, frames, objID, mirageTriggered, 1, hostile.ObjectID()); seen.launched >= 0 || seen.used >= 0 {
					t.Fatalf("unreflected hit cast %d: %+v", mirageTriggered, seen)
				}
				return
			}
			assertProc(t, frames, objID, mirageTriggered, 1, hostile.ObjectID())
			if !holds(hostile.EffectList().All(), mirageTriggered) {
				t.Fatalf("hostile effects = %v, want the triggered %d debuff", hostile.EffectList().All(), mirageTriggered)
			}
		})
	}
}

// TestWeaponOnCritSkillLandsOnCriticalHit: a Sword carrying onCritTriggered
// as its 5% on-critical skill lands it on the monster when the player's
// melee hit is critical and the chance roll wins, with no launch or
// animation of its own; a normal hit lands nothing.
func TestWeaponOnCritSkillLandsOnCriticalHit(t *testing.T) {
	for _, tc := range []struct {
		name  string
		roll  func(int) int
		lands bool
	}{
		{"critical hit", func(int) int { return 0 }, true},
		{"normal hit", noCritRoll, false},
	} {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			catalog := procWeaponCatalog(t, func(w *item.WeaponDetail) {
				w.OnCritSkill = &item.SkillTrigger{Skill: item.SkillRef{ID: onCritTriggered, Level: 1}, Chance: 5}
			})
			srv, c, objID := bootArmed(t, catalog, tc.roll, []int32{fixtureSwordID})
			hostile := spawnReflector(t, srv, objID, 0)

			landFirstHit(t, srv, objID, hostile)
			srv.Settle(t)
			frames := queueFrames(t, c)

			if seen := procSeen(t, frames, objID, onCritTriggered, 1, hostile.ObjectID()); seen.launched >= 0 || seen.used >= 0 {
				t.Fatalf("on-critical weapon skill was broadcast as a cast: %+v", seen)
			}
			if got := holds(hostile.EffectList().All(), onCritTriggered); got != tc.lands {
				t.Fatalf("hostile holds %d = %v, want %v", onCritTriggered, got, tc.lands)
			}
		})
	}
}

// TestWeaponOnCritSkillMissesItsChance: the on-critical skill's 5% chance
// roll of 5 loses, so a critical hit lands nothing.
func TestWeaponOnCritSkillMissesItsChance(t *testing.T) {
	t.Parallel()
	catalog := procWeaponCatalog(t, func(w *item.WeaponDetail) {
		w.OnCritSkill = &item.SkillTrigger{Skill: item.SkillRef{ID: onCritTriggered, Level: 1}, Chance: 5}
	})
	srv, _, objID := bootArmed(t, catalog, func(n int) int {
		if n == 100 {
			return 5
		}
		return 0
	}, []int32{fixtureSwordID})
	hostile := spawnReflector(t, srv, objID, 0)

	landFirstHit(t, srv, objID, hostile)
	srv.Settle(t)
	if holds(hostile.EffectList().All(), onCritTriggered) {
		t.Fatal("on-critical skill landed on a lost chance roll")
	}
}

// TestWeaponOnMagicSkillFiresOnMatchingCast: a Sword carrying
// onGoodTriggered, a good skill, as its 5% on-magic skill fires when the
// player's good self buff lands: the player is told S1_HAS_BEEN_ACTIVATED
// and then holds the skill's buff. A good toggle and an offensive skill set
// nothing off.
func TestWeaponOnMagicSkillFiresOnMatchingCast(t *testing.T) {
	for _, tc := range []struct {
		name  string
		cast  int32
		fires bool
	}{
		{"good self buff", selfBuffSkill, true},
		{"toggle", toggleSkill, false},
	} {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			catalog := procWeaponCatalog(t, func(w *item.WeaponDetail) {
				w.OnCastSkill = &item.SkillTrigger{Skill: item.SkillRef{ID: onGoodTriggered, Level: 1}, Chance: 5}
			})
			srv, c, objID := bootArmed(t, catalog, func(int) int { return 0 }, []int32{fixtureSwordID}, int(tc.cast))
			castSelf(t, srv, c, objID, tc.cast)
			srv.Settle(t)
			frames := queueFrames(t, c)

			activated := slices.IndexFunc(frames, func(f []byte) bool {
				return f[0] == serverpackets.OpcodeSystemMessage &&
					wireReader(f[1:]).ReadInt32() == serverpackets.SystemMessageS1HasBeenActivated
			})
			held := slices.Contains(liveHeldSkillIDs(t, srv, objID), onGoodTriggered)
			if !tc.fires {
				if activated >= 0 || held {
					t.Fatalf("%s set off the on-magic skill: message at %d, held %v", tc.name, activated, held)
				}
				return
			}
			if activated < 0 {
				t.Fatalf("no S1_HAS_BEEN_ACTIVATED among %v", systemMessageIDs(frames))
			}
			assertSystemMessageSkillFrame(t, frames[activated], serverpackets.SystemMessageS1HasBeenActivated, onGoodTriggered, 1)
			if !held {
				t.Fatalf("held skills = %v, want the on-magic %d buff", liveHeldSkillIDs(t, srv, objID), onGoodTriggered)
			}
		})
	}
}

// TestWeaponOnMagicSkillIgnoresOtherKind: a good on-magic skill stays quiet
// when the player lands an offensive MDAM on a monster.
func TestWeaponOnMagicSkillIgnoresOtherKind(t *testing.T) {
	t.Parallel()
	catalog := procWeaponCatalog(t, func(w *item.WeaponDetail) {
		w.OnCastSkill = &item.SkillTrigger{Skill: item.SkillRef{ID: onGoodTriggered, Level: 1}, Chance: -1}
	})
	srv, c, objID := bootArmed(t, catalog, mdamLandsRoll, []int32{fixtureSwordID}, mdamSkill)
	hostile := spawnReflector(t, srv, objID, 0)
	landMDAM(t, srv, objID, hostile)

	if ids := systemMessageIDs(queueFrames(t, c)); slices.Contains(ids, int32(serverpackets.SystemMessageS1HasBeenActivated)) {
		t.Fatalf("offensive cast set off a good on-magic skill: %v", ids)
	}
	if slices.Contains(liveHeldSkillIDs(t, srv, objID), onGoodTriggered) {
		t.Fatal("offensive cast landed the good on-magic skill")
	}
}

// mdamLandsRoll wins every chance roll (out of 100) and loses the magic
// resist and magic critical rolls, so an MDAM lands as a plain hit.
func mdamLandsRoll(n int) int {
	if n == 100 {
		return 0
	}
	return n - 1
}

// landMDAM targets hostile and returns once the player's MDAM has landed on
// it and the server has settled.
func landMDAM(t *testing.T, srv *gameservertest.Server, objID int32, hostile *npc.Hostile) {
	t.Helper()
	c := srv.Client
	full := hostile.CurrentHP()
	px, py, pz := srv.PlayerPosition(t, objID)
	c.Send(encodeAction(hostile.ObjectID(), int32(px), int32(py), int32(pz), false))
	drainUntilQuiet(t, c)
	c.Send(encodeRequestMagicSkillUse(mdamSkill, false, false))
	srv.AdvanceUntil(t, "MDAM lands", func() bool { return hostile.CurrentHP() < full })
	srv.Settle(t)
}

// TestWeaponOnMagicOffensiveSkillNeedsItsLandingRoll: an offensive on-magic
// weapon skill fires on the player's offensive MDAM only once its own
// landing roll on the monster wins. onOffensiveTriggered lands at a fixed
// 100%: the player is told S1_HAS_BEEN_ACTIVATED and the monster holds the
// debuff. unlandableDebuff lands at a fixed 0%: no message, no debuff.
func TestWeaponOnMagicOffensiveSkillNeedsItsLandingRoll(t *testing.T) {
	for _, tc := range []struct {
		name   string
		weapon int32
		fires  bool
	}{
		{"landing roll wins", onOffensiveTriggered, true},
		{"landing roll loses", unlandableDebuff, false},
	} {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			catalog := procWeaponCatalog(t, func(w *item.WeaponDetail) {
				w.OnCastSkill = &item.SkillTrigger{Skill: item.SkillRef{ID: tc.weapon, Level: 1}, Chance: 5}
			})
			srv, c, objID := bootArmed(t, catalog, mdamLandsRoll, []int32{fixtureSwordID}, mdamSkill)
			hostile := spawnReflector(t, srv, objID, 0)
			landMDAM(t, srv, objID, hostile)
			frames := queueFrames(t, c)

			activated := slices.IndexFunc(frames, func(f []byte) bool {
				return f[0] == serverpackets.OpcodeSystemMessage &&
					wireReader(f[1:]).ReadInt32() == serverpackets.SystemMessageS1HasBeenActivated
			})
			held := holds(hostile.EffectList().All(), int(tc.weapon))
			if !tc.fires {
				if activated >= 0 || held {
					t.Fatalf("lost landing roll still fired the on-magic skill: message at %d, monster holds it %v", activated, held)
				}
				return
			}
			if activated < 0 {
				t.Fatalf("no S1_HAS_BEEN_ACTIVATED among %v", systemMessageIDs(frames))
			}
			assertSystemMessageSkillFrame(t, frames[activated], serverpackets.SystemMessageS1HasBeenActivated, tc.weapon, 1)
			if !held {
				t.Fatalf("hostile effects = %v, want the on-magic %d debuff", hostile.EffectList().All(), tc.weapon)
			}
		})
	}
}
