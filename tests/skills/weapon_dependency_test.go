package skills

import (
	"slices"
	"testing"
	"time"

	"github.com/fatal10110/acis_golang/internal/gameserver/model/actor/ai"
	"github.com/fatal10110/acis_golang/internal/gameserver/model/actor/npc"
	"github.com/fatal10110/acis_golang/internal/gameserver/model/actor/player"
	"github.com/fatal10110/acis_golang/internal/gameserver/model/item"
	modelskill "github.com/fatal10110/acis_golang/internal/gameserver/model/skill"
	"github.com/fatal10110/acis_golang/internal/gameserver/network/serverpackets"
	"github.com/fatal10110/acis_golang/internal/gameservertest"
)

const (
	// fixtureShieldID is a shield added to the suite catalog: an untyped
	// left-hand armor, which the item data reads as a SHIELD.
	fixtureShieldID = 625
	// weaponSkillID is an active self skill restricted to some weapon or
	// shield types, like the ~110 shipped skills carrying weaponsAllowed.
	weaponSkillID = 3211
	// weaponPassiveID is a passive ON_HIT chance skill restricted the same
	// way, triggering onHitTriggered.
	weaponPassiveID = 3210
)

// weaponCatalog is the suite item catalog plus the fixture shield.
func weaponCatalog() *item.Table {
	return item.NewTable(append(gameservertest.ItemTemplates().All(), &item.Template{
		ID:            fixtureShieldID,
		Name:          "Shield",
		Kind:          item.KindArmor,
		Slot:          item.SlotLHand,
		Duration:      -1,
		Destroyable:   true,
		DefaultAction: item.ActionEquip,
		Armor:         item.NewArmorDetail(item.ArmorNone, item.SlotLHand),
	}))
}

func weaponSkill(allowed string) modelskill.Definition {
	return modelskill.Definition{
		ID: weaponSkillID, Level: 1, Activation: modelskill.ActivationActive, Target: modelskill.TargetSelf,
		HitTime: 500, StaticHitTime: true, MPConsume: 5, SkillType: "DUMMY",
		WeaponsAllowed: allowed,
	}
}

func playerMP(t *testing.T, srv *gameservertest.Server, objID int32) int {
	t.Helper()
	var mp int
	onPlayerQueue(t, srv, objID, func(pc *player.Character) { mp = pc.CurrentMP() })
	return mp
}

// TestPlayerCastNeedsAllowedWeapon pins L2Skill.getWeaponDependancy as
// CreatureCast.canCast runs it for a player: the active weapon's type (the
// class fists, a FIST, when the right hand is empty) plus a left-hand
// shield must match one of the skill's weaponsAllowed types. A mismatch
// answers S1_CANNOT_BE_USED naming the skill, with no ActionFailed, no cast
// and no MP spent; a match casts.
func TestPlayerCastNeedsAllowedWeapon(t *testing.T) {
	t.Parallel()
	for _, tc := range []struct {
		name    string
		allowed string
		equip   []int32
		casts   bool
	}{
		{"bow skill bare-handed", "BOW", nil, false},
		{"bow skill with a sword", "BOW", []int32{fixtureSwordID}, false},
		{"bow skill with a bow", "BOW", []int32{fixtureBowID, fixtureArrowID}, true},
		{"fist skill bare-handed", "FIST,DUALFIST", nil, true},
		{"fist skill with a sword", "FIST,DUALFIST", []int32{fixtureSwordID}, false},
		{"shield skill with a sword alone", "SHIELD", []int32{fixtureSwordID}, false},
		{"shield skill with a sword and shield", "SHIELD", []int32{fixtureSwordID, fixtureShieldID}, true},
	} {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			defs := []modelskill.Definition{weaponSkill(tc.allowed)}
			srv, c, objID := bootArmedWith(t, defs, weaponCatalog(), noCritRoll, tc.equip, weaponSkillID)
			mp := playerMP(t, srv, objID)

			c.Send(encodeRequestMagicSkillUse(weaponSkillID, false, false))
			frame := c.Read()
			if tc.casts {
				assertFrameOpcode(t, frame, serverpackets.OpcodeMagicSkillUse, tc.name)
				return
			}
			assertSystemMessageSkillFrame(t, frame, serverpackets.SystemMessageS1CannotBeUsed, weaponSkillID, 1)
			assertNoActionFailedUntilQuiet(t, c, tc.name)
			if srv.PlayerCastingNow(t, objID) {
				t.Fatalf("%s started a cast", tc.name)
			}
			if got := playerMP(t, srv, objID); got != mp {
				t.Fatalf("%s: MP = %d after the refusal, want untouched %d", tc.name, got, mp)
			}
		})
	}
}

// TestChanceSkillNeedsAllowedWeapon pins ChanceSkillList.makeCast's
// getWeaponDependancy gate: a sword-wielding player's passive ON_HIT skill
// restricted to bows never procs and names itself in S1_CANNOT_BE_USED,
// while the same skill restricted to swords procs. The weapon is checked
// before the skill's <cond> clauses, so a skill failing both reports the
// weapon only.
func TestChanceSkillNeedsAllowedWeapon(t *testing.T) {
	t.Parallel()
	for _, tc := range []struct {
		name    string
		allowed string
		cond    bool
		procs   bool
	}{
		{"weapon held", "SWORD", false, true},
		{"weapon not held", "BOW", false, false},
		{"weapon checked before condition", "BOW", true, false},
	} {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			passive := modelskill.Definition{
				ID: weaponPassiveID, Level: 1, Activation: modelskill.ActivationPassive, Target: modelskill.TargetSelf,
				SkillType: "BUFF", ChanceType: "ON_HIT", ActivationChance: 5,
				TriggeredID: onHitTriggered, TriggeredLevel: 1,
				WeaponsAllowed: tc.allowed,
			}
			if tc.cond {
				// The player is at full HP, so the clause fails.
				passive.Conditions = []modelskill.ConditionClause{{
					Root:      modelskill.Condition{Kind: "player", Attrs: map[string]string{"hp": "25"}},
					MessageID: serverpackets.SystemMessageCantSeeTarget,
				}}
			}
			defs := append(chanceSkills(), passive)
			srv, c, objID := bootArmedWith(t, defs, weaponCatalog(), noCritRoll, []int32{fixtureSwordID}, weaponPassiveID)
			hostile := spawnReflector(t, srv, objID, 0)

			landFirstHit(t, srv, objID, hostile)
			srv.Settle(t)
			frames := queueFrames(t, c)
			if tc.procs {
				assertProc(t, frames, objID, onHitTriggered, 1, objID)
				assertNoSystemMessage(t, frames, serverpackets.SystemMessageS1CannotBeUsed)
				return
			}
			if seen := procSeen(t, frames, objID, onHitTriggered, 1, objID); seen.launched >= 0 || seen.used >= 0 {
				t.Fatalf("weapon-restricted passive still cast %d: %+v", onHitTriggered, seen)
			}
			if slices.Contains(liveHeldSkillIDs(t, srv, objID), onHitTriggered) {
				t.Fatal("weapon-restricted passive still applied the triggered buff")
			}
			assertNoSystemMessage(t, frames, serverpackets.SystemMessageCantSeeTarget)
			i := slices.IndexFunc(frames, func(f []byte) bool {
				return f[0] == serverpackets.OpcodeSystemMessage &&
					wireReader(f[1:]).ReadInt32() == serverpackets.SystemMessageS1CannotBeUsed
			})
			if i < 0 {
				t.Fatalf("no S1_CANNOT_BE_USED among %v", systemMessageIDs(frames))
			}
			assertSystemMessageSkillFrame(t, frames[i], serverpackets.SystemMessageS1CannotBeUsed, weaponPassiveID, 1)
		})
	}
}

// TestNPCCastNeedsAllowedWeapon pins the same gate for a monster's
// desire-driven cast (CreatureAI.thinkCast -> CreatureCast.canCast): its
// template's right-hand weapon and left-hand shield must match the skill's
// weaponsAllowed types. An unarmed monster holds no weapon type, not even a
// fist's (Npc.getActiveWeaponItem is null). A refused cast starts nothing
// and tells no one.
func TestNPCCastNeedsAllowedWeapon(t *testing.T) {
	t.Parallel()
	for _, tc := range []struct {
		name         string
		allowed      string
		rHand, lHand int
		casts        bool
	}{
		{"unarmed", "SWORD", 0, 0, false},
		{"unarmed fist skill", "FIST,DUALFIST", 0, 0, false},
		{"sword", "SWORD", fixtureSwordID, 0, true},
		{"bow skill with a sword", "BOW", fixtureSwordID, 0, false},
		{"shield", "SHIELD", fixtureSwordID, fixtureShieldID, true},
	} {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			srv := gameservertest.Boot(t,
				gameservertest.WithCharacter("Newbie", 5, 0),
				gameservertest.WithWantChars(1),
				gameservertest.WithItemTemplates(weaponCatalog()),
			)
			startInWorld(t, srv.Client)
			def := weaponSkill(tc.allowed)
			def.MPConsume = 0
			hostile, _ := srv.SpawnCastingHostileNPC(t, &npc.Template{
				ID: 100, TemplateID: 100, Type: "Monster", Level: 1, HPMax: 1000,
				AtkSpd: 300, RunSpeed: 120, WalkSpeed: 60, CollisionRadius: 8, CollisionHeight: 20,
				RightHand: tc.rHand, LeftHand: tc.lHand,
			}, modelskill.NewTable([]modelskill.Definition{def}))
			drainUntilQuiet(t, srv.Client)
			hostile.AI().Desires().AddOrUpdate(&ai.Desire{
				Kind: ai.IntentionCast, FinalTarget: hostile, Skill: modelskill.Ref{ID: weaponSkillID, Level: 1},
				Weight: 1_000_000, QueuedAt: hostile.Now(),
			})

			thinkOnNPCQueue(t, hostile)
			if tc.casts {
				assertFrameOpcode(t, srv.Client.Read(), serverpackets.OpcodeMagicSkillUse, tc.name)
				return
			}
			for {
				frame := srv.Client.ReadWithTimeout(300 * time.Millisecond)
				if frame == nil {
					break
				}
				if frame[0] == serverpackets.OpcodeMagicSkillUse || frame[0] == serverpackets.OpcodeSystemMessage {
					t.Fatalf("%s: refused monster cast sent opcode %#x", tc.name, frame[0])
				}
			}
			if hostile.CastingNow() {
				t.Fatalf("%s: monster is casting", tc.name)
			}
		})
	}
}
